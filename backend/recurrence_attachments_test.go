// A repeating task's attachments have to follow the live occurrence (#2932).
//
// task_attachments is the last side table the create_new duplicate leaves
// behind. Tags, assignees, the due-notify overrides, each item's own repeat
// rule, the task relations and the document links are all carried by now; the
// files are not. The reschedule branch keeps them for free — it edits the same
// row, and the attachment rows still point at it.
//
// The template is the case that makes it a product bug rather than a tidiness
// one: a weekly report is a rule with the blank form pinned to it, and under
// "создавать копию" the form stays on the spent copy in «Готово» while the live
// occurrence shows an empty «Вложения». Worse, the description travels verbatim,
// and MarkdownEditor writes an uploaded file into it as
// `[имя](/api/attachments/<id>/download)` — so the duplicate's own text goes on
// naming rows that belong to the previous occurrence. Deleting that occurrence,
// the ordinary way to clear a finished month, runs purgeTaskUploads over its
// directory and every one of those links 404s from the live task, forever and in
// every occurrence after it.
//
// The file is COPIED, not the row: storage_path is written under
// <uploadDir>/<task id>/, purgeTaskUploads wipes that directory per task id, and
// its doc states the invariant it rests on — "no row is ever copied to another
// task". Two rows sharing one path would make deleting the spent occurrence
// delete the live one's bytes, which is the very failure this is about.
package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// attachmentNames maps a /tasks/<id>/attachments payload to filename → count.
func attachmentNames(rows []map[string]any) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		name, _ := r["filename"].(string)
		out[name]++
	}
	return out
}

// TestRecurrenceCarriesAttachments: the create_new branch.
func TestRecurrenceCarriesAttachments(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	form := []byte("%PDF-1.7 бланк отчёта")
	att := c.expect(t, uploadFile(t, c, "/tasks/"+origID+"/attachments", "file", "бланк.pdf", form),
		http.StatusCreated)
	attID := att["id"].(string)

	// The description as MarkdownEditor writes it after an upload.
	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"description": "Заполнить [бланк.pdf](/api/attachments/" + attID + "/download) и сдать.",
	}), http.StatusOK)

	// A checklist item with a file of its own — the duplicate rebuilds the whole
	// checklist, so an item's attachments go the way its tags and assignees do.
	step := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свериться с бухгалтерией", "column_id": todo, "parent_id": origID,
	}), http.StatusCreated)
	stepID := step["id"].(string)
	attachFile(t, c, stepID, "сверка.xlsx", []byte("xlsx сверка"))

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	cloneID := clone["id"].(string)

	rows := c.get("/tasks/" + cloneID + "/attachments").listBody(t)
	if got := attachmentNames(rows); len(rows) != 1 || got["бланк.pdf"] != 1 {
		t.Fatalf("the duplicate lists %d attachment(s) %v, want the бланк.pdf it repeats with", len(rows), got)
	}
	cloneAttID := rows[0]["id"].(string)
	if cloneAttID == attID {
		t.Fatalf("the duplicate reuses the spent occurrence's attachment row %s", attID)
	}

	// Its own bytes under its own directory: one file each, and both readable.
	if n, ok := countFiles(t, taskUploadDir(cloneID)); !ok || n != 1 {
		t.Fatalf("the duplicate's upload dir holds %d file(s) (exists=%v), want its own copy", n, ok)
	}
	if n, ok := countFiles(t, taskUploadDir(origID)); !ok || n != 1 {
		t.Fatalf("the spent occurrence lost its file: %d (exists=%v)", n, ok)
	}
	if dl := c.get("/attachments/" + cloneAttID + "/download"); dl.Status != http.StatusOK ||
		string(dl.Body) != string(form) {
		t.Fatalf("downloading the duplicate's copy: status %d, %d byte(s)", dl.Status, len(dl.Body))
	}

	// The text the duplicate was born with has to name its own row.
	desc, _ := clone["description"].(string)
	if strings.Contains(desc, attID) || !strings.Contains(desc, cloneAttID) {
		t.Fatalf("the duplicate's description still links the spent occurrence's file:\n%s", desc)
	}

	// The checklist item's file travels the same way.
	stepClone := subtaskByTitle(t, clone, "Свериться с бухгалтерией")["id"].(string)
	stepRows := c.get("/tasks/" + stepClone + "/attachments").listBody(t)
	if got := attachmentNames(stepRows); len(stepRows) != 1 || got["сверка.xlsx"] != 1 {
		t.Fatalf("the duplicated checklist item lists %d attachment(s) %v", len(stepRows), got)
	}

	// The damage this is really about: clearing the finished occurrence must not
	// take the live one's bytes with it (purgeTaskUploads sweeps by task id).
	c.expect(t, c.del("/tasks/"+origID), http.StatusNoContent)
	if dl := c.get("/attachments/" + cloneAttID + "/download"); dl.Status != http.StatusOK {
		t.Fatalf("deleting the spent occurrence broke the live one's file: status %d", dl.Status)
	}
}

// TestRescheduleKeepsAttachments is the asymmetry this is measured against: the
// branch that edits the same row keeps the file without anyone arranging it.
func TestRescheduleKeepsAttachments(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	due := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Сдать отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	id := task["id"].(string)
	attachFile(t, c, id, "бланк.pdf", []byte("%PDF-1.7 бланк"))

	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Сдать отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	rows := c.get("/tasks/" + id + "/attachments").listBody(t)
	if got := attachmentNames(rows); len(rows) != 1 || got["бланк.pdf"] != 1 {
		t.Fatalf("reschedule lost the file: %d row(s) %v", len(rows), got)
	}
}
