// Тумблер «уведомления о сроке» спрашивают РОВНО ОДИН РАЗ — в сканере, когда
// уведомление только создаётся. Дальше строка аутбокса живёт сама: тихие часы
// придерживают её до конца окна (это целая ночь), и в этом зазоре пользователь
// успевает сказать «больше не надо» — выключить тип уведомлений целиком или
// снять галочку у самой задачи. Между этим и отправкой никто не спрашивает
// заново, и утром приходит письмо о сроке, выключенном ночью.
//
// Та же дверь, что у deliveryStillAllowed: она открывается не устаревшей
// ссылкой, а временем. Там пересматривают участие в пространстве, здесь —
// согласие адресата (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// heldQuiet — на сколько тихие часы придерживают строку в этих тестах: ночь.
const heldQuiet = 8 * time.Hour

// prefsForm — снимок экрана настроек: тест меняет один тумблер и отправляет
// префы целиком, как это делает клиент (PUT — полная замена).
type prefsForm struct {
	dueEnabled      bool
	reminderEnabled bool
	quietStartMin   int
	quietEndMin     int
}

// nightPrefs — тишина, начавшаяся пять минут назад и идущая ещё восемь часов,
// оба типа уведомлений включены. Зона — явный UTC, чтобы тест считал те же
// минуты, что и сервер. Границы фиксируются один раз и дальше не пересчитываются:
// иначе повторный PUT выглядел бы сменой окна и перештамповал бы очередь
// (scheduleChanged), а тут проверяется ровно выключенный тумблер.
func nightPrefs() prefsForm {
	now := time.Now().UTC()
	startMin := ((now.Hour()*60+now.Minute()-5)%1440 + 1440) % 1440
	return prefsForm{
		dueEnabled: true, reminderEnabled: true,
		quietStartMin: startMin,
		quietEndMin:   (startMin + int(heldQuiet.Minutes())) % 1440,
	}
}

func (f prefsForm) put(t *testing.T, c *client) {
	t.Helper()
	c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": f.dueEnabled, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": f.reminderEnabled, "quiet_enabled": true,
		"quiet_start_minutes": f.quietStartMin, "quiet_end_minutes": f.quietEndMin,
		"quiet_tz": "UTC", "digest_minutes": 0,
	}), http.StatusOK)
}

// TestTurningDueNotificationsOffDropsTheHeldDelivery: выключенный ночью тип
// уведомлений не имеет права приехать утром.
func TestTurningDueNotificationsOffDropsTheHeldDelivery(t *testing.T) {
	c := signup(t)
	f := nightPrefs()
	f.put(t, c)
	mkOverdueTask(t, c)
	delivID, hits := heldDueDelivery(t, c)

	f.dueEnabled = false
	f.put(t, c) // 23:30: «уведомления о сроке мне больше не нужны»

	assertHeldDeliveryDropped(t, delivID, hits,
		"уведомления о сроке выключены ещё ночью")
}

// TestTurningOffDueNotifyForTheTaskDropsItsHeldDelivery: та же дверь, но жест
// точнее — галочка снята у самой задачи, про которую придержано письмо.
func TestTurningOffDueNotifyForTheTaskDropsItsHeldDelivery(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	taskID := mkOverdueRepeatingTask(t, c, dueRepeatHour)
	delivID, hits := heldDueDelivery(t, c)

	c.expect(t, c.patch("/tasks/"+taskID+"/due-notify", map[string]any{
		"enabled": false, "lead_minutes": 60, "repeat_minutes": dueRepeatHour,
	}), http.StatusOK)

	assertHeldDeliveryDropped(t, delivID, hits,
		"у самой задачи уведомления о сроке сняты ещё ночью")
}

// TestTurningRemindersOffDropsTheHeldDelivery: будильник, отменённый до того,
// как тишина его отпустила, звонить не должен.
func TestTurningRemindersOffDropsTheHeldDelivery(t *testing.T) {
	c := signup(t)
	f := nightPrefs()
	f.put(t, c)
	delivID, hits := heldReminderDelivery(t, c)

	f.reminderEnabled = false
	f.put(t, c)

	assertHeldDeliveryDropped(t, delivID, hits, "напоминания выключены ещё ночью")
}

// Контроль: пересмотр на отправке не имеет права глотать то, от чего никто не
// отказывался. Ничего не выключали — письмо обязано уехать.
func TestHeldDeliveryStillGoesOutWhenNothingWasTurnedOff(t *testing.T) {
	c := signup(t)
	nightPrefs().put(t, c)
	mkOverdueTask(t, c)
	delivID, hits := heldDueDelivery(t, c)

	morningCame(t, delivID)
	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st != "sent" {
		t.Errorf("строка кончила статусом %q вместо sent: пересмотр на отправке выбросил доставку, от которой никто не отказывался", st)
	}
	if n := hits(); n == 0 {
		t.Error("приёмник не получил ничего: придержанное тишиной письмо потерялось утром")
	}
}

// Контроль: тумблер сроков — это тумблер сроков, а не общий выключатель.
// Уведомление другого вида обязано уехать и при выключенных сроках.
func TestSwitchingDueOffLeavesOtherKindsAlone(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	f := nightPrefs()
	f.dueEnabled, f.reminderEnabled = false, false
	f.put(t, c)

	hook, hits := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук активности", hook, "")
	delivID := readyDelivery(t, c.UserID, s.WS, chID, "comment")

	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st != "sent" {
		t.Errorf("комментарий кончил статусом %q вместо sent: выключенные сроки стали общим выключателем", st)
	}
	if n := hits(); n == 0 {
		t.Error("приёмник не получил комментарий: выключенные сроки погасили и чужой вид уведомлений")
	}
}

// assertHeldDeliveryDropped: ночь прошла, строка стала доступной — и обязана
// быть выброшена, а не отправлена. why — то, что пользователь успел сделать в
// зазоре; оно попадает в сообщение об ошибке.
func assertHeldDeliveryDropped(t *testing.T, delivID string, hits func() int, why string) {
	t.Helper()
	morningCame(t, delivID)
	drainDeliveriesUntil(t, func() bool { return deliverySettled(t, delivID) })

	if st := deliveryStatus(t, delivID); st == "sent" {
		t.Errorf("строка аутбокса отправлена, хотя %s: согласие адресата спросили один раз, при постановке, и за ночь никто не переспросил", why)
	}
	if n := hits(); n != 0 {
		t.Errorf("приёмник получил %d сообщени(я), хотя %s", n, why)
	}
}

// heldDueDelivery ставит в аутбокс предупреждение о сроке уже созданной
// просроченной задачи внутри тихих часов и возвращает id придержанной строки и
// счётчик обращений к приёмнику. Путь настоящий: сканер сроков →
// routeNotification → аутбокс.
func heldDueDelivery(t *testing.T, c *client) (string, func() int) {
	t.Helper()
	hook, hits := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук сроков", hook, "")
	mkCatchAllRoute(t, c, chID)

	newScannerAPI(t).ScanNotificationsOnce(context.Background())
	return heldDelivery(t, c.UserID, "due_soon"), hits
}

// heldReminderDelivery — то же для напоминания на минуту, которая уже прошла.
func heldReminderDelivery(t *testing.T, c *client) (string, func() int) {
	t.Helper()
	hook, hits := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук напоминаний", hook, "")
	mkCatchAllRoute(t, c, chID)

	// Лок держим, пока напоминание создаётся: иначе сканер харнесса разберёт его
	// до того, как тест сделает свой тик.
	release := holdScannerLock(t)
	c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "выпить таблетку",
	}), http.StatusCreated)
	release()

	newScannerAPI(t).ScanNotificationsOnce(context.Background())
	return heldDelivery(t, c.UserID, "reminder"), hits
}

// heldDelivery возвращает id последней строки аутбокса такого вида, убедившись,
// что тишина действительно придержала её до утра — иначе премисса теста не
// выполнена и «не отправлено» ничего не доказывает.
func heldDelivery(t *testing.T, userID, kind string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(),
		`SELECT d.id
		   FROM notification_deliveries d
		   JOIN notifications n ON n.id = d.notification_id
		  WHERE n.user_id = $1 AND n.kind = $2
		  ORDER BY d.created_at DESC LIMIT 1`, userID, kind).Scan(&id); err != nil {
		t.Fatalf("строки аутбокса для уведомления %q не нашлось: %v", kind, err)
	}
	if wait := time.Until(deliveryNextAttempt(t, id)); wait < heldQuiet-time.Hour {
		t.Fatalf("строка придержана лишь на %s, а тишина идёт ещё %s — премисса теста не выполнена",
			wait.Round(time.Minute), heldQuiet)
	}
	return id
}

// mkOverdueTask создаёт просроченную задачу БЕЗ пер-задачных настроек срока:
// такая наследует пользовательский тумблер, который тест и выключает. Сканер
// харнесса на это время придержан локом.
func mkOverdueTask(t *testing.T, c *client) string {
	t.Helper()
	release := holdScannerLock(t)
	defer release()

	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "Оплатить хостинг")["id"].(string)
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"due_date": time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
	}), http.StatusOK)
	return id
}

// morningCame отматывает срок отправки в прошлое: харнесс не умеет переводить
// часы, а проверяется именно то, что произойдёт, когда тишина отпустит строку.
func morningCame(t *testing.T, delivID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE notification_deliveries SET next_attempt_at = now() - interval '1 minute' WHERE id = $1`,
		delivID); err != nil {
		t.Fatalf("release held delivery: %v", err)
	}
}

// readyDelivery кладёт уведомление указанного вида и готовую к отправке строку
// аутбокса — без сканера, чтобы вид не зависел от настроек, которые тест как
// раз и выключил.
func readyDelivery(t *testing.T, userID, wsID, chID, kind string) string {
	t.Helper()
	ctx := context.Background()
	var notifID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, workspace_id, kind, text)
		 VALUES ($1, $2, $3, 'кто-то ответил в обсуждении') RETURNING id`,
		userID, wsID, kind).Scan(&notifID); err != nil {
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

// deliverySettled — строка дошла до окончательного статуса: отправлена или
// выброшена. Пока pending/sending, ответа на вопрос теста ещё нет.
func deliverySettled(t *testing.T, delivID string) bool {
	t.Helper()
	switch deliveryStatus(t, delivID) {
	case "sent", "failed":
		return true
	default:
		return false
	}
}
