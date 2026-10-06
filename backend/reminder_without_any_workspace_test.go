// Напоминание — личное: оно принадлежит пользователю, а не пространству. Но
// строка уведомления обязана нести workspace_id, и reminderScope подставляет
// ПЕРВЫЙ воркспейс пользователя просто чтобы строка была валидной. Когда
// пространств нет вовсе (владелец удалил личное и вышел из остальных), подставить
// нечего — возвращается uuid.Nil.
//
// Вся дорога ниже этот случай знает и пропускает: notifiableInWorkspace прямо
// документирует Nil как «нечего утекать, пускаем», deliveryStillAllowed не
// перепроверяет членство, шаблон терпит отсутствие названия пространства. А
// колонка notifications.workspace_id — NOT NULL REFERENCES workspaces(id), то
// есть нулевой uuid не проходит внешним ключом: CreateNotification падает,
// deliverNotification молча выходит, и следом MarkReminderNotified помечает
// напоминание отработавшим. Будильник не звонит НИКОГДА и не повторяется (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
)

// Находка: у пользователя без единого пространства напоминание пропадает.
func TestReminderFiresWithoutAnyWorkspace(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	dropAllWorkspaces(t, c)
	hook, _ := countingHook(t)
	ch := mkWebhookChannelTmpl(t, c, "Личный", hook, "")
	mkRawRoute(t, c.UserID, 1, `{}`, `{}`, []string{ch})

	fireReminder(ctx, t, c, "принять лекарство")

	if n := deliveryCount(t, c.UserID, "reminder"); n != 1 {
		t.Fatalf("строк аутбокса %d, ожидалась 1 — напоминание пропало: пометить его нечем, пространств у пользователя нет", n)
	}
}

// Та же находка со стороны колокольчика: даже без внешних каналов напоминание
// обязано остаться в списке уведомлений. Проверяется отдельно от аутбокса —
// маршрутизацию можно было бы «починить» в обход самой строки уведомления.
func TestReminderIsStoredWithoutAnyWorkspace(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	dropAllWorkspaces(t, c)

	fireReminder(ctx, t, c, "позвонить в клинику")

	if n := notificationCount(t, c.UserID, "reminder"); n != 1 {
		t.Fatalf("уведомлений вида reminder %d, ожидалось 1 — напоминание не доехало даже до колокольчика", n)
	}
}

// Контроль A: напоминание по-прежнему отрабатывает РОВНО один раз. Оно
// помечается отработавшим независимо от того, доставили его или нет, поэтому
// «починка», оставляющая строку неотмеченной, дала бы будильник на каждом тике
// сканера.
func TestReminderWithoutWorkspaceFiresOnce(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	dropAllWorkspaces(t, c)
	hook, _ := countingHook(t)
	ch := mkWebhookChannelTmpl(t, c, "Личный", hook, "")
	mkRawRoute(t, c.UserID, 1, `{}`, `{}`, []string{ch})

	fireReminder(ctx, t, c, "полить цветы")
	newScannerAPI(t).ScanNotificationsOnce(ctx)

	if n := deliveryCount(t, c.UserID, "reminder"); n != 1 {
		t.Fatalf("строк аутбокса %d после второго прохода сканера, ожидалась 1", n)
	}
}

// Контроль B: у пользователя с пространством ничего не меняется — напоминание
// по-прежнему несёт его id и по-прежнему доставляется один раз.
func TestReminderWithWorkspaceUnchanged(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	s := mkStack(t, c)
	hook, _ := countingHook(t)
	ch := mkWebhookChannelTmpl(t, c, "Личный", hook, "")
	mkRawRoute(t, c.UserID, 1, `{"workspace_id":"`+s.WS+`"}`, `{}`, []string{ch})

	fireReminder(ctx, t, c, "забрать документы")

	if n := notificationCount(t, c.UserID, "reminder"); n != 1 {
		t.Fatalf("уведомлений вида reminder %d, ожидалось 1", n)
	}
}

// dropAllWorkspaces удаляет ВСЕ пространства пользователя, включая личное,
// которое заводится при регистрации. DeleteWorkspace спрашивает только про
// владельца — «последнее» пространство ничем не защищено.
func dropAllWorkspaces(t *testing.T, c *client) {
	t.Helper()
	for _, w := range c.get("/workspaces").listBody(t) {
		id, _ := w["id"].(string)
		if id == "" {
			continue
		}
		c.expect(t, c.del("/workspaces/"+id), http.StatusNoContent)
	}
	if got := len(c.get("/workspaces").listBody(t)); got != 0 {
		t.Fatalf("у пользователя осталось %d пространств, ожидалось 0", got)
	}
}

// notificationCount — сколько уведомлений такого вида лежит у пользователя.
func notificationCount(t *testing.T, userID, kind string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = $2`,
		userID, kind).Scan(&n); err != nil {
		t.Fatalf("посчитать уведомления: %v", err)
	}
	return n
}
