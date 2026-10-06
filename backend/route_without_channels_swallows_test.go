// Удаление канала вычищает его id из правил маршрутизации
// (StripChannelFromRoutes) — иначе правило заклинивает на экране настроек. Но
// правило, у которого список каналов стал ПУСТЫМ, никуда не девается: список
// first-match-wins, оно по-прежнему матчит первым, доставлять ему нечем — и
// нижние правила уже не спрашивают. Пользователь удалил телеграм, а перестали
// приходить упоминания ВООБЩЕ, хотя ниже лежит «всё остальное → почта» (#2932).
//
// Пустой список сам по себе — не «заглушить»: и web (NotificationSettings.vue),
// и Android (RouteEdit.toRequest) пишут заглушку явным options.mute, а экран
// рисует бейдж «заглушено» по нему же. Правило без каналов и без mute не
// выражает ничего — и не вправе съедать уведомление.
package main

import (
	"context"
	"net/http"
	"testing"
)

// Находка: правило, осиротевшее после удаления канала, съедает уведомление,
// которое досталось бы правилу ниже.
func TestRouteOrphanedByChannelDeleteDoesNotSwallow(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hookA, _ := countingHook(t)
	hookB, _ := countingHook(t)
	gone := mkWebhookChannelTmpl(t, c, "Телеграм-заменитель", hookA, "")
	kept := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hookB, "")

	// Правило 1: напоминания — в первый канал. Правило 2: всё остальное — во второй.
	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{}`, []string{gone})
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{kept})

	// Пользователь удаляет первый канал. Ручка вычищает его id из правила 1.
	c.expect(t, c.del("/notification-channels/"+gone), http.StatusNoContent)

	fireReminder(ctx, t, c, "выпить таблетку")

	if n := deliveryCount(t, c.UserID, "reminder"); n == 0 {
		t.Fatalf("напоминание не доехало никуда: правило без каналов съело его, хотя ниже лежит catch-all")
	}
}

// Та же дыра без удаления: правило, сохранённое до выбора каналов. Проверяется
// отдельно, потому что чинится оно в маршрутизаторе, а не в ручке удаления.
func TestRouteWithNoChannelsFallsThrough(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hook, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "живое напоминание")

	if n := deliveryCount(t, c.UserID, "reminder"); n == 0 {
		t.Fatalf("напоминание не доехало никуда: пустое правило съело его, хотя ниже лежит catch-all")
	}
}

// Контроль A: заглушка, написанная как заглушка (options.mute), по-прежнему
// глушит — иначе «пропускать правило без каналов» починило бы находку ценой
// самой фичи mute, которая как раз и хранит пустой список.
func TestExplicitMuteWithoutChannelsStillMutes(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hook, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{"mute":true}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "заглушённое напоминание")

	if n := deliveryCount(t, c.UserID, "reminder"); n != 0 {
		t.Fatalf("строк аутбокса %d, ожидалось 0 — явная заглушка перестала глушить", n)
	}
}

// Контроль B: first-match-wins не размывается. Правило с живым каналом
// по-прежнему забирает уведомление себе, и catch-all ниже вторую строку не
// заводит.
func TestRouteWithLiveChannelStillWinsFirst(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hookA, _ := countingHook(t)
	hookB, _ := countingHook(t)
	first := mkWebhookChannelTmpl(t, c, "Первый", hookA, "")
	second := mkWebhookChannelTmpl(t, c, "Второй", hookB, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{}`, []string{first})
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{second})

	fireReminder(ctx, t, c, "обычное напоминание")

	if n := deliveryCount(t, c.UserID, "reminder"); n != 1 {
		t.Fatalf("строк аутбокса %d, ожидалась 1 — first-match-wins сломан", n)
	}
	if n := deliveryCountForChannel(t, c.UserID, "reminder", first); n != 1 {
		t.Fatalf("в канал первого правила ушло %d строк, ожидалась 1", n)
	}
}

// deliveryCountForChannel — сколько строк аутбокса такого вида заведено именно
// на этот канал.
func deliveryCountForChannel(t *testing.T, userID, kind, channelID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*)
		   FROM notification_deliveries d
		   JOIN notifications n ON n.id = d.notification_id
		  WHERE n.user_id = $1 AND n.kind = $2 AND d.channel_id = $3`,
		userID, kind, channelID).Scan(&n); err != nil {
		t.Fatalf("посчитать строки аутбокса канала: %v", err)
	}
	return n
}
