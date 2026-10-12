// Членство проверяется ОДИН раз — в deliverNotification, когда уведомление
// создаётся. Но строка аутбокса отправляется не тогда: тихие часы откладывают её
// до конца окна (ночью это часы), дайджест — до конца слота. В этот зазор
// участника исключают из воркспейса, и воркер всё равно уносит сообщение наружу.
// Мало того, templateData подставляет АКТУАЛЬНЫЙ заголовок задачи — то есть в
// почту бывшему коллеге уезжает текст, изменённый уже ПОСЛЕ исключения (#2932).
package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// titleTmpl рендерит только заголовок задачи, чтобы в теле вебхука было видно,
// какую именно версию названия унесла доставка.
const titleTmpl = "{{.TaskTitle}}"

func TestDeliveryStopsWhenMembershipEnded(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	s := mkStack(t, owner)
	mate := addMember(t, owner, s.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук бывшего участника", hook, titleTmpl)

	// Уведомление создано, пока участник был в воркспейсе, и отложено (тихие
	// часы/дайджест). Строка ждёт в аутбоксе.
	taskID := mkTask(t, owner, s.Board, s.col(t, 0), "Договор с клиентом")["id"].(string)
	delivID := pendingTaskDelivery(t, mate.UserID, s.WS, taskID, chID)

	// Пока строка ждёт, участника исключают — и задачу переименовывают.
	owner.expect(t, owner.del("/workspaces/"+s.WS+"/members/"+mate.UserID), http.StatusNoContent)
	renameTask(t, owner, taskID, "Договор с ACME на 12 млн")

	releaseAndDrain(t, delivID)

	if got := bodies(); len(got) != 0 {
		t.Fatalf("исключённому участнику всё-таки ушло %d сообщение(й): %q\n%s",
			len(got), strings.Join(got, " | "), dumpDeliveries(t))
	}
	st, lastErr := deliveryOutcome(t, delivID)
	if st != "failed" {
		t.Errorf("строка аутбокса: статус %q, want failed — доставка отменена, и аутбокс должен об этом сказать\n%s",
			st, dumpDeliveries(t))
	}
	if lastErr == "" {
		t.Error("строка аутбокса: last_error пуст — в истории канала нечего показать про отменённую доставку")
	}
}

// Контроль: та же отложенная строка, но участника не трогали. Доставка уходит и
// несёт актуальный заголовок — значит красное выше про исключение, а не про
// сломанный харнесс или шаблон.
func TestDeliveryStillGoesToMemberWhoStayed(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	s := mkStack(t, owner)
	mate := addMember(t, owner, s.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук участника", hook, titleTmpl)

	taskID := mkTask(t, owner, s.Board, s.col(t, 0), "Договор с клиентом")["id"].(string)
	delivID := pendingTaskDelivery(t, mate.UserID, s.WS, taskID, chID)
	renameTask(t, owner, taskID, "Договор с ACME на 12 млн")

	releaseAndDrain(t, delivID)

	got := bodies()
	if len(got) != 1 {
		t.Fatalf("участнику воркспейса ушло %d сообщение(й), want 1\n%s", len(got), dumpDeliveries(t))
	}
	if !strings.Contains(got[0], "ACME") {
		t.Errorf("доставка несёт %q — ожидался актуальный заголовок задачи", got[0])
	}
	if st, _ := deliveryOutcome(t, delivID); st != "sent" {
		t.Errorf("строка аутбокса: статус %q, want sent", st)
	}
}

// Контроль: сводный путь. Дайджест держит строки дольше всех, и ровно в его окне
// исключение застаёт доставку — свод не должен уйти ни одной строкой.
func TestDigestStopsWhenMembershipEnded(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	s := mkStack(t, owner)
	mate := addMember(t, owner, s.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук свода", hook, titleTmpl)

	taskID := mkTask(t, owner, s.Board, s.col(t, 0), "Договор с клиентом")["id"].(string)
	ids := []string{
		pendingTaskDeliveryIn(t, mate.UserID, s.WS, taskID, chID, "свод:слот-исключение"),
		pendingTaskDeliveryIn(t, mate.UserID, s.WS, taskID, chID, "свод:слот-исключение"),
	}
	owner.expect(t, owner.del("/workspaces/"+s.WS+"/members/"+mate.UserID), http.StatusNoContent)

	releaseAndDrain(t, ids...)

	if got := bodies(); len(got) != 0 {
		t.Fatalf("свод ушёл исключённому участнику (%d сообщение(й): %q)\n%s",
			len(got), strings.Join(got, " | "), dumpDeliveries(t))
	}
	for i, id := range ids {
		st, lastErr := deliveryOutcome(t, id)
		if st != "failed" || lastErr == "" {
			t.Errorf("строка %d свода: статус %q, last_error %q, ожидалось failed с текстом причины", i+1, st, lastErr)
		}
	}
}

// recordingHook — вебхук-приёмник, сохраняющий тела доставленных сообщений.
func recordingHook(t *testing.T) (url string, got func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			seen = append(seen, string(b))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func renameTask(t *testing.T, c *client, taskID, title string) {
	t.Helper()
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{"title": title}), http.StatusOK)
}

// pendingTaskDelivery кладёт уведомление о задаче и готовую к отправке строку
// аутбокса — состояние, в котором доставку застают тихие часы или окно дайджеста.
// Пишется напрямую: отложенную строку не выдаёт ни одна ручка.
func pendingTaskDelivery(t *testing.T, userID, wsID, taskID, chID string) string {
	t.Helper()
	return pendingTaskDeliveryIn(t, userID, wsID, taskID, chID, "")
}

func pendingTaskDeliveryIn(t *testing.T, userID, wsID, taskID, chID, group string) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, task_id, kind, text)
		 VALUES ($1, $2, $3, 'comment', 'новый комментарий') RETURNING id`,
		userID, wsID, taskID).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	var delivID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, next_attempt_at, digest_group)
		 VALUES ($1, $2, 'pending', 0, now() + interval '1 hour', $3) RETURNING id`,
		notifID, chID, group).Scan(&delivID); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	return delivID
}

// releaseAndDrain makes held deliveries due and drains the outbox until they
// settle. Held at insert (pendingTaskDelivery) so the harness's own background
// worker can't send them before the test has changed the world under them — it
// used to, on a fast enough runner, between the insert and the transfer.
func releaseAndDrain(t *testing.T, ids ...string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE notification_deliveries SET next_attempt_at = now() - interval '1 minute' WHERE id = ANY($1::uuid[])`,
		ids); err != nil {
		t.Fatalf("release deliveries: %v", err)
	}
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })
}
