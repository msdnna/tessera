// Правило маршрутизации, чей канал ВЫКЛЮЧЕН, всё равно забирает уведомление
// себе и не отдаёт его правилу ниже. Это не та же история, что удалённый канал
// (route_without_channels_swallows_test.go): там правило остаётся с пустым
// списком, не выражает ничего и обязано пропустить ход, а здесь пользователь
// сам поставил доставку на паузу — и «всё остальное → почта», лежащее ниже,
// это адрес, которого он для напоминаний не выбирал.
//
// Асимметрия держится на одной строке роутера: `live` считает СУЩЕСТВУЮЩИЕ
// каналы правила, а `!ch.Enabled` отсеивается уже в цикле доставки. Свернуть
// одно в другое — однострочное «упрощение», после которого выключенный телеграм
// тихо разворачивает поток напоминаний в почту (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
)

func TestRouteWithDisabledChannelDoesNotRerouteBelow(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hookA, _ := countingHook(t)
	hookB, _ := countingHook(t)
	paused := mkWebhookChannelTmpl(t, c, "Телеграм-заменитель", hookA, "")
	below := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hookB, "")

	// Правило 1: напоминания — в первый канал. Правило 2: всё остальное — во второй.
	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{}`, []string{paused})
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{below})

	disableChannel(t, c, paused, hookA)
	fireReminder(ctx, t, c, "выпить таблетку")

	if n := deliveryCount(t, c.UserID, "reminder"); n != 0 {
		t.Fatalf("строк аутбокса %d, ожидалось 0 — выключенный канал перестал быть паузой", n)
	}
	if n := deliveryCountForChannel(t, c.UserID, "reminder", below); n != 0 {
		t.Fatalf("в канал правила НИЖЕ ушло %d строк: пауза на одном канале развернула поток в адрес, которого пользователь для напоминаний не выбирал", n)
	}
}

// Контроль: разница именно в выключателе. Та же пара правил с включённым
// каналом доставляет напоминание ровно один раз и именно в первый канал —
// иначе нулевой результат выше не значил бы ничего.
func TestRouteWithEnabledChannelStillDelivers(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hookA, _ := countingHook(t)
	hookB, _ := countingHook(t)
	live := mkWebhookChannelTmpl(t, c, "Телеграм-заменитель", hookA, "")
	below := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hookB, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{}`, []string{live})
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{below})

	fireReminder(ctx, t, c, "выпить таблетку")

	if n := deliveryCountForChannel(t, c.UserID, "reminder", live); n != 1 {
		t.Fatalf("в канал первого правила ушло %d строк, ожидалась 1", n)
	}
	if n := deliveryCountForChannel(t, c.UserID, "reminder", below); n != 0 {
		t.Fatalf("в канал правила ниже ушло %d строк, ожидалось 0 — first-match-wins сломан", n)
	}
}

// disableChannel гасит канал ручкой, а не UPDATE'ом в обход неё: выключатель —
// это тот же PATCH, что шлёт экран настроек (NotificationSettings.vue
// toggleChannel), со всем телом целиком.
func disableChannel(t *testing.T, c *client, id, url string) {
	t.Helper()
	ch := c.expect(t, c.patch("/notification-channels/"+id, map[string]any{
		"type": "webhook", "label": "Телеграм-заменитель",
		"config": map[string]any{"url": url}, "enabled": false,
	}), http.StatusOK)
	if ch["enabled"] != false {
		t.Fatalf("канал остался включённым: %v", ch["enabled"])
	}
}
