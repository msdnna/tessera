// The user's own actions come back over the socket with their author (#3102).
//
// The web «Начало работы» checklist counts a result («Создать проект»,
// «Перенести карточку», «Поля на карточке» …) whether or not the tour was
// running when the user did it. It reads that off the workspace socket: an event
// whose `actor` is the signed-in user is something they did themselves, on any
// screen, by mouse, keys or ⌘K. These events used to travel without an actor
// (only task.created / task.moved / task.updated from the main task routes had
// one), so the checklist could not tell the user's project from a colleague's.
// The board-activity toast gains a name on the same events.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestOwnActionsCarryTheActor(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	// Membership is read when the socket connects — dial after the workspace.
	conn, status := dialWS(t, c.token, false)
	if conn == nil {
		t.Fatalf("handshake failed (status %d)", status)
	}
	defer conn.Close()

	done := func(r resp) {
		t.Helper()
		if r.Status >= 300 {
			t.Fatalf("status %d\n%s", r.Status, r.Body)
		}
	}
	expectActor := func(evType string) map[string]any {
		t.Helper()
		ev := awaitWSEvent(t, conn, evType, 5*time.Second)
		if ev == nil {
			t.Fatalf("no %s event", evType)
		}
		if ev["actor"] != c.UserID {
			t.Fatalf("%s carried actor %v, want the caller %s", evType, ev["actor"], c.UserID)
		}
		return ev
	}

	g := mkGroup(t, c, ws, "Группа")
	expectActor("group.created")
	g2 := mkGroup(t, c, ws, "Вторая")
	expectActor("group.created")

	p := mkProject(t, c, ws, g, "Проект")
	expectActor("project.created")

	b := mkBoard(t, c, p, "Доска")
	expectActor("board.created")

	done(c.patch("/projects/"+p+"/move", map[string]any{"group_id": g2}))
	expectActor("project.moved")

	cols := c.get("/boards/" + b + "/columns").listBody(t)
	task := mkTask(t, c, b, cols[0]["id"].(string), "Задача")
	taskID := task["id"].(string)
	expectActor("task.created")

	tag := c.expect(t, c.post("/projects/"+p+"/tags", map[string]any{"name": "web"}), http.StatusCreated)
	done(c.post("/tasks/"+taskID+"/tags", map[string]any{"tag_id": tag["id"]}))
	expectActor("task.tagged")

	done(c.post("/tasks/"+taskID+"/assignees", map[string]any{"user_id": c.UserID}))
	expectActor("task.assigned")

	m := c.expect(t, c.post("/projects/"+p+"/milestones", map[string]any{"title": "Спринт 1"}), http.StatusCreated)
	expectActor("milestone.created")
	done(c.post("/tasks/"+taskID+"/milestone", map[string]any{"milestone_id": m["id"]}))
	expectActor("task.updated")
}
