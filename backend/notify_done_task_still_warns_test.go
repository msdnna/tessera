// Сканер сроков намеренно не смотрит на закрытые и убранные в архив задачи:
// ListDueTasksForScan отбирает только `completed_at IS NULL AND archived_at IS
// NULL`. Но отбор этот делается РОВНО ОДИН РАЗ — когда уведомление создаётся.
// Строка аутбокса уезжает не сразу: тихие часы придерживают её до конца окна,
// это целая ночь. В зазоре пользователь успевает сделать ровно то, ради чего
// предупреждение и было послано, — закрыть задачу или убрать её в архив, — а
// утром всё равно получает письмо «срок подходит» о задаче, которую вчера
// доделал.
//
// Та же дверь, что у участия в пространстве и у тумблера уведомлений: она
// открывается не устаревшей ссылкой, а временем. Здесь пересматривается гвард
// самого сканера (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
)

// TestClosingTheTaskOvernightDropsTheHeldDueDelivery: задача закрыта до того,
// как тишина отпустила письмо о её сроке, — письму больше нечего сообщать.
func TestClosingTheTaskOvernightDropsTheHeldDueDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	_, taskID := mkOverdueTaskIn(t, c)
	delivID, hits := heldDueDelivery(t, c)

	closeTask(t, c, taskID) // 23:30: «сделал, больше не напоминай»

	assertHeldDeliveryDropped(t, delivID, hits, "задачу закрыли ещё ночью")
}

// TestArchivingTheTaskOvernightDropsTheHeldDueDelivery: тот же гвард сканера,
// вторая его половина — карточка убрана в архив.
func TestArchivingTheTaskOvernightDropsTheHeldDueDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	_, taskID := mkOverdueTaskIn(t, c)
	delivID, hits := heldDueDelivery(t, c)

	c.expect(t, c.patch("/tasks/"+taskID+"/archive", nil), http.StatusNoContent)

	assertHeldDeliveryDropped(t, delivID, hits, "задачу убрали в архив ещё ночью")
}

// Контроль: проверяется состояние на момент отправки, а не «закрывалась ли
// когда-нибудь». Закрыл ночью, передумал и открыл обратно — предупреждение о
// сроке обязано приехать.
func TestReopenedTaskKeepsItsHeldDueDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	_, taskID := mkOverdueTaskIn(t, c)
	delivID, hits := heldDueDelivery(t, c)

	closeTask(t, c, taskID)
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{"completed": false}), http.StatusOK)

	morningCame(t, delivID)
	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st != "sent" {
		t.Errorf("строка кончила статусом %q вместо sent: задачу открыли обратно, а пересмотр на отправке помнит только, что её когда-то закрывали", st)
	}
	if n := hits(); n == 0 {
		t.Error("приёмник не получил ничего: задачу открыли обратно, а письмо о сроке всё равно потерялось")
	}
}

// Контроль: гвард сканера — это гвард сроков, а не общий выключатель. У
// закрытой задачи остаётся обсуждение, и уведомление о нём обязано уехать.
func TestClosedTaskStillDeliversItsOtherNotifications(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	s, taskID := mkOverdueTaskIn(t, c)
	closeTask(t, c, taskID)

	hook, hits := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук обсуждения", hook, "")
	delivID := readyTaskDelivery(t, c.UserID, s.WS, taskID, chID, "comment")

	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st != "sent" {
		t.Errorf("комментарий кончил статусом %q вместо sent: закрытая задача стала общим выключателем", st)
	}
	if n := hits(); n == 0 {
		t.Error("приёмник не получил комментарий: закрытая задача погасила и чужой вид уведомлений")
	}
}

// closeTask закрывает задачу так же, как это делает галочка на карточке.
func closeTask(t *testing.T, c *client, taskID string) {
	t.Helper()
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{"completed": true}), http.StatusOK)
}

// readyTaskDelivery — readyDelivery для уведомления, привязанного к задаче:
// контролю важно, что строка указывает именно на закрытую карточку.
func readyTaskDelivery(t *testing.T, userID, wsID, taskID, chID, kind string) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, task_id, kind, text)
		 VALUES ($1, $2, $3, $4, 'кто-то ответил в обсуждении') RETURNING id`,
		userID, wsID, taskID, kind).Scan(&notifID); err != nil {
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
