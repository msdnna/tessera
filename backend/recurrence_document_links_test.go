// A repeating task's document links have to follow the live occurrence (#2932).
//
// document_task_links is the last side table the create_new duplicate did not
// touch: tags, assignees, the due-notify overrides, each item's own repeat rule
// and the task relations are all carried by now, but the link between a task and
// the document (or the block of it) the work is governed by stayed on the spent
// copy. The reschedule branch keeps it for free — it edits the same row and the
// link rows still point at it.
//
// The damage is on both panels. The task modal's «Документы» tab of the live
// occurrence is empty, and the document's «Задачи» panel (DocLinks.vue) renders
// every row it is given with no filter on completed_at: the регламент goes on
// naming last week's finished copy as the task that implements it.
//
// The link is MOVED, not copied — this is where it parts with carryRelations. A
// relation joins two tasks, and the spent occurrence's «Связи» tab is the record
// of what that week depended on; a document link joins a task to a living
// artefact, and its `quote` is a snapshot of a block of THAT document. One link
// per occurrence would pile a dead row onto the document's panel every day a
// daily chore repeats, and the reschedule branch shows exactly one link naming
// the live occurrence — which is what the duplicate has to look like too.
package main

import (
	"net/http"
	"testing"
	"time"
)

// docLinkTaskIDs maps a /documents/<id>/tasks payload to the set of task ids it
// names — the rows the document's «Задачи» panel prints.
func docLinkTaskIDs(rows []map[string]any) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		id, _ := r["task_id"].(string)
		out[id]++
	}
	return out
}

func TestRecurrenceCarriesDocumentLinks(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s, docID := mkLinkedDoc(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ── the create_new branch ──────────────────────────────────────────
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	// Two links from the same document: one to the document as a whole, one
	// pinned to a block with the quote that made it worth linking. Both are
	// rows of the same table and both have to travel.
	c.expect(t, c.post("/documents/"+docID+"/tasks", map[string]any{
		"task_id": origID,
	}), http.StatusCreated)
	c.expect(t, c.post("/documents/"+docID+"/tasks", map[string]any{
		"task_id": origID, "block_id": "b7", "quote": "Отчёт сдаётся в понедельник",
	}), http.StatusCreated)

	// A checklist item with a link of its own — the duplicate rebuilds the
	// whole checklist, so an item's links go the way its tags and assignees do.
	step := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свериться с бухгалтерией", "column_id": todo, "parent_id": origID,
	}), http.StatusCreated)
	stepID := step["id"].(string)
	c.expect(t, c.post("/documents/"+docID+"/tasks", map[string]any{
		"task_id": stepID, "block_id": "b9", "quote": "Сверка с бухгалтерией",
	}), http.StatusCreated)

	if rows := c.get("/documents/" + docID + "/tasks").listBody(t); len(rows) != 3 {
		t.Fatalf("fixture: the document lists %d links, want 3\n%v", len(rows), rows)
	}

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	cloneID := clone["id"].(string)

	fromTask := c.get("/tasks/" + cloneID + "/documents").listBody(t)
	if len(fromTask) != 2 {
		t.Fatalf("the next occurrence shows %d document links, want 2 — they stayed on the spent copy\n%v", len(fromTask), fromTask)
	}
	// The block link keeps what it was pinned to: a quote that lost its block
	// would point the panel at the document as a whole.
	var quoted map[string]any
	for _, l := range fromTask {
		if l["block_id"] == "b7" {
			quoted = l
		}
		if l["document_title"] != "Регламент" {
			t.Fatalf("link came over without its document: %#v", l)
		}
	}
	if quoted == nil || quoted["quote"] != "Отчёт сдаётся в понедельник" {
		t.Fatalf("the block link lost its anchor or its quote: %v", fromTask)
	}

	cloneStep := subtaskByTitle(t, clone, "Свериться с бухгалтерией")
	if rows := c.get("/tasks/" + cloneStep["id"].(string) + "/documents").listBody(t); len(rows) != 1 {
		t.Fatalf("the duplicate's checklist item lost its own document link: %v", rows)
	}

	// The document's panel still lists three rows — one per link, not one per
	// link per occurrence — and every one of them names the live occurrence.
	docSide := c.get("/documents/" + docID + "/tasks").listBody(t)
	if len(docSide) != 3 {
		t.Fatalf("the document lists %d links after one repeat, want 3 — the panel grows an occurrence at a time\n%v", len(docSide), docSide)
	}
	byTask := docLinkTaskIDs(docSide)
	if byTask[origID] != 0 {
		t.Fatalf("the document still points at the spent copy: %v", byTask)
	}
	if byTask[cloneID] != 2 || byTask[cloneStep["id"].(string)] != 1 {
		t.Fatalf("the document does not point at the live occurrence: %v", byTask)
	}
	if rows := c.get("/tasks/" + origID + "/documents").listBody(t); len(rows) != 0 {
		t.Fatalf("the spent copy kept %d document links: %v", len(rows), rows)
	}

	// ── the reschedule branch: the control, green before the fix and after ──
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	c.expect(t, c.post("/documents/"+docID+"/tasks", map[string]any{
		"task_id": dailyID,
	}), http.StatusCreated)
	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	if rows := c.get("/tasks/" + dailyID + "/documents").listBody(t); len(rows) != 1 {
		t.Fatalf("a reschedule dropped the task's document links: %v", rows)
	}
}
