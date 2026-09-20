// Напоминание — личное: оно принадлежит пользователю, а не пространству
// (reminders вообще не workspace-scoped). Но уведомление обязано нести
// workspace_id, и reminderScope подставляет ПЕРВЫЙ воркспейс пользователя
// (ORDER BY created_at) — чисто чтобы строка была валидной и доехала по сокету.
//
// Этот произвольный id уходил дальше, в Event маршрутизатора. Правило
// «не беспокоить меня ничем из рабочего пространства» (matcher.workspace_id +
// mute) матчило личный будильник, который к тому пространству отношения не
// имеет, и глотало его. Зеркально: правило «всё из X → телеграм» забирало
// напоминание себе, и правило ниже его уже не видело (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// Находка: заглушка пространства съедает личное напоминание.
func TestPersonalReminderSurvivesWorkspaceMute(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	s := mkStack(t, c)
	dropPersonalWorkspace(t, c, s.WS) // остаётся только рабочее — оно и станет запасным
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hook, "")

	// Правило 1: всё из этого пространства — заглушить. Правило 2: остальное — в канал.
	mkRawRoute(t, c.UserID, 1, `{"workspace_id":"`+s.WS+`"}`, `{"mute":true}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "выпить таблетку")

	if n := deliveryCount(t, c.UserID, "reminder"); n == 0 {
		t.Fatalf("личное напоминание заглушено правилом пространства %s, к которому оно не относится", s.WS)
	}
}

// Та же подмена без mute: правило пространства ЗАБИРАЕТ личное напоминание себе,
// и оно уезжает в канал, который пользователь выбрал для рабочих уведомлений.
// Проверяется отдельно от заглушки: mute можно было бы «починить» частным
// случаем, а настоящая причина — произвольный workspace_id в событии.
func TestPersonalReminderNotClaimedByWorkspaceRule(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	s := mkStack(t, c)
	dropPersonalWorkspace(t, c, s.WS)
	hookWork, _ := countingHook(t)
	hookOwn, _ := countingHook(t)
	work := mkWebhookChannelTmpl(t, c, "Рабочий", hookWork, "")
	own := mkWebhookChannelTmpl(t, c, "Личный", hookOwn, "")

	mkRawRoute(t, c.UserID, 1, `{"workspace_id":"`+s.WS+`"}`, `{}`, []string{work})
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{own})

	fireReminder(ctx, t, c, "забрать посылку")

	if n := deliveryCountForChannel(t, c.UserID, "reminder", work); n != 0 {
		t.Fatalf("в рабочий канал ушло %d строк, ожидалось 0 — правило пространства забрало личное напоминание", n)
	}
	if n := deliveryCountForChannel(t, c.UserID, "reminder", own); n != 1 {
		t.Fatalf("в личный канал ушло %d строк, ожидалась 1", n)
	}
}

// Контроль A: напоминание, ПРИВЯЗАННОЕ к задаче, пространству принадлежит
// по-настоящему (reminderScope возвращает воркспейс задачи вместе с task_id) —
// и заглушка пространства обязана его глушить. Иначе «личное вне пространств»
// починило бы находку ценой самой адресации правил.
func TestTaskLinkedReminderStillObeysWorkspaceMute(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача пространства")
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hook, "")

	mkRawRoute(t, c.UserID, 1, `{"workspace_id":"`+s.WS+`"}`, `{"mute":true}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminderOnTask(ctx, t, c, "обсудить на планёрке", task["id"].(string))

	if n := deliveryCount(t, c.UserID, "reminder"); n != 0 {
		t.Fatalf("строк аутбокса %d, ожидалось 0 — заглушка пространства перестала глушить его же задачу", n)
	}
}

// Контроль B: заглушка по ВИДУ («напоминания → тишина») по-прежнему глушит
// личное напоминание — это ровно то, чем пользователь выключает будильник.
func TestKindMuteStillMutesPersonalReminder(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	mkStack(t, c)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Почта-заменитель", hook, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{"mute":true}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "заглушённое напоминание")

	if n := deliveryCount(t, c.UserID, "reminder"); n != 0 {
		t.Fatalf("строк аутбокса %d, ожидалось 0 — заглушка по виду перестала глушить", n)
	}
}

// dropPersonalWorkspace удаляет личное пространство, которое регистрация заводит
// каждому: пока оно на месте, запасным оказывается именно оно (ORDER BY
// created_at), и подмена не видна. Пользователь, работающий только в командном
// пространстве, вправе его удалить — и тогда запасным становится чужое по смыслу
// рабочее.
func dropPersonalWorkspace(t *testing.T, c *client, keep string) {
	t.Helper()
	for _, w := range c.get("/workspaces").listBody(t) {
		id, _ := w["id"].(string)
		if id == "" || id == keep {
			continue
		}
		c.expect(t, c.del("/workspaces/"+id), http.StatusNoContent)
	}
	if got := len(c.get("/workspaces").listBody(t)); got != 1 {
		t.Fatalf("у пользователя осталось %d пространств, ожидалось 1", got)
	}
}

// fireReminderOnTask — как fireReminder, но напоминание привязано к задаче:
// такое напоминание пространству принадлежит по-настоящему.
func fireReminderOnTask(ctx context.Context, t *testing.T, c *client, message, taskID string) {
	t.Helper()
	release := holdScannerLock(t)
	c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   message,
		"task_id":   taskID,
	}), http.StatusCreated)
	release()
	newScannerAPI(t).ScanNotificationsOnce(ctx)
}
