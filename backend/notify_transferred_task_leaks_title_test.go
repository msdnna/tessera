// Третья дверь в ту же щель, что исключение участника и выключенный ночью
// тумблер: гвард доставки спрашивает про воркспейс, ЗАПИСАННЫЙ в уведомлении, а
// задача за ночь успевает уехать из него. TransferProject перештамповывает шесть
// видов межпространственных связей (отношения, вехи, ссылки на документы, теги,
// вложенность документов, конференции) — уведомления в этом списке не значатся.
//
// Строка аутбокса хранит workspace_id источника, адресат из него никуда не
// уходил, членство сходится — и templateData подставляет АКТУАЛЬНЫЙ заголовок
// задачи, живущей теперь в пространстве, куда адресата не звали. Ровно тот
// вынос чужих правок наружу, который reminderScope отказывается нести (#2932).
package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestTransferredTaskDropsTheHeldDelivery: пока строка ждала конца тишины,
// проект переехал в пространство, где адресата нет. Отправлять нечего.
func TestTransferredTaskDropsTheHeldDelivery(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	src := mkStack(t, owner)
	mate := addMember(t, owner, src.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук уехавшей задачи", hook, titleTmpl)

	taskID := mkTask(t, owner, src.Board, src.col(t, 0), "Договор с клиентом")["id"].(string)
	delivID := pendingTaskDelivery(t, mate.UserID, src.WS, taskID, chID)

	// Пока строка ждёт, проект уносят в соседнее пространство — и задачу там
	// переименовывают. Адресата в это пространство не приглашали.
	away := mkWorkspace(t, owner, "Другое пространство "+t.Name())
	transferProject(t, owner, src.Project, away)
	renameTask(t, owner, taskID, "Договор с ACME на 12 млн")

	drainDeliveriesUntil(t, func() bool { return allSettled(t, []string{delivID}) })

	if got := bodies(); len(got) != 0 {
		t.Fatalf("ушло %d сообщение(й) про задачу из чужого пространства: %q\n%s",
			len(got), strings.Join(got, " | "), dumpDeliveries(t))
	}
	st, lastErr := deliveryOutcome(t, delivID)
	if st != "failed" {
		t.Errorf("строка аутбокса: статус %q, want failed — задача уехала, и аутбокс должен об этом сказать\n%s",
			st, dumpDeliveries(t))
	}
	if lastErr == "" {
		t.Error("строка аутбокса: last_error пуст — в истории канала нечего показать про отменённую доставку")
	}
}

// Контроль: переезд сам по себе доставку не отменяет. Задача уехала в
// пространство, где адресат тоже состоит, — письмо обязано прийти и принести
// актуальный заголовок. Красное выше — про потерю доступа, а не про переезд.
func TestTransferWithinReachKeepsTheHeldDelivery(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	src := mkStack(t, owner)
	mate := addMember(t, owner, src.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук переезда по соседству", hook, titleTmpl)

	taskID := mkTask(t, owner, src.Board, src.col(t, 0), "Договор с клиентом")["id"].(string)
	delivID := pendingTaskDelivery(t, mate.UserID, src.WS, taskID, chID)

	near := mkWorkspace(t, owner, "Соседнее пространство "+t.Name())
	addMemberTo(t, owner, near, mate)
	transferProject(t, owner, src.Project, near)
	renameTask(t, owner, taskID, "Договор с ACME на 12 млн")

	drainDeliveriesUntil(t, func() bool { return allSettled(t, []string{delivID}) })

	got := bodies()
	if len(got) != 1 {
		t.Fatalf("участнику обоих пространств ушло %d сообщение(й), want 1\n%s", len(got), dumpDeliveries(t))
	}
	if !strings.Contains(got[0], "ACME") {
		t.Errorf("доставка несёт %q — ожидался актуальный заголовок задачи", got[0])
	}
	if st, _ := deliveryOutcome(t, delivID); st != "sent" {
		t.Errorf("строка аутбокса: статус %q, want sent", st)
	}
}

// Контроль и одновременно вторая половина фикса: у напоминания текст —
// собственный текст пользователя, и будильник обязан прозвонить, даже если
// задача, к которой напоминание было привязано, уехала. Уносить с собой её
// нынешний заголовок он при этом не должен — ровно та развилка, которую
// reminderScope уже делает на сканировании.
func TestTransferredTaskStillRingsTheReminderWithoutItsTitle(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	src := mkStack(t, owner)
	mate := addMember(t, owner, src.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук напоминания", hook, titleTmpl)

	taskID := mkTask(t, owner, src.Board, src.col(t, 0), "Договор с клиентом")["id"].(string)
	delivID := pendingReminderDelivery(t, mate.UserID, src.WS, taskID, chID)

	away := mkWorkspace(t, owner, "Другое пространство "+t.Name())
	transferProject(t, owner, src.Project, away)
	renameTask(t, owner, taskID, "Договор с ACME на 12 млн")

	drainDeliveriesUntil(t, func() bool { return allSettled(t, []string{delivID}) })

	got := bodies()
	if len(got) != 1 {
		t.Fatalf("напоминание ушло %d раз(а), want 1 — собственный будильник пользователя потерялся из-за чужого переезда\n%s",
			len(got), dumpDeliveries(t))
	}
	if strings.Contains(got[0], "ACME") {
		t.Errorf("напоминание унесло заголовок уехавшей задачи: %q", got[0])
	}
	if st, _ := deliveryOutcome(t, delivID); st != "sent" {
		t.Errorf("строка аутбокса: статус %q, want sent", st)
	}
}

// Контроль: новый гвард — про задачу, а не про воркспейс вообще. Уведомление
// без задачи спрашивать не о чем, и оно обязано уехать как прежде.
func TestTaskLessDeliveryIgnoresTheTaskReachGate(t *testing.T) {
	hook, bodies := recordingHook(t)

	owner := signup(t)
	src := mkStack(t, owner)
	mate := addMember(t, owner, src.WS)
	chID := mkWebhookChannelTmpl(t, mate, "Хук без задачи", hook, "")

	delivID := readyDelivery(t, mate.UserID, src.WS, chID, "comment")

	away := mkWorkspace(t, owner, "Другое пространство "+t.Name())
	transferProject(t, owner, src.Project, away)

	drainDeliveriesUntil(t, func() bool { return allSettled(t, []string{delivID}) })

	if got := bodies(); len(got) != 1 {
		t.Fatalf("уведомление без задачи ушло %d раз(а), want 1 — гвард задачи стал общим выключателем\n%s",
			len(got), dumpDeliveries(t))
	}
	if st, _ := deliveryOutcome(t, delivID); st != "sent" {
		t.Errorf("строка аутбокса: статус %q, want sent", st)
	}
}

// transferProject уносит проект в другое пространство той же ручкой, что и UI.
func transferProject(t *testing.T, owner *client, projectID, wsID string) {
	t.Helper()
	owner.expect(t, owner.post("/projects/"+projectID+"/transfer",
		map[string]any{"workspace_id": wsID}), http.StatusOK)
}

// addMemberTo зовёт уже существующего пользователя во второе пространство:
// addMember регистрирует нового, а контролю нужен тот же самый адресат.
func addMemberTo(t *testing.T, owner *client, wsID string, member *client) {
	t.Helper()
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": member.Email}), http.StatusCreated)
}

// pendingReminderDelivery — pendingTaskDelivery для напоминания: текст
// собственный, но задача названа, и именно её заголовок доставка не должна
// унести после переезда.
func pendingReminderDelivery(t *testing.T, userID, wsID, taskID, chID string) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, task_id, kind, text)
		 VALUES ($1, $2, $3, 'reminder', 'напоминание: выпить таблетку') RETURNING id`,
		userID, wsID, taskID).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	var delivID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, next_attempt_at)
		 VALUES ($1, $2, 'pending', 0, now() - interval '1 minute') RETURNING id`,
		notifID, chID).Scan(&delivID); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	return delivID
}
