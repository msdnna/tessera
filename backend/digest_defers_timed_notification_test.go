// Окно дайджеста существует ради потока активности: вместо десяти писем о
// комментариях — один свод. Но digestSlot применяется ко всем видам без разбора,
// а слот лежит на сетке и всегда в БУДУЩЕМ (до digest_minutes вперёд). Часть
// уведомлений сама называет момент, которого читатель ждёт: напоминание — это
// будильник на выбранную пользователем минуту, «предупреди за 15 минут до срока»
// — тоже, а приглашение в звонок может указывать на идущий разговор. Батчинг
// молча переносит их за этот момент: напоминание на 14:00 при часовом окне
// уезжает в 14:59, предупреждение «за 15 минут» приходит после срока,
// приглашение — когда звонок кончился. Ровно этот довод в том же файле уже
// проведён для device-пушей («push, пришедший с опозданием на полчаса, хуже
// отсутствия push'а»), но только для них (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tessera/handlers"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// digestHour — окно дайджеста, с которым гоняются эти тесты: час округляет слот
// до ближайшего HH:00, то есть отодвигает доставку на десятки минут вперёд.
const digestHour = 60

// TestReminderIsNotHeldByDigestWindow: напоминание — будильник на точную минуту.
// Строка аутбокса обязана быть готова к отправке сейчас и вне группы свода.
func TestReminderIsNotHeldByDigestWindow(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук напоминаний", hook, "")
	setDigestPrefs(t, c, digestHour)
	mkCatchAllRoute(t, c, chID)

	// Лок держим, пока напоминание создаётся: иначе сканер харнесса разберёт его
	// до того, как тест сам сделает тик.
	release := holdScannerLock(t)
	c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "выпить таблетку",
	}), http.StatusCreated)
	release()

	newScannerAPI(t).ScanNotificationsOnce(ctx)

	group, next := deliveryPlan(t, c.UserID, "reminder")
	if group != "" {
		t.Errorf("напоминание попало в группу свода %q — доставка ждёт окна", group)
	}
	if late := time.Until(next); late > time.Minute {
		t.Errorf("напоминание отложено на %s вперёд, want «сейчас»: будильник на 14:00 прозвонит в 14:59", late.Round(time.Second))
	}
}

// TestDueSoonIsNotHeldByDigestWindow: «предупреди за 15 минут до срока» — тоже
// названный пользователем момент, и часовое окно уносит предупреждение за сам
// срок.
func TestDueSoonIsNotHeldByDigestWindow(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук сроков", hook, "")
	setDigestPrefs(t, c, digestHour)
	mkCatchAllRoute(t, c, chID)

	release := holdScannerLock(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Сдать отчёт")
	id := task["id"].(string)
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"due_date": time.Now().Add(10 * time.Minute).Format(time.RFC3339),
	}), http.StatusOK)
	// Предупреждать за 15 минут: окно уже открыто (до срока 10 минут).
	c.expect(t, c.patch("/tasks/"+id+"/due-notify", map[string]any{
		"enabled": true, "lead_minutes": 15, "repeat_minutes": 0,
	}), http.StatusOK)
	release()

	newScannerAPI(t).ScanNotificationsOnce(ctx)

	group, next := deliveryPlan(t, c.UserID, "due_soon")
	if group != "" {
		t.Errorf("предупреждение о сроке попало в группу свода %q — доставка ждёт окна", group)
	}
	if late := time.Until(next); late > time.Minute {
		t.Errorf("предупреждение о сроке отложено на %s вперёд, want «сейчас»: «за 15 минут» приедет после срока", late.Round(time.Second))
	}
}

// Контроль: поток активности батчиться обязан — иначе освобождение таймированных
// видов заодно отключило бы само окно. Назначение задачи ждёт слота и несёт
// группу.
func TestActivityNotificationStillWaitsForDigestWindow(t *testing.T) {
	owner := signup(t)
	member := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, member, "Хук активности", hook, "")
	setDigestPrefs(t, member, digestHour)
	mkCatchAllRoute(t, member, chID)

	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "Заметная задача")
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members", map[string]any{"email": member.Email}), http.StatusCreated)
	if r := owner.post("/tasks/"+task["id"].(string)+"/assignees", map[string]any{"user_id": member.UserID}); r.Status != http.StatusNoContent {
		t.Fatalf("assign: %d\n%s", r.Status, r.Body)
	}

	group, next := deliveryPlan(t, member.UserID, "assigned")
	if group == "" {
		t.Errorf("назначение осталось вне группы свода — окно дайджеста перестало собирать активность")
	}
	if !next.After(time.Now().Add(time.Minute)) {
		t.Errorf("назначение готово к отправке в %s — окно дайджеста его не придержало", next)
	}
}

// setDigestPrefs включает окно дайджеста, оставляя тихие часы выключенными: тут
// проверяется именно батчинг, а не осознанная тишина.
func setDigestPrefs(t *testing.T, c *client, digestMinutes int) {
	t.Helper()
	c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": true, "quiet_enabled": false,
		"quiet_start_minutes": 1320, "quiet_end_minutes": 480, "quiet_tz": "",
		"digest_minutes": digestMinutes,
	}), http.StatusOK)
}

// mkCatchAllRoute — правило «всё в этот канал» (пустой matcher = catch-all).
func mkCatchAllRoute(t *testing.T, c *client, chID string) {
	t.Helper()
	c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{}, "channel_ids": []string{chID},
	}), http.StatusCreated)
}

// newScannerAPI — свой экземпляр API, чтобы тик сканера делал тест, а не харнесс.
func newScannerAPI(t *testing.T) *handlers.API {
	t.Helper()
	return handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
}

// deliveryPlan — как маршрутизатор запланировал доставку последнего уведомления
// такого вида: группа свода и момент, с которого строку можно брать.
func deliveryPlan(t *testing.T, userID, kind string) (string, time.Time) {
	t.Helper()
	var group string
	var next time.Time
	if err := testPool.QueryRow(context.Background(),
		`SELECT d.digest_group, d.next_attempt_at
		   FROM notification_deliveries d
		   JOIN notifications n ON n.id = d.notification_id
		  WHERE n.user_id = $1 AND n.kind = $2
		  ORDER BY d.created_at DESC LIMIT 1`, userID, kind).Scan(&group, &next); err != nil {
		t.Fatalf("строки аутбокса для уведомления %q не нашлось: %v", kind, err)
	}
	return group, next
}
