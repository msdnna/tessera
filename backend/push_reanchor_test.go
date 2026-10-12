// Строку аутбокса для device-канала маршрутизатор пишет иначе, чем все
// остальные, и делает это намеренно: `next_attempt_at = now`, `digest_group =
// ""` — «No digest group and no deferral: a push that arrives batched half an
// hour late is worse than no push at all». Тихие часы для пуша тоже не
// откладывание, а подавление: внутри окна строка не пишется вовсе.
//
// reanchorDeliveries этого различия не знает. Она пересчитывает КАЖДУЮ
// нетронутую pending-строку пользователя через digestSlot — потому что
// ListReschedulableDeliveries отбирает по `status='pending' AND attempts=0` и
// ни о чём не спрашивает канал. Поэтому сохранение настроек уведомлений (любое,
// меняющее расписание: окно свода, тихие часы, зона) забирает живой пуш,
// который маршрутизатор только что разрешил отправить немедленно, и уносит его
// в будущий слот сетки — до суток вперёд — да ещё и в группу свода. Группа
// означает, что drainDeliveries склеит такие пуши в одно сообщение с заголовком
// «Сводка» и без ссылки на задачу: deliverGroup отправляет Kind "digest" без
// TaskID и ConferenceID, то есть пуш теряет ещё и переход по нажатию.
//
// Окно узкое — строка живёт от постановки до ближайшего тика воркера (10 с), —
// но сам сюжет обычный: пуш прилетает ровно тогда, когда человек в приложении,
// а экран настроек уведомлений — то место, где он в этот момент с большой
// вероятностью и находится (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tessera/handlers"
)

// TestPrefsSaveDoesNotBatchPendingPush: сохранение настроек не трогает строку
// пуша, которую маршрутизатор поставил «на сейчас и без свода».
func TestPrefsSaveDoesNotBatchPendingPush(t *testing.T) {
	c := signup(t)

	// Харнесс крутит воркер аутбокса на тикере, а sender'а типа device в нём
	// нет: незанятая строка была бы закрыта как failed раньше, чем тест успеет
	// сохранить настройки. Лок держим весь тест — так же, как сканерный.
	release := holdDeliveryLock(t)
	defer release()

	// Канал устройства с пуш-токеном — то, что регистрирует Android-клиент.
	dev := c.expect(t, c.post("/notification-devices", map[string]any{
		"device_id": "dev-push-reanchor", "platform": "android", "fcm_token": "tok-push-reanchor",
	}), http.StatusCreated)
	devID, _ := dev["id"].(string)
	if devID == "" {
		t.Fatalf("у зарегистрированного устройства нет канала: %v", dev)
	}

	// Контроль: обычный канал, строку которого переанкорить как раз НУЖНО.
	hook, _ := countingHook(t)
	hookID := mkWebhookChannelTmpl(t, c, "Хук контроля", hook, "")

	// Ровно то, что пишет routeNotification: обоим каналам — «сейчас», без
	// группы. Дальше расходятся они только из-за типа канала.
	planted := time.Now()
	notifID := plantNotification(t, c.UserID, "mention", "вас упомянули")
	plantDelivery(t, notifID, devID, planted)
	plantDelivery(t, notifID, hookID, planted)

	// Сохранение настроек с новым окном свода — самый обычный повод для
	// reanchorDeliveries.
	setDigestPrefs(t, c, digestHour)

	devGroup, devNext := deliveryFor(t, notifID, devID)
	if devGroup != "" {
		t.Errorf("пуш попал в группу свода %q: drainDeliveries склеит его в «Сводку» без ссылки на задачу", devGroup)
	}
	if late := devNext.Sub(planted); late > time.Minute {
		t.Errorf("пуш отложен на %s вперёд, want «сейчас»: маршрутизатор разрешил его немедленно, "+
			"сохранение настроек унесло в слот дайджеста", late.Round(time.Second))
	}

	// Контроль: на обычном канале пересчёт обязан сработать — иначе тест зеленеет
	// на фиксе, который просто выключил reanchorDeliveries целиком.
	hookGroup, hookNext := deliveryFor(t, notifID, hookID)
	if hookGroup == "" || !hookNext.After(planted) {
		t.Fatalf("контроль не сработал: строка обычного канала не переанкорена (группа %q, момент %s) — "+
			"проверка пуша ничего не доказывает", hookGroup, hookNext.Format(time.RFC3339))
	}
}

// plantNotification кладёт уведомление без задачи и пространства и возвращает
// его id.
func plantNotification(t *testing.T, userID, kind, text string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO notifications (user_id, kind, text) VALUES ($1, $2, $3) RETURNING id`,
		userID, kind, text).Scan(&id); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	return id
}

// plantDelivery кладёт строку аутбокса так, как её пишет routeNotification для
// доставки без отсрочки: готова сейчас, вне группы свода.
func plantDelivery(t *testing.T, notifID, chID string, at time.Time) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO notification_deliveries (notification_id, channel_id, next_attempt_at, digest_group, created_at)
		 VALUES ($1, $2, $3, '', $3)`, notifID, chID, at); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
}

// deliveryFor — как выглядит строка аутбокса конкретного канала: группа свода и
// момент, с которого её можно брать.
func deliveryFor(t *testing.T, notifID, chID string) (string, time.Time) {
	t.Helper()
	var group string
	var next time.Time
	if err := testPool.QueryRow(context.Background(),
		`SELECT digest_group, next_attempt_at FROM notification_deliveries
		  WHERE notification_id = $1 AND channel_id = $2`, notifID, chID).Scan(&group, &next); err != nil {
		t.Fatalf("строки аутбокса канала %s не нашлось: %v", chID, err)
	}
	return group, next
}

// holdDeliveryLock забирает advisory-лок воркера доставки и возвращает функцию
// освобождения (идемпотентную; освобождается и по концу теста).
func holdDeliveryLock(t *testing.T) func() {
	t.Helper()
	ctx := context.Background()
	holder, err := testPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire holder conn: %v", err)
	}
	key := handlers.AdvisoryLockKey(handlers.NotifyDeliveryLock)
	var locked bool
	if err := holder.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		holder.Release()
		t.Fatalf("could not take the delivery lock: locked=%v err=%v", locked, err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		_, _ = holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		holder.Release()
	}
	t.Cleanup(release)
	return release
}
