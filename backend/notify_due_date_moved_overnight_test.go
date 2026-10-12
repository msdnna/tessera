// Предупреждение «срок задачи подходит» — не событие, а утверждение о
// состоянии: оно верно ровно до тех пор, пока срок действительно близко.
// Сканер проверяет это дважды — сначала отбором (`t.DueDate == nil` пропускается),
// потом окном `now >= due − lead`. Обе проверки делаются РОВНО ОДИН РАЗ, когда
// уведомление создаётся, а строка аутбокса уезжает не сразу: тихие часы
// придерживают её до конца окна, целую ночь.
//
// В зазоре пользователь успевает сделать ровно то, ради чего предупреждение и
// было послано, — передоговориться о сроке. Снял дедлайн совсем или отодвинул
// на неделю, — а утром всё равно получает «срок задачи подходит» о задаче, у
// которой срока больше нет или до него ещё семь дней. Причём отодвинутый срок
// перевзводит состояние, так что правильное предупреждение приедет ПОТОМ, своим
// чередом: устаревшее письмо не заменяет верное, а добавляется к нему.
//
// Та же дверь, что у участия в пространстве, у тумблера уведомлений и у гварда
// закрытой задачи: она открывается не устаревшей ссылкой, а временем. Здесь
// пересматривается второй гвард сканера — сам срок (#2932).
package main

import (
	"net/http"
	"testing"
	"time"
)

// setDue переносит срок задачи; nil снимает его совсем (PATCH трёхпозиционный).
func setDue(t *testing.T, c *client, taskID string, at *time.Time) {
	t.Helper()
	var v any
	if at != nil {
		v = at.Format(time.RFC3339)
	}
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{"due_date": v}), http.StatusOK)
}

// TestClearingTheDueDateOvernightDropsTheHeldDueDelivery: срок сняли до того,
// как тишина отпустила письмо о нём. Сообщать больше нечего — у задачи нет
// дедлайна, и сканер такую задачу не взял бы вовсе.
func TestClearingTheDueDateOvernightDropsTheHeldDueDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	_, taskID := mkOverdueTaskIn(t, c)
	delivID, hits := heldDueDelivery(t, c)

	setDue(t, c, taskID, nil) // 23:30: «сроков у этой задачи больше нет»

	assertHeldDeliveryDropped(t, delivID, hits, "срок у задачи сняли ещё ночью")
}

// TestPushingTheDueDateOutOvernightDropsTheHeldDueDelivery: срок отодвинули на
// неделю — до него дальше, чем предупреждают заранее (по умолчанию за час), так
// что сканер сейчас не сказал бы о нём ни слова.
func TestPushingTheDueDateOutOvernightDropsTheHeldDueDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	_, taskID := mkOverdueTaskIn(t, c)
	delivID, hits := heldDueDelivery(t, c)

	week := time.Now().Add(7 * 24 * time.Hour)
	setDue(t, c, taskID, &week) // 23:30: «договорились на следующую неделю»

	assertHeldDeliveryDropped(t, delivID, hits, "срок отодвинули на неделю вперёд")
}

// Контроль: проверяется расстояние до срока, а не факт правки. Срок подвинули,
// но он всё ещё внутри окна предупреждения (через полчаса при заблаговременности
// в час) — письмо обязано приехать.
func TestDueDateStillApproachingKeepsItsHeldDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	_, taskID := mkOverdueTaskIn(t, c)
	delivID, hits := heldDueDelivery(t, c)

	soon := time.Now().Add(30 * time.Minute)
	setDue(t, c, taskID, &soon)

	morningCame(t, delivID)
	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st != "sent" {
		t.Errorf("строка кончила статусом %q вместо sent: срок всего лишь подвинули на полчаса вперёд, он по-прежнему подходит", st)
	}
	if n := hits(); n == 0 {
		t.Error("приёмник не получил ничего: любая правка срока стала отменой предупреждения о нём")
	}
}

// Контроль: гвард срока — это гвард сроков, а не общий выключатель. У задачи без
// дедлайна остаётся обсуждение, и уведомление о нём обязано уехать.
func TestTaskWithoutDueDateStillDeliversItsOtherNotifications(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	s, taskID := mkOverdueTaskIn(t, c)
	setDue(t, c, taskID, nil)

	hook, hits := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук обсуждения", hook, "")
	delivID := readyTaskDelivery(t, c.UserID, s.WS, taskID, chID, "comment")

	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st != "sent" {
		t.Errorf("комментарий кончил статусом %q вместо sent: снятый срок стал общим выключателем", st)
	}
	if n := hits(); n == 0 {
		t.Error("приёмник не получил комментарий: снятый срок погасил и чужой вид уведомлений")
	}
}
