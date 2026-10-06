// mail.noop логирует тело письма и возвращает nil — на self-host без SMTP лог
// единственное место, где всплывает ссылка подтверждения или сброса пароля, и
// для почты аутентификации это верно. Канал доставки идёт через тот же
// mailer.Send, а у строки аутбокса такого читателя нет: settleDelivery видит
// nil и закрывает строку как 'sent'. В истории канала сплошь «доставлено»,
// хотя из сервера не вышло ни одного письма. Тот же обман и у пробы: /test
// получает nil и ставит verified — зелёная метка «проверен» на канале, который
// провабельно не может доставить (#2932).
package main

import (
	"net/http"
	"strings"
	"testing"
)

// mkEmailChannel заводит почтовый канал на адрес самого пользователя.
func mkEmailChannel(t *testing.T, c *client, label string) string {
	t.Helper()
	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "email", "label": label, "config": map[string]any{"address": c.Email},
	}), http.StatusCreated)
	return ch["id"].(string)
}

// probeEmailChannel шлёт пробу и возвращает предупреждение ответа. Оно же —
// признак сборки без SMTP: с настроенным SMTP проба уходит по-настоящему и
// предупреждать не о чем.
func probeEmailChannel(t *testing.T, c *client, chID string) string {
	t.Helper()
	res := c.expect(t, c.post("/notification-channels/"+chID+"/test", nil), http.StatusOK)
	w, _ := res["warning"].(string)
	return w
}

// channelVerified читает метку «проверен» со списка каналов — ровно то, что
// клиент рисует на строке канала.
func channelVerified(t *testing.T, c *client, chID string) bool {
	t.Helper()
	for _, ch := range c.get("/notification-channels").listBody(t) {
		if ch["id"] == chID {
			v, _ := ch["verified"].(bool)
			return v
		}
	}
	t.Fatalf("канал %s пропал из списка", chID)
	return false
}

// Находка: без SMTP строка аутбокса закрывается как отправленная.
func TestEmailDeliveryWithoutSMTPIsNotRecordedAsSent(t *testing.T) {
	c := signup(t)
	wsID := mkWorkspace(t, c, "Почта без SMTP: аутбокс")
	chID := mkEmailChannel(t, c, "Почта")
	if probeEmailChannel(t, c, chID) == "" {
		t.Skip("в этой сборке SMTP настроен — письму есть куда уйти")
	}

	ids := pendingDeliveries(t, c.UserID, wsID, chID, "", "напоминание, которое некуда отправить")
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	st, lastErr := deliveryOutcome(t, ids[0])
	if st == "sent" {
		t.Fatalf("строка закрыта как отправленная, хотя SMTP не настроен и письмо никуда не ушло\n%s",
			dumpDeliveries(t))
	}
	if st != "failed" {
		t.Fatalf("статус строки %q, ожидалось failed\n%s", st, dumpDeliveries(t))
	}
	if !strings.Contains(lastErr, "smtp") {
		t.Errorf("last_error = %q — в истории канала нечего показать про ненастроенный SMTP", lastErr)
	}
}

// Находка: та же ложь у пробы — метка «проверен» за отправку, которой не было.
func TestEmailProbeWithoutSMTPDoesNotVerifyChannel(t *testing.T) {
	c := signup(t)
	chID := mkEmailChannel(t, c, "Почта")
	if probeEmailChannel(t, c, chID) == "" {
		t.Skip("в этой сборке SMTP настроен — проба уходит по-настоящему")
	}
	if channelVerified(t, c, chID) {
		t.Fatalf("канал носит метку «проверен», хотя проба не вышла за пределы сервера")
	}
}

// Контроль: исправный транспорт по-прежнему закрывает строку как отправленную —
// значит выше расхождение про ненастроенный SMTP, а не про сломанный drain.
func TestWebhookDeliveryStillSettlesSent(t *testing.T) {
	hook, posts := countingHook(t)

	c := signup(t)
	wsID := mkWorkspace(t, c, "Контроль: живой вебхук")
	chID := mkWebhookChannelTmpl(t, c, "Хук", hook, "")

	ids := pendingDeliveries(t, c.UserID, wsID, chID, "", "напоминание, которому есть куда уйти")
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	if st, _ := deliveryOutcome(t, ids[0]); st != "sent" {
		t.Fatalf("статус живой доставки %q, ожидалось sent\n%s", st, dumpDeliveries(t))
	}
	if got := posts(); got != 1 {
		t.Errorf("вебхук получил %d POST, ожидался 1", got)
	}
}

// Контроль: проба по-прежнему отвечает сентинелом, который оба клиента
// переводят сами (TestSMTPWarningIsASentinel пришпиливает его текст) — метка
// снимается, а контракт ответа остаётся прежним.
func TestEmailProbeWithoutSMTPStillWarns(t *testing.T) {
	c := signup(t)
	chID := mkEmailChannel(t, c, "Почта")
	w := probeEmailChannel(t, c, chID)
	if w == "" {
		t.Skip("в этой сборке SMTP настроен — предупреждать не о чем")
	}
	if w != "smtp is not configured" {
		t.Errorf("warning = %q, want %q", w, "smtp is not configured")
	}
}
