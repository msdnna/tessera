// Notifications "about me" (#3114): whatever is done to a task a person takes
// part in reaches them — including the task being deleted outright, being taken
// off it, and the task being closed or reopened. The socket events that name the
// change carry who made it, so a client can attribute it (the toasts of #3117).
package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// aboutMeFixture is an owner who acts and a member who is acted upon, sharing
// one workspace.
func aboutMeFixture(t *testing.T) (actor, member *client, s stack) {
	t.Helper()
	actor = signup(t)
	s = mkStack(t, actor)
	member = signup(t)
	actor.expect(t, actor.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)
	return actor, member, s
}

// notificationsOfKind returns the user's notifications of one kind, newest first.
func notificationsOfKind(t *testing.T, c *client, kind string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, n := range c.get("/notifications").listBody(t) {
		if n["kind"] == kind {
			out = append(out, n)
		}
	}
	return out
}

func assign(t *testing.T, c *client, taskID, userID string) {
	t.Helper()
	c.expect(t, c.post("/tasks/"+taskID+"/assignees", map[string]any{"user_id": userID}), http.StatusNoContent)
}

func numberOf(task map[string]any) int64 {
	n, _ := task["number"].(float64)
	return int64(n)
}

func TestDeletingATaskTellsItsParticipants(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	task := mkTask(t, actor, s.Board, s.col(t, 0), "Смета на ремонт кровли")
	id := task["id"].(string)
	assign(t, actor, id, member.UserID)
	assign(t, actor, id, actor.UserID)

	conn, st := dialWS(t, member.token, false)
	if st != http.StatusSwitchingProtocols || conn == nil {
		t.Fatalf("socket handshake: %d", st)
	}
	defer conn.Close()

	actor.expect(t, actor.del("/tasks/"+id), http.StatusNoContent)

	t.Run("the assignee hears it, with the task named", func(t *testing.T) {
		got := notificationsOfKind(t, member, "deleted")
		if len(got) != 1 {
			t.Fatalf("deleted notifications = %d, want 1: %v", len(got), got)
		}
		n := got[0]
		if n["task_id"] != nil {
			t.Fatalf("task_id = %v, want null — the row would cascade away with the task", n["task_id"])
		}
		ref := "#" + strconv.FormatInt(numberOf(task), 10)
		if text, _ := n["text"].(string); !strings.Contains(text, ref) || !strings.Contains(text, "Смета на ремонт кровли") {
			t.Fatalf("text = %q, want the number %s and the title", text, ref)
		}
		p, _ := n["payload"].(map[string]any)
		if p["event"] != "task_deleted" || p["title"] != "Смета на ремонт кровли" ||
			p["board_id"] != s.Board || p["task_number"] != float64(numberOf(task)) {
			t.Fatalf("payload = %v", p)
		}
		if n["actor_id"] != actor.UserID {
			t.Fatalf("actor_id = %v, want %s", n["actor_id"], actor.UserID)
		}
	})

	t.Run("the one who deleted it does not", func(t *testing.T) {
		if got := notificationsOfKind(t, actor, "deleted"); len(got) != 0 {
			t.Fatalf("the actor was notified of their own delete: %v", got)
		}
	})

	t.Run("the socket names the task and the actor", func(t *testing.T) {
		ev := awaitEvent(t, conn, "task.deleted")
		if ev == nil {
			t.Fatal("no task.deleted event")
		}
		if ev["actor"] != actor.UserID {
			t.Fatalf("task.deleted actor = %v, want %s", ev["actor"], actor.UserID)
		}
		d, _ := ev["data"].(map[string]any)
		if d["id"] != id || d["title"] != "Смета на ремонт кровли" || d["board_id"] != s.Board ||
			d["number"] != float64(numberOf(task)) {
			t.Fatalf("task.deleted data = %v", d)
		}
	})
}

// A cascade takes the subtasks too. Each person hears once, about the task
// nearest the root they took part in: the root's participant about the root, a
// participant of a subtask alone about that subtask, under its own number.
func TestDeletingATaskTellsSubtaskParticipantsOnce(t *testing.T) {
	t.Parallel()
	actor, rootMember, s := aboutMeFixture(t)
	subMember := signup(t)
	actor.expect(t, actor.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": subMember.Email, "role": "member"}), http.StatusCreated)

	col := s.col(t, 0)
	root := mkTask(t, actor, s.Board, col, "Корень")
	sub := mkTask(t, actor, s.Board, col, "Подзадача")
	rootID, subID := root["id"].(string), sub["id"].(string)
	actor.expect(t, actor.patch("/tasks/"+subID+"/parent", map[string]any{"parent_id": rootID}), http.StatusOK)
	assign(t, actor, rootID, rootMember.UserID)
	assign(t, actor, subID, rootMember.UserID) // on both: still one notification
	assign(t, actor, subID, subMember.UserID)

	actor.expect(t, actor.del("/tasks/"+rootID), http.StatusNoContent)

	got := notificationsOfKind(t, rootMember, "deleted")
	if len(got) != 1 {
		t.Fatalf("root participant: %d deleted notifications, want 1: %v", len(got), got)
	}
	if p, _ := got[0]["payload"].(map[string]any); p["task_number"] != float64(numberOf(root)) {
		t.Fatalf("root participant heard about %v, want the root #%d", p["task_number"], numberOf(root))
	}

	got = notificationsOfKind(t, subMember, "deleted")
	if len(got) != 1 {
		t.Fatalf("subtask participant: %d deleted notifications, want 1: %v", len(got), got)
	}
	if p, _ := got[0]["payload"].(map[string]any); p["task_number"] != float64(numberOf(sub)) || p["title"] != "Подзадача" {
		t.Fatalf("subtask participant heard %v, want the subtask #%d", p, numberOf(sub))
	}
}

func TestTakingSomeoneOffATaskTellsThem(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	task := mkTask(t, actor, s.Board, s.col(t, 0), "Полить цветы")
	id := task["id"].(string)
	assign(t, actor, id, member.UserID)
	assign(t, actor, id, actor.UserID)

	actor.expect(t, actor.del("/tasks/"+id+"/assignees/"+member.UserID), http.StatusNoContent)
	// A second remove of someone no longer on the task is not a second event.
	actor.expect(t, actor.del("/tasks/"+id+"/assignees/"+member.UserID), http.StatusNoContent)
	// Taking yourself off is not news to you.
	actor.expect(t, actor.del("/tasks/"+id+"/assignees/"+actor.UserID), http.StatusNoContent)

	got := notificationsOfKind(t, member, "unassigned")
	if len(got) != 1 {
		t.Fatalf("unassigned notifications = %d, want exactly 1: %v", len(got), got)
	}
	if got[0]["task_id"] != id {
		t.Fatalf("task_id = %v, want %s", got[0]["task_id"], id)
	}
	if p, _ := got[0]["payload"].(map[string]any); p["event"] != "task_unassigned" {
		t.Fatalf("payload = %v", p)
	}
	if text, _ := got[0]["text"].(string); !strings.Contains(text, "снял(а) вас с задачи #"+strconv.FormatInt(numberOf(task), 10)) {
		t.Fatalf("text = %q", text)
	}
	if got := notificationsOfKind(t, actor, "unassigned"); len(got) != 0 {
		t.Fatalf("the actor was notified of taking themselves off: %v", got)
	}
}

// A drag into the board's done column is «выполнил(а)», out of it «вернул(а) в
// работу», anything else a plain move — one notification per move, not two.
func TestMovingIntoAndOutOfDoneSaysCompletedAndReopened(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	task := mkTask(t, actor, s.Board, s.col(t, 0), "Выкатить релиз")
	id := task["id"].(string)
	assign(t, actor, id, member.UserID)
	done := s.col(t, len(s.Columns)-1) // a fresh board's done column is its rightmost

	actor.expect(t, actor.patch("/tasks/"+id+"/move", map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)
	actor.expect(t, actor.patch("/tasks/"+id+"/move", map[string]any{"column_id": done}), http.StatusOK)
	actor.expect(t, actor.patch("/tasks/"+id+"/move", map[string]any{"column_id": s.col(t, 0)}), http.StatusOK)

	if got := notificationsOfKind(t, member, "moved"); len(got) != 1 {
		t.Fatalf("moved notifications = %d, want 1 (the plain move only): %v", len(got), got)
	}
	completed := notificationsOfKind(t, member, "completed")
	if len(completed) != 1 {
		t.Fatalf("completed notifications = %d, want 1: %v", len(completed), completed)
	}
	if p, _ := completed[0]["payload"].(map[string]any); p["event"] != "task_completed" {
		t.Fatalf("completed payload = %v", p)
	}
	reopened := notificationsOfKind(t, member, "reopened")
	if len(reopened) != 1 {
		t.Fatalf("reopened notifications = %d, want 1: %v", len(reopened), reopened)
	}
	if p, _ := reopened[0]["payload"].(map[string]any); p["event"] != "task_reopened" {
		t.Fatalf("reopened payload = %v", p)
	}
	for _, kind := range []string{"moved", "completed", "reopened"} {
		if got := notificationsOfKind(t, actor, kind); len(got) != 0 {
			t.Fatalf("the actor was notified of their own %s: %v", kind, got)
		}
	}
}

// The checkbox closes and reopens too; the completion is its own notification,
// and whatever else the same request changed still goes out as an edit.
func TestTickingTheCheckboxSaysCompletedAndReopened(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	task := mkTask(t, actor, s.Board, s.col(t, 0), "Сдать отчёт")
	id := task["id"].(string)
	assign(t, actor, id, member.UserID)

	actor.expect(t, actor.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
	if got := notificationsOfKind(t, member, "completed"); len(got) != 1 {
		t.Fatalf("completed notifications = %d, want 1: %v", len(got), got)
	}
	if got := notificationsOfKind(t, member, "updated"); len(got) != 0 {
		t.Fatalf("a bare close also went out as an edit: %v", got)
	}

	actor.expect(t, actor.patch("/tasks/"+id, map[string]any{"completed": false, "title": "Сдать отчёт за квартал"}), http.StatusOK)
	if got := notificationsOfKind(t, member, "reopened"); len(got) != 1 {
		t.Fatalf("reopened notifications = %d, want 1: %v", len(got), got)
	}
	updated := notificationsOfKind(t, member, "updated")
	if len(updated) != 1 {
		t.Fatalf("updated notifications = %d, want 1 (the title): %v", len(updated), updated)
	}
	p, _ := updated[0]["payload"].(map[string]any)
	if fields, _ := p["fields"].([]any); len(fields) != 1 || fields[0] != "title" {
		t.Fatalf("updated fields = %v, want only the title", p["fields"])
	}
}

// task.archived / task.restored / task.commented carry who did it, so a client
// can attribute them; their payloads are what they were.
func TestTaskEventsCarryTheActor(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	task := mkTask(t, actor, s.Board, s.col(t, 0), "Проверить счета")
	id := task["id"].(string)

	conn, st := dialWS(t, member.token, false)
	if st != http.StatusSwitchingProtocols || conn == nil {
		t.Fatalf("socket handshake: %d", st)
	}
	defer conn.Close()

	actor.expect(t, actor.post("/tasks/"+id+"/comments", map[string]any{"body": "смотрю"}), http.StatusCreated)
	ev := awaitEvent(t, conn, "task.commented")
	if ev == nil || ev["actor"] != actor.UserID {
		t.Fatalf("task.commented = %v, want actor %s", ev, actor.UserID)
	}
	if d, _ := ev["data"].(map[string]any); d["task_id"] != id {
		t.Fatalf("task.commented data = %v", d)
	}

	actor.expect(t, actor.patch("/tasks/"+id+"/archive", map[string]any{}), http.StatusNoContent)
	ev = awaitEvent(t, conn, "task.archived")
	if ev == nil || ev["actor"] != actor.UserID {
		t.Fatalf("task.archived = %v, want actor %s", ev, actor.UserID)
	}
	if d, _ := ev["data"].(map[string]any); d["id"] != id {
		t.Fatalf("task.archived data = %v", d)
	}

	actor.expect(t, actor.patch("/tasks/"+id+"/restore", map[string]any{}), http.StatusNoContent)
	ev = awaitEvent(t, conn, "task.restored")
	if ev == nil || ev["actor"] != actor.UserID {
		t.Fatalf("task.restored = %v, want actor %s", ev, actor.UserID)
	}
	if d, _ := ev["data"].(map[string]any); d["id"] != id {
		t.Fatalf("task.restored data = %v", d)
	}
}

// awaitEvent drains frames until one of the given type arrives, or the budget
// runs out (nil).
func awaitEvent(t *testing.T, conn *websocket.Conn, typ string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ev := readEvent(t, conn, time.Until(deadline))
		if ev == nil {
			return nil
		}
		if ev["type"] == typ {
			return ev
		}
	}
	return nil
}
