// «Входящие» (#2972, migration 0081): private notes, the joined feed, the task's
// own section and snoozing. The private note is the one with teeth — before 0081
// every note was the workspace's, so each reader path (list, by id, search,
// task section, socket) had nothing to hide and now must hide it.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPrivateNoteStaysWithItsAuthor(t *testing.T) {
	owner := signup(t)
	s := mkStack(t, owner)
	colleague := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": colleague.Email, "role": "member"}), http.StatusCreated)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "Релиз 0.150")
	taskID := task["id"].(string)

	conn, st := dialWS(t, colleague.token, false)
	if st != http.StatusSwitchingProtocols || conn == nil {
		t.Fatalf("colleague socket handshake: %d", st)
	}
	defer conn.Close()

	const secret = "зарплатная вилка кандидата"
	note := owner.expect(t, owner.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": secret, "body": "только мне", "visibility": "private", "task_id": taskID,
	}), http.StatusCreated)
	id := note["id"].(string)
	if note["visibility"] != "private" || note["task_id"] != taskID {
		t.Fatalf("created note: %v", note)
	}
	// The socket is read once, at the end (a timed-out read spoils a gorilla
	// connection): everything up to the delete of the hidden note must arrive,
	// and nothing of the private one.

	// A shared note is the control: the colleague sees that one everywhere.
	shared := owner.expect(t, owner.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": "общая повестка",
	}), http.StatusCreated)
	if shared["visibility"] != "workspace" {
		t.Fatalf("a note without visibility must stay shared (pre-0081 behaviour): %v", shared)
	}

	contains := func(list []map[string]any, id string) bool {
		for _, n := range list {
			if n["id"] == id {
				return true
			}
		}
		return false
	}
	if l := colleague.get("/workspaces/" + s.WS + "/notes").listBody(t); contains(l, id) || !contains(l, shared["id"].(string)) {
		t.Fatalf("colleague's note list: %v", l)
	}
	if !contains(owner.get("/workspaces/"+s.WS+"/notes").listBody(t), id) {
		t.Fatal("the author lost their own private note")
	}
	if r := colleague.get("/notes/" + id); r.Status != http.StatusNotFound {
		t.Fatalf("colleague GET private note: %d, want 404", r.Status)
	}
	if r := colleague.patch("/notes/"+id, map[string]any{"title": "x"}); r.Status != http.StatusNotFound {
		t.Fatalf("colleague PATCH private note: %d, want 404", r.Status)
	}
	if r := colleague.del("/notes/" + id); r.Status != http.StatusNotFound {
		t.Fatalf("colleague DELETE private note: %d, want 404", r.Status)
	}
	search := colleague.get("/workspaces/" + s.WS + "/search?q=" + "зарплатная").mapBody(t)
	if raw, _ := json.Marshal(search["notes"]); strings.Contains(string(raw), id) {
		t.Fatalf("search found a colleague's private note: %s", raw)
	}
	mine := owner.get("/workspaces/" + s.WS + "/search?q=" + "зарплатная").mapBody(t)
	if raw, _ := json.Marshal(mine["notes"]); !strings.Contains(string(raw), id) {
		t.Fatalf("the author's search misses their private note: %s", raw)
	}
	if l := colleague.get("/tasks/" + taskID + "/notes").listBody(t); contains(l, id) {
		t.Fatalf("task section shows a colleague's private note: %v", l)
	}
	if l := owner.get("/tasks/" + taskID + "/notes").listBody(t); !contains(l, id) {
		t.Fatalf("task section misses the author's note: %v", l)
	}
	inbox := colleague.get("/workspaces/" + s.WS + "/inbox").mapBody(t)
	if raw, _ := json.Marshal(inbox["notes"]); strings.Contains(string(raw), id) {
		t.Fatalf("colleague's inbox carries the private note: %s", raw)
	}

	// Only the author decides who sees a note.
	sid := shared["id"].(string)
	if r := colleague.patch("/notes/"+sid, map[string]any{"title": "общая повестка", "visibility": "private"}); r.Status != http.StatusForbidden {
		t.Fatalf("non-author hiding a shared note: %d, want 403", r.Status)
	}
	// The author hides it: everyone else is told it is gone.
	owner.expect(t, owner.patch("/notes/"+sid, map[string]any{"title": "общая повестка", "visibility": "private"}), http.StatusOK)
	deadline := time.Now().Add(3 * time.Second)
	for {
		ev := readEvent(t, conn, time.Until(deadline))
		if ev == nil {
			t.Fatal("hiding a shared note did not take it off a colleague's screen")
		}
		raw, _ := json.Marshal(ev)
		if strings.Contains(string(raw), secret) || strings.Contains(string(raw), id) {
			t.Fatalf("a colleague's socket received the private note: %s", raw)
		}
		if ev["type"] == "note.updated" && strings.Contains(string(raw), sid) {
			t.Fatalf("a colleague's socket received the now-private note: %s", raw)
		}
		if ev["type"] == "note.deleted" && strings.Contains(string(raw), sid) {
			break
		}
	}
	if r := colleague.get("/notes/" + sid); r.Status != http.StatusNotFound {
		t.Fatalf("hidden note still readable by a colleague: %d", r.Status)
	}
}

func TestNoteTaskMustBeInItsWorkspace(t *testing.T) {
	a := signup(t)
	sa := mkStack(t, a)
	sb := mkStack(t, a)
	foreign := mkTask(t, a, sb.Board, sb.col(t, 0), "в другом пространстве")
	r := a.post("/workspaces/"+sa.WS+"/notes", map[string]any{"title": "x", "task_id": foreign["id"]})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("note tied to a task of another workspace: %d, want 400", r.Status)
	}
	n := a.expect(t, a.post("/workspaces/"+sa.WS+"/notes", map[string]any{"title": "x"}), http.StatusCreated)
	own := mkTask(t, a, sa.Board, sa.col(t, 0), "своя")
	// Link, then unlink with null; an absent task_id keeps the link.
	linked := a.expect(t, a.patch("/notes/"+n["id"].(string), map[string]any{"title": "x", "task_id": own["id"]}), http.StatusOK)
	if linked["task_id"] != own["id"] {
		t.Fatalf("link: %v", linked)
	}
	kept := a.expect(t, a.patch("/notes/"+n["id"].(string), map[string]any{"title": "y"}), http.StatusOK)
	if kept["task_id"] != own["id"] {
		t.Fatalf("absent task_id dropped the link: %v", kept)
	}
	cleared := a.expect(t, a.patch("/notes/"+n["id"].(string), map[string]any{"title": "y", "task_id": nil}), http.StatusOK)
	if cleared["task_id"] != nil {
		t.Fatalf("null task_id kept the link: %v", cleared)
	}
}

func TestInboxFeed(t *testing.T) {
	u := signup(t)
	s := mkStack(t, u)
	other := mkStack(t, u)
	task := mkTask(t, u, s.Board, s.col(t, 0), "Онбординг: пустые доски")
	taskID := task["id"].(string)
	soon := time.Now().Add(time.Hour).Format(time.RFC3339)

	// Linked to a task → takes the task's workspace without being told.
	linked := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Созвон по релизу", "body": "Повестка", "task_id": taskID,
	}), http.StatusCreated)
	if linked["workspace_id"] != s.WS || linked["body"] != "Повестка" {
		t.Fatalf("linked reminder: %v", linked)
	}
	here := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Позвонить в банк", "workspace_id": s.WS,
	}), http.StatusCreated)
	nowhere := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Старое без пространства",
	}), http.StatusCreated)
	elsewhere := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Чужое пространство", "workspace_id": other.WS,
	}), http.StatusCreated)
	note := u.expect(t, u.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": "Идеи для онбординга", "visibility": "private", "task_id": taskID,
	}), http.StatusCreated)

	inbox := u.expect(t, u.get("/workspaces/"+s.WS+"/inbox"), http.StatusOK)
	rems, _ := inbox["reminders"].([]any)
	ids := map[string]map[string]any{}
	for _, r := range rems {
		m := r.(map[string]any)
		ids[m["id"].(string)] = m
	}
	for _, want := range []map[string]any{linked, here, nowhere} {
		if ids[want["id"].(string)] == nil {
			t.Fatalf("inbox misses reminder %q: %v", want["message"], rems)
		}
	}
	if ids[elsewhere["id"].(string)] != nil {
		t.Fatalf("inbox carries another workspace's reminder")
	}
	chip, _ := ids[linked["id"].(string)]["task"].(map[string]any)
	if chip == nil || chip["title"] != "Онбординг: пустые доски" || chip["board_slug"] == "" {
		t.Fatalf("linked reminder's task chip: %v", ids[linked["id"].(string)])
	}
	if ids[here["id"].(string)]["task"] != nil {
		t.Fatal("a reminder without a task got a chip")
	}
	notes, _ := inbox["notes"].([]any)
	if len(notes) != 1 || notes[0].(map[string]any)["id"] != note["id"] ||
		notes[0].(map[string]any)["task"] == nil {
		t.Fatalf("inbox notes: %v", notes)
	}

	// The task's own section lists the caller's reminders only.
	stranger := signup(t)
	u.expect(t, u.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": stranger.Email, "role": "member"}), http.StatusCreated)
	stranger.expect(t, stranger.post("/reminders", map[string]any{
		"remind_at": soon, "message": "не твоё", "task_id": taskID,
	}), http.StatusCreated)
	own := u.get("/tasks/" + taskID + "/reminders").listBody(t)
	if len(own) != 1 || own[0]["id"] != linked["id"] {
		t.Fatalf("task reminders: %v", own)
	}
}

func TestReminderSnoozeAndDone(t *testing.T) {
	u := signup(t)
	at := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	r := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": at.Format(time.RFC3339), "message": "Проверить пайплайн",
	}), http.StatusCreated)
	id := r["id"].(string)

	first := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	sn := u.expect(t, u.post("/reminders/"+id+"/snooze", map[string]any{"remind_at": first.Format(time.RFC3339)}), http.StatusOK)
	if parse(t, sn["snoozed_from"]) != at || parse(t, sn["remind_at"]) != first {
		t.Fatalf("snooze: %v", sn)
	}
	// Snoozed again: the chip keeps the time it was first due.
	again := u.expect(t, u.post("/reminders/"+id+"/snooze", map[string]any{"remind_at": first.Add(time.Hour).Format(time.RFC3339)}), http.StatusOK)
	if parse(t, again["snoozed_from"]) != at {
		t.Fatalf("second snooze moved snoozed_from: %v", again)
	}

	// done stamps done_at; the old-style full PATCH (Android) keeps body and the
	// snooze mark because remind_at did not change.
	done := u.expect(t, u.patch("/reminders/"+id, map[string]any{
		"remind_at": again["remind_at"], "message": "Проверить пайплайн", "done": true,
	}), http.StatusOK)
	if done["done"] != true || done["done_at"] == nil || done["snoozed_from"] == nil {
		t.Fatalf("done: %v", done)
	}
	undone := u.expect(t, u.patch("/reminders/"+id, map[string]any{"done": false}), http.StatusOK)
	if undone["done"] != false || undone["done_at"] != nil {
		t.Fatalf("undone: %v", undone)
	}
	// A plain reschedule is not a snooze: the mark goes.
	moved := u.expect(t, u.patch("/reminders/"+id, map[string]any{
		"remind_at": first.Add(3 * time.Hour).Format(time.RFC3339),
	}), http.StatusOK)
	if moved["snoozed_from"] != nil || moved["message"] != "Проверить пайплайн" {
		t.Fatalf("reschedule: %v", moved)
	}
	// Undo of a snooze restores both fields explicitly.
	back := u.expect(t, u.patch("/reminders/"+id, map[string]any{
		"remind_at": at.Format(time.RFC3339), "snoozed_from": nil,
	}), http.StatusOK)
	if parse(t, back["remind_at"]) != at || back["snoozed_from"] != nil {
		t.Fatalf("undo: %v", back)
	}
}

func parse(t *testing.T, v any) time.Time {
	t.Helper()
	s, _ := v.(string)
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("time %v: %v", v, err)
	}
	return at.UTC()
}
