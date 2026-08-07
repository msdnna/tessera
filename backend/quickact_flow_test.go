package main

import (
	"net/http"
	"testing"
)

// Quick actions typed into a comment. The parser has its own table tests in
// internal/quickact; these check the half that needs a database: argument
// resolution against the workspace, the shared task ops firing, and the
// built-in / custom / plain-text split surviving the round trip.

// commandSummary digs the applied/errored command keys out of a response.
func commandSummary(t *testing.T, r resp) (applied, failed []string) {
	t.Helper()
	m := r.mapBody(t)
	sum, ok := m["command_summary"].(map[string]any)
	if !ok {
		t.Fatalf("no command_summary in response\n%s", r.Body)
	}
	for _, key := range []string{"applied", "errors"} {
		list, _ := sum[key].([]any)
		for _, item := range list {
			oc, _ := item.(map[string]any)
			name, _ := oc["key"].(string)
			if key == "applied" {
				applied = append(applied, name)
			} else {
				failed = append(failed, name)
			}
		}
	}
	return applied, failed
}

// TestQuickActionCloseConsumesComment covers the headline behaviour: a comment
// that is nothing but commands executes and leaves no comment behind.
func TestQuickActionCloseConsumesComment(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача под /close")
	taskID := task["id"].(string)

	// The board's rightmost column is its done column by default.
	done := s.col(t, len(s.Columns)-1)

	r := c.post("/tasks/"+taskID+"/comments", map[string]any{"body": "/close"})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200 (pure-command comment)\n%s", r.Status, r.Body)
	}
	if applied, failed := commandSummary(t, r); len(applied) != 1 || applied[0] != "close" || len(failed) != 0 {
		t.Fatalf("summary applied=%v failed=%v, want [close] / []", applied, failed)
	}

	if comments := c.get("/tasks/" + taskID + "/comments").listBody(t); len(comments) != 0 {
		t.Fatalf("got %d comments, want none (the body was pure commands)", len(comments))
	}
	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["column_id"] != done {
		t.Fatalf("column_id = %v, want the done column %s", after["column_id"], done)
	}
	if after["completed_at"] == nil {
		t.Fatal("completed_at is null: the done column should have completed the task")
	}
}

// TestQuickActionKeepsSurroundingText checks the mixed case: commands run and
// are stripped, the prose the user wrote stays.
func TestQuickActionKeepsSurroundingText(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача под /priority")
	taskID := task["id"].(string)

	r := c.post("/tasks/"+taskID+"/comments", map[string]any{
		"body": "/priority высокий\nПосмотри, пожалуйста.",
	})
	m := c.expect(t, r, http.StatusCreated)
	if got := m["body"]; got != "Посмотри, пожалуйста." {
		t.Fatalf("comment body = %q, want the prose without the command line", got)
	}
	if applied, _ := commandSummary(t, r); len(applied) != 1 || applied[0] != "priority" {
		t.Fatalf("applied = %v, want [priority]", applied)
	}
	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["priority"].(float64) != 3 {
		t.Fatalf("priority = %v, want 3", after["priority"])
	}
}

// TestQuickActionAssignAndRelate covers the two commands that resolve an
// argument against the workspace: a member mention and a #N reference.
func TestQuickActionAssignAndRelate(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Основная")
	other := mkTask(t, c, s.Board, s.col(t, 0), "Блокируемая")
	taskID := task["id"].(string)
	otherNumber := int(other["number"].(float64))

	// The signup email's local part is what "@login" matches on.
	login := c.Email[:len(c.Email)-len("@test.local")]
	r := c.post("/tasks/"+taskID+"/comments", map[string]any{
		"body": "/assign @" + login + "\n/blocks #" + itoa(otherNumber),
	})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	if applied, failed := commandSummary(t, r); len(applied) != 2 || len(failed) != 0 {
		t.Fatalf("applied=%v failed=%v, want both commands applied", applied, failed)
	}

	detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	assignees, _ := detail["assignees"].([]any)
	if len(assignees) != 1 {
		t.Fatalf("got %d assignees, want 1", len(assignees))
	}
	rels := c.get("/tasks/" + taskID + "/relations").listBody(t)
	if len(rels) != 1 || rels[0]["kind"] != "blocks" {
		t.Fatalf("relations = %v, want one 'blocks'", rels)
	}
}

// TestQuickActionCustomCommandNotExecuted is the original scenario from the
// task: "/hold" is a note to a bot, so it must survive as text and do nothing.
func TestQuickActionCustomCommandNotExecuted(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача с /hold")
	taskID := task["id"].(string)

	c.expect(t, c.put("/workspaces/"+s.WS+"/commands", map[string]any{
		"commands": []map[string]any{{"key": "hold", "description": "Отложить"}},
	}), http.StatusOK)

	m := c.expect(t, c.post("/tasks/"+taskID+"/comments", map[string]any{"body": "/hold"}),
		http.StatusCreated)
	if got := m["body"]; got != "/hold" {
		t.Fatalf("comment body = %q, want the custom command left verbatim", got)
	}
	if m["command_summary"] != nil {
		t.Fatalf("custom command produced a summary: %v", m["command_summary"])
	}
	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["completed_at"] != nil || after["column_id"] != s.col(t, 0) {
		t.Fatal("a custom command changed the task; it must do nothing")
	}
}

// TestQuickActionPartialFailure checks that one bad command does not cancel the
// others, and that a failing command reports why.
func TestQuickActionPartialFailure(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Частичный провал")
	taskID := task["id"].(string)

	r := c.post("/tasks/"+taskID+"/comments", map[string]any{
		"body": "/tag нет-такого-тега\n/priority срочный",
	})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	applied, failed := commandSummary(t, r)
	if len(applied) != 1 || applied[0] != "priority" {
		t.Fatalf("applied = %v, want [priority]", applied)
	}
	if len(failed) != 1 || failed[0] != "tag" {
		t.Fatalf("failed = %v, want [tag]", failed)
	}
	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["priority"].(float64) != 4 {
		t.Fatalf("priority = %v, want 4 — the good command must still apply", after["priority"])
	}
}

// TestQuickActionAllFailed guards the one case where a comment could vanish
// silently: nothing but commands, and none of them worked.
func TestQuickActionAllFailed(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Всё сломалось")
	taskID := task["id"].(string)

	r := c.post("/tasks/"+taskID+"/comments", map[string]any{"body": "/move Нет такой колонки"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 (nothing applied, nothing to store)\n%s", r.Status, r.Body)
	}
	if comments := c.get("/tasks/" + taskID + "/comments").listBody(t); len(comments) != 0 {
		t.Fatalf("got %d comments, want none", len(comments))
	}
}

// TestQuickActionCodeBlockNotExecuted is the documentation case: this
// repository writes about its own commands, and those examples must not run.
func TestQuickActionCodeBlockNotExecuted(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Документация")
	taskID := task["id"].(string)

	body := "Например:\n```\n/close\n```\nвот так."
	m := c.expect(t, c.post("/tasks/"+taskID+"/comments", map[string]any{"body": body}),
		http.StatusCreated)
	if m["body"] != body {
		t.Fatalf("comment body = %q, want it verbatim", m["body"])
	}
	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["completed_at"] != nil {
		t.Fatal("a /close inside a code fence closed the task")
	}
}

// TestQuickActionPreviewIsReadOnly checks the editor hint: it must describe the
// same commands the backend would run, and change nothing doing so.
func TestQuickActionPreviewIsReadOnly(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Предпросмотр")
	taskID := task["id"].(string)
	before := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)

	m := c.expect(t, c.post("/tasks/"+taskID+"/commands/preview",
		map[string]any{"body": "/close\n/priority ерунда"}), http.StatusOK)
	cmds, _ := m["commands"].([]any)
	if len(cmds) != 2 {
		t.Fatalf("preview returned %d commands, want 2\n%s", len(cmds), m)
	}
	first, _ := cmds[0].(map[string]any)
	if first["summary"] == "" || first["error"] != nil {
		t.Fatalf("preview of /close = %v, want a summary and no error", first)
	}
	second, _ := cmds[1].(map[string]any)
	if second["error"] == nil {
		t.Fatalf("preview of an invalid priority = %v, want an error", second)
	}
	if m["rest_empty"] != true {
		t.Fatalf("rest_empty = %v, want true", m["rest_empty"])
	}

	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["completed_at"] != before["completed_at"] || after["column_id"] != before["column_id"] {
		t.Fatal("preview mutated the task")
	}
	if comments := c.get("/tasks/" + taskID + "/comments").listBody(t); len(comments) != 0 {
		t.Fatalf("preview created %d comments", len(comments))
	}
}

// TestWorkspaceCommandsDictionary covers the dictionary's gates: managers only,
// and no shadowing a built-in.
func TestWorkspaceCommandsDictionary(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	m := owner.expect(t, owner.get("/workspaces/"+s.WS+"/commands"), http.StatusOK)
	if m["can_manage"] != true {
		t.Fatalf("can_manage = %v for the owner, want true", m["can_manage"])
	}
	if builtin, _ := m["builtin"].([]any); len(builtin) == 0 {
		t.Fatal("builtin registry came back empty")
	}

	// A key that collides with a built-in would make "/close" look inert while
	// the backend quietly closed the task.
	r := owner.put("/workspaces/"+s.WS+"/commands", map[string]any{
		"commands": []map[string]any{{"key": "close", "description": "Не выйдет"}},
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("status %d for a built-in key, want 400\n%s", r.Status, r.Body)
	}

	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)
	if r := member.put("/workspaces/"+s.WS+"/commands", map[string]any{
		"commands": []map[string]any{{"key": "approve", "description": "Одобрить"}},
	}); r.Status != http.StatusForbidden {
		t.Fatalf("status %d for a plain member, want 403\n%s", r.Status, r.Body)
	}
	mm := member.expect(t, member.get("/workspaces/"+s.WS+"/commands"), http.StatusOK)
	if mm["can_manage"] != false {
		t.Fatalf("can_manage = %v for a plain member, want false", mm["can_manage"])
	}
}

// itoa keeps the test bodies readable without dragging strconv into each one.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
