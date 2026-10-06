// «Повторять каждые N минут» — это «долби меня, пока не разберусь»: смысл
// повтора в том, что он ДОХОДИТ. Тихие часы ровно это и запрещают — сканер
// сроков про окно тишины не знает вовсе и фирит всю ночь, а routeNotification
// каждую получившуюся строку аутбокса откладывает на конец окна, все на один и
// тот же момент. Просроченная задача с повтором в 15 минут за ночь 22:00–08:00
// даёт сорок уведомлений, и в 08:00 они приходят одним залпом — сорок
// одинаковых писем из фичи, чьё единственное назначение в том, чтобы ночь
// осталась тихой. Долбить некого, когда доставка всё равно запрещена.
//
// Первое предупреждение при этом остаётся: оно ждёт конца окна и утром
// приходит РОВНО ОДИН РАЗ, а повтор возобновляется, как только окно кончилось.
// Тот же класс, что «повтор доставки обходил тихие часы» (#2932), но дверь со
// стороны источника, а не доставки.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// dueRepeatHour — интервал повтора в этих тестах. Час выбран нарочно: сам по
// себе он в коротком тесте не сработает ни разу, каждый повтор тест вызывает
// сам, отматывая last_fired_at назад. Так тик сканера харнесса (раз в минуту,
// по всем задачам сразу) в подсчёт не вмешивается — на момент его тика отметка
// всегда свежая.
const dueRepeatHour = 60

// TestQuietHoursDoNotStockpileDueRepeats: внутри окна тишины повтор обязан
// молчать, а не копиться в аутбоксе на утро.
func TestQuietHoursDoNotStockpileDueRepeats(t *testing.T) {
	c := signup(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук сроков", brokenHook(t), "")
	mkCatchAllRoute(t, c, chID)
	setQuietWindowCoveringNow(t, c)

	taskID := mkOverdueRepeatingTask(t, c, dueRepeatHour)

	// Первый тик — само предупреждение (его тихие часы лишь откладывают).
	// Дальше четыре повтора: перед каждым отметка отматывается на два часа,
	// то есть интервал повтора заведомо истёк.
	api := newScannerAPI(t)
	api.ScanNotificationsOnce(context.Background())
	for range 4 {
		backdateDueFire(t, taskID, c.UserID, 2*time.Hour)
		api.ScanNotificationsOnce(context.Background())
	}

	if n := dueDeliveries(t, c.UserID); n != 1 {
		t.Errorf("в аутбоксе %d строк со сроком, want 1: тихие часы не погасили повторы, а сложили их — в конце окна они придут залпом одинаковых писем", n)
	}
}

// Контроль: повтор обязан жить. Без тихих часов те же пять тиков дают пять
// уведомлений — иначе гашение внутри окна заодно выключило бы саму фичу.
func TestDueRepeatStillFiresOutsideQuietHours(t *testing.T) {
	c := signup(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук сроков", brokenHook(t), "")
	mkCatchAllRoute(t, c, chID)
	setQuietWindowIn(t, c, 3*time.Hour) // окно есть, но до него далеко

	taskID := mkOverdueRepeatingTask(t, c, dueRepeatHour)

	api := newScannerAPI(t)
	api.ScanNotificationsOnce(context.Background())
	for range 4 {
		backdateDueFire(t, taskID, c.UserID, 2*time.Hour)
		api.ScanNotificationsOnce(context.Background())
	}

	if n := dueDeliveries(t, c.UserID); n != 5 {
		t.Errorf("в аутбоксе %d строк со сроком, want 5: повтор перестал повторять вне окна тишины", n)
	}
}

// Контроль: молчит именно ПОВТОР, а не предупреждение. Задача, о которой ещё не
// сообщали, обязана попасть в аутбокс и внутри окна — тихие часы откладывают
// доставку до утра, но не отменяют её.
func TestFirstDueWarningIsEnqueuedInsideQuietHours(t *testing.T) {
	c := signup(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук сроков", brokenHook(t), "")
	mkCatchAllRoute(t, c, chID)
	setQuietWindowCoveringNow(t, c)

	mkOverdueRepeatingTask(t, c, dueRepeatHour)
	newScannerAPI(t).ScanNotificationsOnce(context.Background())

	if n := dueDeliveries(t, c.UserID); n != 1 {
		t.Fatalf("в аутбоксе %d строк со сроком, want 1: предупреждение о сроке пропало вместе с повторами", n)
	}
	if _, next := deliveryPlan(t, c.UserID, "due_soon"); !next.After(time.Now().Add(time.Minute)) {
		t.Errorf("предупреждение готово к отправке в %s — тихие часы перестали его откладывать", next.UTC().Format("15:04"))
	}
}

// setQuietWindowCoveringNow включает тихие часы, накрывающие текущий момент:
// окно началось пять минут назад и длится всю ночь.
func setQuietWindowCoveringNow(t *testing.T, c *client) {
	t.Helper()
	now := time.Now().UTC()
	startMin := ((now.Hour()*60+now.Minute()-5)%1440 + 1440) % 1440
	endMin := (startMin + int(quietSpan.Minutes())) % 1440
	c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": true, "quiet_enabled": true,
		"quiet_start_minutes": startMin, "quiet_end_minutes": endMin, "quiet_tz": "UTC",
		"digest_minutes": 0,
	}), http.StatusOK)
}

// mkOverdueRepeatingTask создаёт просроченную задачу с включённым повтором и
// возвращает её id. Сканер харнесса на это время придержан локом: иначе он
// разберёт задачу до того, как тест сделает свой первый тик.
func mkOverdueRepeatingTask(t *testing.T, c *client, repeatMinutes int) string {
	t.Helper()
	release := holdScannerLock(t)
	defer release()

	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "Продлить домен")["id"].(string)
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"due_date": time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+id+"/due-notify", map[string]any{
		"enabled": true, "lead_minutes": 60, "repeat_minutes": repeatMinutes,
	}), http.StatusOK)
	return id
}

// backdateDueFire отматывает отметку последнего срабатывания назад, чтобы
// следующий тик сканера считал интервал повтора истёкшим.
func backdateDueFire(t *testing.T, taskID, userID string, by time.Duration) {
	t.Helper()
	tag, err := testPool.Exec(context.Background(),
		`UPDATE due_notification_state SET last_fired_at = now() - make_interval(mins => $3)
		  WHERE task_id = $1 AND user_id = $2`, taskID, userID, int(by.Minutes()))
	if err != nil {
		t.Fatalf("backdate due state: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("отметки о срабатывании нет — предупреждение о сроке не сработало вовсе")
	}
}

// dueDeliveries — сколько строк аутбокса накопилось по срокам этого
// пользователя.
func dueDeliveries(t *testing.T, userID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_deliveries d
		   JOIN notifications n ON n.id = d.notification_id
		  WHERE n.user_id = $1 AND n.kind = 'due_soon'`, userID).Scan(&n); err != nil {
		t.Fatalf("count due deliveries: %v", err)
	}
	return n
}
