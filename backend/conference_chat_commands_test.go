package main

import (
	"net/http"
	"testing"
)

// The call chat's «/» task and @-mentions (#3055).

func TestConferenceChatCreatesTask(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	ms := mkMilestone(t, owner, s.Project, "Спринт 42")
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Планирование", "milestone_id": ms["id"],
	}), http.StatusCreated)
	confID := conf["id"].(string)

	res := owner.expect(t, owner.post("/conferences/"+confID+"/tasks", map[string]any{
		"title": "  Прогноз   на burn-up ", "board_id": s.Board,
	}), http.StatusCreated)
	task := res["task"].(map[string]any)
	msg := res["message"].(map[string]any)

	t.Run("the task lands in «К работе» of the board, in the call's stage", func(t *testing.T) {
		if task["title"] != "Прогноз на burn-up" {
			t.Fatalf("title not normalised: %q", task["title"])
		}
		var todo string
		for _, col := range s.Columns {
			if col["name_key"] == "todo" {
				todo = col["id"].(string)
			}
		}
		if todo == "" {
			todo = s.col(t, 0)
		}
		if task["column_id"] != todo {
			t.Fatalf("column = %v, want the todo column %s", task["column_id"], todo)
		}
		got := owner.expect(t, owner.get("/tasks/"+task["id"].(string)), http.StatusOK)
		if got["milestone_id"] != ms["id"] {
			t.Fatalf("task not in the call's stage: %v", got["milestone_id"])
		}
	})

	t.Run("the chat gets a task line with a link", func(t *testing.T) {
		if msg["kind"] != "task" {
			t.Fatalf("message kind = %v, want task", msg["kind"])
		}
		ref, _ := msg["task"].(map[string]any)
		if ref == nil || ref["id"] != task["id"] || ref["board_slug"] == "" || ref["project_slug"] == "" {
			t.Fatalf("task ref missing: %#v", msg)
		}
		page := owner.expect(t, owner.get("/conferences/"+confID+"/messages"), http.StatusOK)
		msgs := page["messages"].([]any)
		if len(msgs) != 1 {
			t.Fatalf("want one line, got %d", len(msgs))
		}
		line := msgs[0].(map[string]any)
		if line["kind"] != "task" || line["task"] == nil {
			t.Fatalf("listed line lost its task: %#v", line)
		}
		if body, _ := line["body"].(string); body == "" {
			t.Fatal("a task line must still read as text for old clients")
		}
	})

	t.Run("a deleted task leaves the text without the link", func(t *testing.T) {
		owner.expect(t, owner.del("/tasks/"+task["id"].(string)), http.StatusNoContent)
		page := owner.expect(t, owner.get("/conferences/"+confID+"/messages"), http.StatusOK)
		line := page["messages"].([]any)[0].(map[string]any)
		if line["task"] != nil {
			t.Fatalf("link to a deleted task survived: %#v", line)
		}
		if body, _ := line["body"].(string); body == "" {
			t.Fatal("the line's text went with the task")
		}
	})
}

func TestConferenceChatTaskRefusals(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	other := mkStack(t, owner)
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Созвон"}), http.StatusCreated)
	path := "/conferences/" + conf["id"].(string) + "/tasks"

	owner.expect(t, owner.post(path, map[string]any{"title": "x", "board_id": other.Board}), http.StatusBadRequest)
	owner.expect(t, owner.post(path, map[string]any{"title": "   ", "board_id": s.Board}), http.StatusBadRequest)
	owner.expect(t, owner.post(path, map[string]any{"title": "x", "board_id": s.Board, "column_id": other.col(t, 0)}), http.StatusBadRequest)
	stranger := signup(t)
	stranger.expect(t, stranger.post(path, map[string]any{"title": "x", "board_id": s.Board}), http.StatusForbidden)
}

// A task of another project's board does not go into the call's stage: a stage
// is its project's.
func TestConferenceChatTaskSkipsForeignStage(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	ms := mkMilestone(t, owner, s.Project, "Этап")
	p2 := mkProject(t, owner, s.WS, s.Group, "Другой проект "+t.Name())
	b2 := mkBoard(t, owner, p2, "Доска 2")
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Созвон", "milestone_id": ms["id"],
	}), http.StatusCreated)
	res := owner.expect(t, owner.post("/conferences/"+conf["id"].(string)+"/tasks",
		map[string]any{"title": "задача", "board_id": b2}), http.StatusCreated)
	got := owner.expect(t, owner.get("/tasks/"+res["task"].(map[string]any)["id"].(string)), http.StatusOK)
	if got["milestone_id"] != nil {
		t.Fatalf("a foreign project's task was pinned to the stage: %v", got["milestone_id"])
	}
}

func TestConferenceChatMentions(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	mate := addMember(t, owner, s.WS)
	outsider := signup(t)
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Ревью дизайна"}), http.StatusCreated)
	confID := conf["id"].(string)

	owner.expect(t, owner.post("/conferences/"+confID+"/messages", map[string]any{
		"body":     "@mate посмотри макет",
		"mentions": []string{mate.UserID, mate.UserID, outsider.UserID, owner.UserID},
	}), http.StatusCreated)

	t.Run("the mentioned member is told once, about the call", func(t *testing.T) {
		var hits []map[string]any
		for _, row := range mate.get("/notifications").listBody(t) {
			if row["kind"] == "mention" {
				hits = append(hits, row)
			}
		}
		if len(hits) != 1 {
			t.Fatalf("mate has %d mention notifications, want 1", len(hits))
		}
		p, _ := hits[0]["payload"].(map[string]any)
		if p["event"] != "conference_mention" || p["conference_id"] != confID {
			t.Fatalf("payload does not point at the call: %#v", p)
		}
		if hits[0]["task_id"] != nil {
			t.Fatalf("a call mention must not point at a task: %#v", hits[0])
		}
	})
	t.Run("an outsider and the sender hear nothing", func(t *testing.T) {
		if n := mentionCount(t, outsider); n != 0 {
			t.Fatalf("outsider got %d", n)
		}
		if n := mentionCount(t, owner); n != 0 {
			t.Fatalf("sender got %d", n)
		}
	})
	t.Run("a bad id is refused, and so is a flood", func(t *testing.T) {
		owner.expect(t, owner.post("/conferences/"+confID+"/messages",
			map[string]any{"body": "x", "mentions": []string{"nope"}}), http.StatusBadRequest)
		flood := make([]string, 51)
		for i := range flood {
			flood[i] = mate.UserID
		}
		owner.expect(t, owner.post("/conferences/"+confID+"/messages",
			map[string]any{"body": "x", "mentions": flood}), http.StatusBadRequest)
	})
}
