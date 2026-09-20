// Тихие часы — обещание молчания: с 22:00 до 08:00 ни письма, ни телеграма, ни
// пуша. Обещание проверяется РОВНО ОДИН РАЗ — при постановке строки в аутбокс
// (routeNotification сдвигает доставку на конец окна). Дальше строка живёт своей
// жизнью: неудачная отправка считает next = now + attempts² минут и кладёт
// повтор куда придётся. Доставка, созданная в 21:59, падает (приёмник лежит) —
// и повторы 22:00, 22:04, 22:13, 22:29, 22:54 уже ВНУТРИ объявленной тишины:
// как только вебхук оживает, сообщение приходит ночью. Для device-канала это
// ещё заметнее: в тихие часы строка не заводится вовсе, а ретрай заносит её
// туда и телефон звонит (#2932).
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// quietSpan — длина окна тишины в этих тестах: ночь, как у настроек по
// умолчанию (22:00–08:00).
const quietSpan = 8 * time.Hour

// TestRetryIsHeldUntilQuietHoursEnd: повтор, попадающий в окно тишины, обязан
// ждать конца окна, а не звонить среди ночи.
func TestRetryIsHeldUntilQuietHoursEnd(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	chID := mkWebhookChannelTmpl(t, c, "Хук с лежащим приёмником", brokenHook(t), "")
	// Тишина начинается через пять минут: строка ставится ещё «в голосе», а
	// повтор (16 минут при четвёртой попытке) приходится уже на окно.
	startMin, endMin := setQuietWindowIn(t, c, 5*time.Minute)

	delivID := failingDelivery(t, c.UserID, s.WS, chID, "счёт из банка", 3)
	drainDeliveriesUntil(t, func() bool { return deliveryAttempts(t, delivID) >= 4 })

	next := deliveryNextAttempt(t, delivID)
	if inQuietUTC(next, startMin, endMin) {
		t.Errorf("повтор назначен на %s — это внутри объявленной тишины (%s); молчание обещано, а сообщение придёт ночью",
			next.UTC().Format("15:04"), quietLabel(startMin, endMin))
	}
}

// Контроль: тихие часы не должны превращаться в общий тормоз. Окно, до которого
// повтору далеко, обязан оставить квадратичную отсрочку как есть.
func TestRetryOutsideQuietHoursKeepsItsBackoff(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	chID := mkWebhookChannelTmpl(t, c, "Хук с лежащим приёмником", brokenHook(t), "")
	// Тишина — только через три часа; повтор через 16 минут её не касается.
	setQuietWindowIn(t, c, 3*time.Hour)

	delivID := failingDelivery(t, c.UserID, s.WS, chID, "счёт из банка", 3)
	drainDeliveriesUntil(t, func() bool { return deliveryAttempts(t, delivID) >= 4 })

	switch wait := time.Until(deliveryNextAttempt(t, delivID)); {
	case wait > 30*time.Minute:
		t.Errorf("повтор отложен на %s — далёкое окно тишины задержало доставку, которой оно не касается", wait.Round(time.Second))
	case wait < 10*time.Minute:
		t.Errorf("повтор назначен через %s, want ~16 минут: отсрочка перестала расти", wait.Round(time.Second))
	}
}

// brokenHook — приёмник, который всегда отвечает 500: ошибка временная, строка
// уходит в повтор (а не в permanent, как было бы на 4xx).
func brokenHook(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// setQuietWindowIn включает тихие часы, начинающиеся через after, и возвращает
// границы окна в минутах от полуночи UTC. Зона задаётся явным UTC, чтобы тест
// считал те же минуты, что и сервер.
func setQuietWindowIn(t *testing.T, c *client, after time.Duration) (int, int) {
	t.Helper()
	now := time.Now().UTC()
	startMin := (now.Hour()*60 + now.Minute() + int(after.Minutes())) % 1440
	endMin := (startMin + int(quietSpan.Minutes())) % 1440
	c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": true, "quiet_enabled": true,
		"quiet_start_minutes": startMin, "quiet_end_minutes": endMin, "quiet_tz": "UTC",
		"digest_minutes": 0,
	}), http.StatusOK)
	return startMin, endMin
}

// inQuietUTC повторяет правило сервера (quietWindow), включая переход через
// полночь: окно 22:00–08:00 — это start > end.
func inQuietUTC(at time.Time, startMin, endMin int) bool {
	mins := at.UTC().Hour()*60 + at.UTC().Minute()
	if startMin < endMin {
		return mins >= startMin && mins < endMin
	}
	return mins >= startMin || mins < endMin
}

func quietLabel(startMin, endMin int) string {
	hhmm := func(m int) string {
		return time.Date(2000, 1, 1, 0, m, 0, 0, time.UTC).Format("15:04")
	}
	return hhmm(startMin) + "–" + hhmm(endMin) + " UTC"
}

// failingDelivery кладёт уведомление и готовую к отправке строку аутбокса с уже
// потраченными попытками: claim добавит свою, и отсрочка следующего повтора
// вырастет до шестнадцати минут (attempts² при четвёртой попытке). Пятая — уже
// предел (maxDeliveryAttempts), там строка не повторяется, а отбрасывается.
func failingDelivery(t *testing.T, userID, wsID, chID, text string, attempts int) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, kind, text)
		 VALUES ($1, $2, 'comment', $3) RETURNING id`, userID, wsID, text).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	var delivID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, next_attempt_at)
		 VALUES ($1, $2, 'pending', $3, now() - interval '1 minute') RETURNING id`,
		notifID, chID, attempts).Scan(&delivID); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	return delivID
}

func deliveryAttempts(t *testing.T, delivID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT attempts FROM notification_deliveries WHERE id = $1`, delivID).Scan(&n); err != nil {
		t.Fatalf("read delivery attempts: %v", err)
	}
	return n
}

func deliveryNextAttempt(t *testing.T, delivID string) time.Time {
	t.Helper()
	var at time.Time
	if err := testPool.QueryRow(context.Background(),
		`SELECT next_attempt_at FROM notification_deliveries WHERE id = $1`, delivID).Scan(&at); err != nil {
		t.Fatalf("read delivery next_attempt_at: %v", err)
	}
	return at
}
