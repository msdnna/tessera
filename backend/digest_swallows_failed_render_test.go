// Шаблон канала проверяется один раз — при сохранении, против notify.SampleData,
// где заполнено каждое поле. Настоящее уведомление беднее образца: напоминание
// приходит без задачи, и шаблон, который что-то делает с .TaskTitle, падает
// именно на нём. Одиночная доставка это падение записывает — deliverOne
// возвращает notify.Permanent("template render"), строка становится 'failed' с
// текстом ошибки, и пользователь видит в истории канала, что шаблон сломан.
// Дайджест ту же самую строку выбрасывает из свода молча: deliverGroup её
// пропускает, а settleDelivery раздаёт всем строкам группы один и тот же исход
// отправки. Ничего не ушло — в аутбоксе 'sent' без ошибки, и канал выглядит
// здоровым (#2932).
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// tasklessBreakingTmpl проходит validateTemplate (у образца заголовок задачи
// длинный) и падает на уведомлении без задачи, где .TaskTitle пуст.
const tasklessBreakingTmpl = "{{slice .TaskTitle 0 4}}"

func TestDigestReportsRowItCouldNotRender(t *testing.T) {
	hook, posts := countingHook(t)

	c := signup(t)
	wsID := mkWorkspace(t, c, "Дайджест: непрорисованная строка")
	// Форма приняла шаблон — в глазах пользователя канал здоров.
	chID := mkWebhookChannelTmpl(t, c, "Хук дайджеста", hook, tasklessBreakingTmpl)

	ids := pendingDeliveries(t, c.UserID, wsID, chID, "дайджест:слот-1",
		"первое напоминание без задачи", "второе напоминание без задачи")
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	if got := posts(); got != 0 {
		t.Fatalf("свод ушёл %d раз(а), хотя отрисовать не удалось ни одну строку", got)
	}
	for i, id := range ids {
		st, lastErr := deliveryOutcome(t, id)
		if st != "failed" {
			t.Errorf("строка %d свода: статус %q при пустом своде — доставки не было, а аутбокс о ней молчит\n%s",
				i+1, st, dumpDeliveries(t))
		}
		if lastErr == "" {
			t.Errorf("строка %d свода: last_error пуст — в истории канала нечего показать про сломанный шаблон", i+1)
		}
	}
}

// Контроль: та же неотрисовываемая пара, но без дайджеста. Одиночный путь
// сообщает о поломке — значит шаблон действительно падает на этих данных, и
// расхождение выше про свод, а не про шаблон.
func TestSingleDeliveryReportsRowItCouldNotRender(t *testing.T) {
	hook, posts := countingHook(t)

	c := signup(t)
	wsID := mkWorkspace(t, c, "Одиночная: непрорисованная строка")
	chID := mkWebhookChannelTmpl(t, c, "Хук одиночный", hook, tasklessBreakingTmpl)

	ids := pendingDeliveries(t, c.UserID, wsID, chID, "", "напоминание без задачи")
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	if got := posts(); got != 0 {
		t.Fatalf("сломанный шаблон всё-таки что-то отправил (%d POST)", got)
	}
	st, lastErr := deliveryOutcome(t, ids[0])
	if st != "failed" || lastErr == "" {
		t.Fatalf("одиночная доставка: статус %q, last_error %q, ожидалось failed с текстом ошибки\n%s",
			st, lastErr, dumpDeliveries(t))
	}
}

// Контроль: исправный шаблон — свод по-прежнему складывается в одно сообщение,
// и обе строки закрываются как отправленные.
func TestDigestStillCombinesRenderableRows(t *testing.T) {
	hook, posts := countingHook(t)

	c := signup(t)
	wsID := mkWorkspace(t, c, "Дайджест: исправный шаблон")
	chID := mkWebhookChannelTmpl(t, c, "Хук исправный", hook, "")

	ids := pendingDeliveries(t, c.UserID, wsID, chID, "дайджест:слот-2",
		"первое уведомление", "второе уведомление")
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	if got := posts(); got != 1 {
		t.Fatalf("две строки одного слота ушли %d сообщением(ями), want 1 свод\n%s", got, dumpDeliveries(t))
	}
	for i, id := range ids {
		if st, _ := deliveryOutcome(t, id); st != "sent" {
			t.Errorf("строка %d исправного свода: статус %q, want sent", i+1, st)
		}
	}
}

// countingHook — вебхук-приёмник, считающий доставленные сообщения.
func countingHook(t *testing.T) (url string, count func() int) {
	t.Helper()
	var mu sync.Mutex
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			posts++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() int {
		mu.Lock()
		defer mu.Unlock()
		return posts
	}
}

// mkWebhookChannelTmpl — mkWebhookChannel со своим шаблоном сообщения (пустой =
// шаблон по умолчанию).
func mkWebhookChannelTmpl(t *testing.T, c *client, label, url, tmpl string) string {
	t.Helper()
	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": label, "template": tmpl,
		"config": map[string]any{"url": url},
	}), http.StatusCreated)
	return ch["id"].(string)
}

// pendingDeliveries кладёт уведомления без задачи и готовые к отправке строки
// аутбокса одним запросом: группа дайджеста собирается только из того, что один
// drain забрал одним claim'ом, поэтому строки не должны успеть разъехаться по
// разным тикам фонового воркера. group="" — обычные, не сводные строки.
func pendingDeliveries(t *testing.T, userID, wsID, chID, group string, texts ...string) []string {
	t.Helper()
	ctx := context.Background()
	ids := make([]string, 0, len(texts))
	notifIDs := make([]string, 0, len(texts))
	for _, text := range texts {
		var notifID string
		if err := testPool.QueryRow(ctx,
			`INSERT INTO notifications (user_id, workspace_id, kind, text)
			 VALUES ($1, $2, 'reminder', $3) RETURNING id`, userID, wsID, text).Scan(&notifID); err != nil {
			t.Fatalf("insert notification: %v", err)
		}
		notifIDs = append(notifIDs, notifID)
	}
	rows, err := testPool.Query(ctx,
		`INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, next_attempt_at, digest_group)
		 SELECT n, $2, 'pending', 0, now() - interval '1 minute', $3 FROM unnest($1::uuid[]) AS n
		 RETURNING id`, notifIDs, chID, group)
	if err != nil {
		t.Fatalf("insert deliveries: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan delivery id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("insert deliveries: %v", err)
	}
	if len(ids) != len(texts) {
		t.Fatalf("вставлено %d строк аутбокса, ожидалось %d", len(ids), len(texts))
	}
	return ids
}

// allSettled — все строки вышли из очереди (отправлены или отброшены).
func allSettled(t *testing.T, ids []string) bool {
	t.Helper()
	for _, id := range ids {
		if st, _ := deliveryOutcome(t, id); st != "sent" && st != "failed" {
			return false
		}
	}
	return true
}

// deliveryOutcome — статус строки аутбокса и записанная по ней ошибка.
func deliveryOutcome(t *testing.T, delivID string) (string, string) {
	t.Helper()
	var st, lastErr string
	if err := testPool.QueryRow(context.Background(),
		`SELECT status, last_error FROM notification_deliveries WHERE id = $1`, delivID).Scan(&st, &lastErr); err != nil {
		t.Fatalf("read delivery outcome: %v", err)
	}
	return st, lastErr
}
