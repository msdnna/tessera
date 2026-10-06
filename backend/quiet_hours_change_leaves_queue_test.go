// Тихие часы спрашивают РОВНО ОДИН РАЗ — при постановке строки в аутбокс:
// routeNotification сдвигает доставку на конец окна и записывает этот момент в
// next_attempt_at. Дальше строка живёт по абсолютному времени, а настройка —
// своей жизнью, и смена окна до неё не доходит.
//
// Обе стороны ломаются молча и наблюдаемо:
//   - выключил тишину в 23:30 («я ещё работаю, шлите») — PUT отвечает 200, экран
//     показывает выключенный тумблер, а придержанные письма всё равно ждут до
//     08:00: очередь заморожена настройкой, которой больше нет;
//   - расширил окно до 10:00 — уже отложенные выстрелят в 08:00, посреди
//     тишины, которую пользователь только что объявил.
//
// Тот же класс, что «смена срока хранения не доходила до уже снятых записей»
// (#2932): значение штампуется при записи, а настройка читается как правило.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// heldWarning ставит в аутбокс предупреждение о сроке внутри тихих часов,
// заканчивающихся через until, и возвращает момент, до которого строка
// придержана. Путь настоящий: сканер сроков → routeNotification → аутбокс.
func heldWarning(t *testing.T, c *client, until time.Duration) time.Time {
	t.Helper()
	chID := mkWebhookChannelTmpl(t, c, "Хук сроков", brokenHook(t), "")
	mkCatchAllRoute(t, c, chID)
	setQuietUntil(t, c, until)

	mkOverdueRepeatingTask(t, c, dueRepeatHour)
	newScannerAPI(t).ScanNotificationsOnce(context.Background())

	_, next := deliveryPlan(t, c.UserID, "due_soon")
	if wait := time.Until(next); wait < until-2*time.Minute {
		t.Fatalf("строка придержана лишь на %s, а окно тишины кончается через %s — премисса теста не выполнена",
			wait.Round(time.Minute), until)
	}
	return next
}

// TestTurningQuietHoursOffReleasesTheHeldQueue: выключенная тишина обязана
// отпустить то, что она же и придержала.
func TestTurningQuietHoursOffReleasesTheHeldQueue(t *testing.T) {
	c := signup(t)
	heldWarning(t, c, 8*time.Hour)

	setQuietOff(t, c)

	// Сравнение с запасом, а не с нулём: строка стала доступной, и рабочий
	// харнесса успевает её забрать и вернуть в повтор на минуту вперёд (приёмник
	// отвечает 500). Десять минут отделяют «отпущено» от «ждёт до утра».
	_, next := deliveryPlan(t, c.UserID, "due_soon")
	if wait := time.Until(next); wait > 10*time.Minute {
		t.Errorf("доставка по-прежнему ждёт %s (до %s UTC), хотя тихие часы выключены: очередь заморожена настройкой, которой больше нет",
			wait.Round(time.Minute), next.UTC().Format("15:04"))
	}
}

// TestShorteningQuietHoursMovesTheHeldQueueEarlier: конец окна сдвинули на
// раньше — придержанное обязано уехать за ним.
func TestShorteningQuietHoursMovesTheHeldQueueEarlier(t *testing.T) {
	c := signup(t)
	heldWarning(t, c, 8*time.Hour)

	want := setQuietUntil(t, c, 2*time.Hour)

	_, next := deliveryPlan(t, c.UserID, "due_soon")
	if next.Sub(want).Abs() > 2*time.Minute {
		t.Errorf("доставка назначена на %s UTC, а окно тишины теперь кончается в %s UTC: сокращение окна не дошло до уже отложенных строк",
			next.UTC().Format("15:04"), want.UTC().Format("15:04"))
	}
}

// TestExtendingQuietHoursHoldsTheQueueLonger: окно расширили — придержанное не
// имеет права выстрелить в старый конец, посреди только что объявленной тишины.
func TestExtendingQuietHoursHoldsTheQueueLonger(t *testing.T) {
	c := signup(t)
	old := heldWarning(t, c, 2*time.Hour)

	want := setQuietUntil(t, c, 6*time.Hour)

	_, next := deliveryPlan(t, c.UserID, "due_soon")
	if next.Before(want.Add(-2 * time.Minute)) {
		t.Errorf("доставка назначена на %s UTC (старый конец окна — %s UTC), а тишина теперь до %s UTC: сообщение придёт внутри объявленного молчания",
			next.UTC().Format("15:04"), old.UTC().Format("15:04"), want.UTC().Format("15:04"))
	}
}

// Контроль: перештамповка не имеет права трогать строку, которая уже отправлялась.
// Её next_attempt_at — не решение о тишине, а квадратичная отсрочка после
// неудачи; сбросить её к моменту постановки значит превратить лежащий приёмник
// в горячий цикл повторов.
func TestPrefsChangeKeepsARetryingDeliveryOnItsBackoff(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	chID := mkWebhookChannelTmpl(t, c, "Хук с лежащим приёмником", brokenHook(t), "")
	setQuietUntil(t, c, 8*time.Hour)

	delivID := backoffDelivery(t, c.UserID, s.WS, chID, 16*time.Minute)
	before := deliveryNextAttempt(t, delivID)

	setQuietOff(t, c)

	if after := deliveryNextAttempt(t, delivID); after.Sub(before).Abs() > time.Second {
		t.Errorf("отсрочка повтора уехала с %s UTC на %s UTC: смена настроек сбросила квадратичный бэкофф, и лежащий приёмник получит горячий цикл",
			before.UTC().Format("15:04:05"), after.UTC().Format("15:04:05"))
	}
}

// setQuietUntil включает тихие часы, начавшиеся пять минут назад и
// заканчивающиеся через after, и возвращает момент конца окна. Зона — явный
// UTC, чтобы тест считал те же минуты, что и сервер.
func setQuietUntil(t *testing.T, c *client, after time.Duration) time.Time {
	t.Helper()
	now := time.Now().UTC()
	start := now.Add(-5 * time.Minute)
	end := now.Add(after).Truncate(time.Minute)
	putPrefs(t, c, true, start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute())
	return end
}

// setQuietOff выключает тихие часы, оставляя границы окна нетронутыми — так это
// и делает тумблер на экране настроек.
func setQuietOff(t *testing.T, c *client) {
	t.Helper()
	now := time.Now().UTC()
	start := now.Add(-5 * time.Minute)
	putPrefs(t, c, false, start.Hour()*60+start.Minute(), start.Hour()*60+start.Minute()+480)
}

func putPrefs(t *testing.T, c *client, quiet bool, startMin, endMin int) {
	t.Helper()
	c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": true, "quiet_enabled": quiet,
		"quiet_start_minutes": startMin % 1440, "quiet_end_minutes": endMin % 1440,
		"quiet_tz": "UTC", "digest_minutes": 0,
	}), http.StatusOK)
}

// backoffDelivery кладёт строку аутбокса, уже пережившую несколько неудачных
// отправок: попытки потрачены, следующая назначена через in, а поставлена она
// была час назад (то есть перештамповка «от момента постановки» была бы видна
// сразу).
func backoffDelivery(t *testing.T, userID, wsID, chID string, in time.Duration) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, kind, text)
		 VALUES ($1, $2, 'comment', 'счёт из банка') RETURNING id`, userID, wsID).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	var delivID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, created_at, next_attempt_at)
		 VALUES ($1, $2, 'pending', 4, now() - interval '1 hour', now() + make_interval(secs => $3))
		 RETURNING id`, notifID, chID, in.Seconds()).Scan(&delivID); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	return delivID
}
