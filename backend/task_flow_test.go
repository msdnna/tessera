// Task lifecycle: create/get/update, kanban moves, archive/restore/delete,
// subtask tree (+cycle guards), assignees, eisenhower, cross-board transfer and
// the recurrence rule round-trip (incl. a complete-triggered advance).
package main

import (
	"net/http"
	"testing"
	"time"
)

// parseTS parses an RFC3339 timestamp coming back from the API.
func parseTS(t *testing.T, v any) time.Time {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("timestamp is not a string: %v", v)
	}
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse timestamp %q: %v", s, err)
	}
	return ts
}

// Create with rich fields, full GET shape, and the (full-replace) update.
func TestTaskCreateGetUpdate(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	due := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	created := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Полная задача", "column_id": s.col(t, 0),
		"description": "Описание в **Markdown**", "priority": 2,
		"due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	id := created["id"].(string)
	if created["description"] != "Описание в **Markdown**" || created["priority"] != float64(2) {
		t.Fatalf("create fields: %v", created)
	}
	if !parseTS(t, created["due_date"]).Equal(due) {
		t.Fatalf("due_date round-trip: %v", created["due_date"])
	}
	if created["number"] == nil {
		t.Fatalf("task has no workspace number: %v", created)
	}

	// GET returns the full detail shape: tags/subtasks are [] when empty.
	// NOTE: assignees comes back as null (not []) on a task without assignees —
	// unlike tags/subtasks/gitlab_assignees, which are normalised to [].
	full := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
	for _, k := range []string{"tags", "subtasks"} {
		if _, ok := full[k].([]any); !ok {
			t.Fatalf("GET task: %s not an array: %v", k, full[k])
		}
	}
	if v, ok := full["assignees"]; !ok {
		t.Fatalf("GET task: no assignees key")
	} else if _, isArr := v.([]any); v != nil && !isArr {
		t.Fatalf("GET task: assignees neither null nor array: %v", v)
	}

	// Update is full-replace: send every field we want to keep.
	due2 := due.AddDate(0, 0, 5)
	updated := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Обновлённая", "description": "Новое описание", "priority": 3,
		"due_date": due2.Format(time.RFC3339),
	}), http.StatusOK)
	if updated["title"] != "Обновлённая" || updated["description"] != "Новое описание" ||
		updated["priority"] != float64(3) || !parseTS(t, updated["due_date"]).Equal(due2) {
		t.Fatalf("update fields: %v", updated)
	}
	kinds := eventKinds(t, c, id)
	for _, k := range []string{"created", "renamed", "description", "priority", "due"} {
		if !hasKind(kinds, k) {
			t.Fatalf("journal missing %q: %v", k, kinds)
		}
	}

	// Tri-state PATCH: an omitted field keeps its stored value. A title-only edit
	// used to wipe priority and due — the same full-replace rule that let a client
	// forgetting `completed` silently un-complete a task.
	kept := c.expect(t, c.patch("/tasks/"+id, map[string]any{"title": "Только название"}), http.StatusOK)
	if kept["description"] != "Новое описание" || kept["priority"] != float64(3) ||
		!parseTS(t, kept["due_date"]).Equal(due2) {
		t.Fatalf("omitted fields should be preserved: %v", kept)
	}
	// Present description (even empty) still replaces — the modal can clear it.
	cleared := c.expect(t, c.patch("/tasks/"+id, map[string]any{"description": ""}), http.StatusOK)
	if cleared["description"] != "" {
		t.Fatalf("explicit empty description should clear: %v", cleared)
	}
	// Explicit null clears a nullable field; the neighbouring one is untouched.
	nulled := c.expect(t, c.patch("/tasks/"+id, map[string]any{"due_date": nil}), http.StatusOK)
	if nulled["due_date"] != nil || nulled["priority"] != float64(3) {
		t.Fatalf("null due_date should clear only due_date: %v", nulled)
	}

	// `completed` is the field the old full-replace semantics endangered: an edit
	// that never mentions it must not un-complete the task.
	done := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
	if done["completed_at"] == nil {
		t.Fatalf("completed:true did not set completed_at: %v", done)
	}
	still := c.expect(t, c.patch("/tasks/"+id, map[string]any{"title": "Переименована"}), http.StatusOK)
	if still["completed_at"] == nil {
		t.Fatalf("a title-only edit un-completed the task: %v", still)
	}
	// …while an explicit false still un-completes it.
	reopened := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": false}), http.StatusOK)
	if reopened["completed_at"] != nil {
		t.Fatalf("completed:false did not clear completed_at: %v", reopened)
	}

	// Title is no longer required (absent = keep), but an explicit empty one is
	// still rejected.
	if r := c.patch("/tasks/"+id, map[string]any{"description": "без названия"}); r.Status != http.StatusOK {
		t.Fatalf("update without title: %d", r.Status)
	}
	if r := c.patch("/tasks/"+id, map[string]any{"title": "   "}); r.Status != http.StatusBadRequest {
		t.Fatalf("blank title should be rejected: %d", r.Status)
	}
}

// The board list omits the (potentially large) description to stay small, but
// flags has_description so the card can offer a lazy "hover to load" affordance;
// the full text comes from GET /tasks/:id/description.
func TestBoardListOmitsDescription(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	withDesc := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "С описанием", "column_id": s.col(t, 0), "description": "# Длинный markdown",
	}), http.StatusCreated)
	withID := withDesc["id"].(string)
	noDesc := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Без описания", "column_id": s.col(t, 0),
	}), http.StatusCreated)
	noID := noDesc["id"].(string)

	for _, row := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
		if _, present := row["description"]; present {
			t.Fatalf("board list leaked description: %v", row)
		}
		switch row["id"] {
		case withID:
			if row["has_description"] != true {
				t.Fatalf("task with description: has_description=%v", row["has_description"])
			}
		case noID:
			if row["has_description"] != false {
				t.Fatalf("task without description: has_description=%v", row["has_description"])
			}
		}
	}

	// The dedicated endpoint returns the full text on demand.
	got := c.expect(t, c.get("/tasks/"+withID+"/description"), http.StatusOK)
	if got["description"] != "# Длинный markdown" {
		t.Fatalf("description endpoint: %v", got)
	}
}

// A task carrying several tags AND several assignees at once is the case the
// board queries used to fan out on: joining both M:N tables produced a row per
// *combination* (3 tags × 2 assignees = 6 rows), collapsed again by array_agg.
// The LATERAL rewrite emits one row per task, so this pins down both halves of
// the contract — exactly one row per task, and the full sets in each array.
func TestBoardListAggregatesMultiValuedMeta(t *testing.T) {
	t.Parallel()
	c := signup(t)
	mate := signup(t)
	s := mkStack(t, c)
	c.expect(t, c.post("/workspaces/"+s.WS+"/members", map[string]any{"email": mate.Email}), http.StatusCreated)

	tagIDs := map[string]bool{}
	for _, name := range []string{"альфа", "бета", "гамма"} {
		tag := c.expect(t, c.post("/projects/"+s.Project+"/tags", map[string]any{"name": name}), http.StatusCreated)
		tagIDs[tag["id"].(string)] = true
	}
	wantAssignees := map[string]bool{c.UserID: true, mate.UserID: true}

	parent := mkTask(t, c, s.Board, s.col(t, 0), "Родитель")["id"].(string)
	child := mkTask(t, c, s.Board, s.col(t, 0), "Подзадача")["id"].(string)
	c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)

	// Same multi-valued meta on both, so the top-level and subtask queries are
	// each exercised with more than one row to aggregate.
	for _, id := range []string{parent, child} {
		for tagID := range tagIDs {
			c.expect(t, c.post("/tasks/"+id+"/tags", map[string]any{"tag_id": tagID}), http.StatusNoContent)
		}
		for userID := range wantAssignees {
			if r := c.post("/tasks/"+id+"/assignees", map[string]any{"user_id": userID}); r.Status != http.StatusNoContent {
				t.Fatalf("add assignee %s to %s: %d\n%s", userID, id, r.Status, r.Body)
			}
		}
	}

	check := func(what, path, wantID string) {
		t.Helper()
		var seen int
		for _, row := range c.get(path).listBody(t) {
			if row["id"] != wantID {
				continue
			}
			seen++
			gotTags, _ := row["tag_ids"].([]any)
			if len(gotTags) != len(tagIDs) {
				t.Fatalf("%s tag_ids = %v, want %d ids", what, row["tag_ids"], len(tagIDs))
			}
			for _, v := range gotTags {
				if !tagIDs[v.(string)] {
					t.Fatalf("%s unexpected tag id %v", what, v)
				}
			}
			gotAs, _ := row["assignee_ids"].([]any)
			if len(gotAs) != len(wantAssignees) {
				t.Fatalf("%s assignee_ids = %v, want %d ids", what, row["assignee_ids"], len(wantAssignees))
			}
			for _, v := range gotAs {
				if !wantAssignees[v.(string)] {
					t.Fatalf("%s unexpected assignee id %v", what, v)
				}
			}
		}
		// GROUP BY used to collapse the fanned-out rows; a LATERAL that lost its
		// aggregate would leak them into the response instead, so pin the count.
		if seen != 1 {
			t.Fatalf("%s appeared %d times in the listing, want exactly 1", what, seen)
		}
	}
	check("top-level task", "/boards/"+s.Board+"/tasks", parent)
	check("subtask", "/boards/"+s.Board+"/subtasks", child)
}

// Kanban move: to another column, and between two neighbours (midpoint position).
func TestTaskMoveBetweenNeighbours(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	a := mkTask(t, c, s.Board, s.col(t, 0), "А")
	b := mkTask(t, c, s.Board, s.col(t, 0), "Б")
	x := mkTask(t, c, s.Board, s.col(t, 1), "Х")

	// Plain column change. NOTE: without before_id/after_id the server does NOT
	// append — it assigns the default midpoint position (clients always pass
	// neighbours), so only the column change is asserted here.
	moved := c.expect(t, c.patch("/tasks/"+x["id"].(string)+"/move",
		map[string]any{"column_id": s.col(t, 0)}), http.StatusOK)
	if moved["column_id"] != s.col(t, 0) {
		t.Fatalf("move column: %v", moved)
	}

	// Between А and Б: position strictly between the neighbours.
	moved = c.expect(t, c.patch("/tasks/"+x["id"].(string)+"/move", map[string]any{
		"column_id": s.col(t, 0), "before_id": a["id"], "after_id": b["id"],
	}), http.StatusOK)
	if p := fpos(t, moved); p <= fpos(t, a) || p >= fpos(t, b) {
		t.Fatalf("between move: pos %v not in (%v, %v)", p, a["position"], b["position"])
	}

	// A column of another board is rejected.
	other := mkStack(t, c)
	if r := c.patch("/tasks/"+x["id"].(string)+"/move",
		map[string]any{"column_id": other.col(t, 0)}); r.Status != http.StatusBadRequest {
		t.Fatalf("foreign column move: %d", r.Status)
	}
}

// Archive → archive list, restore → board again, then hard delete.
func TestTaskArchiveRestoreDelete(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "В архив")
	id := task["id"].(string)

	if r := c.patch("/tasks/"+id+"/archive", nil); r.Status != http.StatusNoContent {
		t.Fatalf("archive: %d\n%s", r.Status, r.Body)
	}
	if hasID(c.get("/boards/"+s.Board+"/tasks").listBody(t), id) {
		t.Fatalf("archived task still on the board")
	}
	if !hasID(c.get("/boards/"+s.Board+"/archive").listBody(t), id) {
		t.Fatalf("archived task not in the archive list")
	}
	if !hasKind(eventKinds(t, c, id), "archived") {
		t.Fatalf("no archived journal event")
	}

	if r := c.patch("/tasks/"+id+"/restore", nil); r.Status != http.StatusNoContent {
		t.Fatalf("restore: %d\n%s", r.Status, r.Body)
	}
	if !hasID(c.get("/boards/"+s.Board+"/tasks").listBody(t), id) {
		t.Fatalf("restored task not back on the board")
	}
	if len(c.get("/boards/"+s.Board+"/archive").listBody(t)) != 0 {
		t.Fatalf("archive list not empty after restore")
	}

	if r := c.del("/tasks/" + id); r.Status != http.StatusNoContent {
		t.Fatalf("delete: %d\n%s", r.Status, r.Body)
	}
	if r := c.get("/tasks/" + id); r.Status != http.StatusNotFound {
		t.Fatalf("deleted task GET: %d", r.Status)
	}
}

// Subtask tree: attach/detach via /parent, board-wide subtask list, cycle guards.
func TestSubtaskTree(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	parent := mkTask(t, c, s.Board, s.col(t, 0), "Родитель")
	child := mkTask(t, c, s.Board, s.col(t, 1), "Ребёнок")
	pid, cid := parent["id"].(string), child["id"].(string)

	// Attach: the child inherits the parent's column.
	att := c.expect(t, c.patch("/tasks/"+cid+"/parent", map[string]any{"parent_id": pid}), http.StatusOK)
	if att["parent_id"] != pid || att["column_id"] != s.col(t, 0) {
		t.Fatalf("attach: %v", att)
	}

	// Parent's GET now lists the child; the board subtask list carries it too.
	full := c.expect(t, c.get("/tasks/"+pid), http.StatusOK)
	subs, _ := full["subtasks"].([]any)
	if len(subs) != 1 || subs[0].(map[string]any)["id"] != cid {
		t.Fatalf("parent subtasks: %v", full["subtasks"])
	}
	if !hasID(c.get("/boards/"+s.Board+"/subtasks").listBody(t), cid) {
		t.Fatalf("board subtask list missing the child")
	}

	// Cycle guards: self-parent and parenting to one's own child → 400.
	if r := c.patch("/tasks/"+pid+"/parent", map[string]any{"parent_id": pid}); r.Status != http.StatusBadRequest {
		t.Fatalf("self parent: %d", r.Status)
	}
	if r := c.patch("/tasks/"+pid+"/parent", map[string]any{"parent_id": cid}); r.Status != http.StatusBadRequest {
		t.Fatalf("cyclic parent: %d", r.Status)
	}

	// Detach: back to a top-level card.
	det := c.expect(t, c.patch("/tasks/"+cid+"/parent", map[string]any{"parent_id": nil}), http.StatusOK)
	if det["parent_id"] != nil {
		t.Fatalf("detach: %v", det)
	}
}

// Assignees: add a workspace member, see them on the task, remove them.
func TestTaskAssignees(t *testing.T) {
	t.Parallel()
	c := signup(t)
	member := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "С исполнителем")
	id := task["id"].(string)

	// The second user must be a workspace member first.
	c.expect(t, c.post("/workspaces/"+s.WS+"/members", map[string]any{"email": member.Email}), http.StatusCreated)

	if r := c.post("/tasks/"+id+"/assignees", map[string]any{"user_id": member.UserID}); r.Status != http.StatusNoContent {
		t.Fatalf("add assignee: %d\n%s", r.Status, r.Body)
	}
	full := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
	as, _ := full["assignees"].([]any)
	if len(as) != 1 || as[0].(map[string]any)["id"] != member.UserID {
		t.Fatalf("assignees: %v", full["assignees"])
	}
	if !hasKind(eventKinds(t, c, id), "assigned") {
		t.Fatalf("no assigned journal event")
	}

	if r := c.del("/tasks/" + id + "/assignees/" + member.UserID); r.Status != http.StatusNoContent {
		t.Fatalf("remove assignee: %d\n%s", r.Status, r.Body)
	}
	full = c.expect(t, c.get("/tasks/"+id), http.StatusOK)
	if as, _ = full["assignees"].([]any); len(as) != 0 {
		t.Fatalf("assignees after remove: %v", full["assignees"])
	}
}

// Eisenhower quadrant override: set, validate range, clear.
func TestTaskEisenhower(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "Матрица")["id"].(string)

	m := c.expect(t, c.patch("/tasks/"+id+"/eisenhower", map[string]any{"quadrant": 2}), http.StatusOK)
	if m["eisenhower_quadrant"] != float64(2) {
		t.Fatalf("set quadrant: %v", m)
	}
	if r := c.patch("/tasks/"+id+"/eisenhower", map[string]any{"quadrant": 5}); r.Status != http.StatusBadRequest {
		t.Fatalf("out-of-range quadrant: %d", r.Status)
	}
	m = c.expect(t, c.patch("/tasks/"+id+"/eisenhower", map[string]any{"quadrant": nil}), http.StatusOK)
	if m["eisenhower_quadrant"] != nil {
		t.Fatalf("clear quadrant: %v", m)
	}
}

// Transfer to another board of the same workspace (subtasks follow); another
// workspace is forbidden.
func TestTaskTransfer(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	board2 := mkBoard(t, c, s.Project, "Вторая доска")
	cols2 := c.get("/boards/" + board2 + "/columns").listBody(t)

	parent := mkTask(t, c, s.Board, s.col(t, 0), "Переезжает")
	pid := parent["id"].(string)
	sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Подзадача едет следом", "column_id": s.col(t, 0), "parent_id": pid,
	}), http.StatusCreated)

	// Without column_id the task lands in the target board's first column.
	tr := c.expect(t, c.patch("/tasks/"+pid+"/transfer", map[string]any{"board_id": board2}), http.StatusOK)
	if tr["board_id"] != board2 || tr["column_id"] != cols2[0]["id"] {
		t.Fatalf("transfer target: %v", tr)
	}
	moved := c.expect(t, c.get("/tasks/"+sub["id"].(string)), http.StatusOK)
	if moved["board_id"] != board2 || moved["parent_id"] != pid {
		t.Fatalf("subtask after transfer: %v", moved)
	}

	// Explicit target column on the target board.
	tr = c.expect(t, c.patch("/tasks/"+pid+"/transfer",
		map[string]any{"board_id": s.Board, "column_id": s.col(t, 2)}), http.StatusOK)
	if tr["board_id"] != s.Board || tr["column_id"] != s.col(t, 2) {
		t.Fatalf("transfer with column: %v", tr)
	}

	// A board in another workspace → 403; a column not on the target board → 400.
	foreign := mkStack(t, c) // same user, different workspace
	if r := c.patch("/tasks/"+pid+"/transfer", map[string]any{"board_id": foreign.Board}); r.Status != http.StatusForbidden {
		t.Fatalf("cross-workspace transfer: %d", r.Status)
	}
	if r := c.patch("/tasks/"+pid+"/transfer",
		map[string]any{"board_id": board2, "column_id": s.col(t, 0)}); r.Status != http.StatusBadRequest {
		t.Fatalf("foreign column transfer: %d", r.Status)
	}
}

// Recurrence: rule round-trip through update/GET, wipe-on-omission (full-replace
// fact), and a complete-triggered advance (due steps forward, task reopens).
func TestTaskRecurrence(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	due := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Повторяющаяся", "column_id": s.col(t, 0), "due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	id := task["id"].(string)

	// Attach a weekly schedule-triggered rule; the server stores the canonical form.
	up := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Повторяющаяся", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 2, "trigger": "schedule"},
	}), http.StatusOK)
	rec, ok := up["recurrence"].(map[string]any)
	if !ok || rec["freq"] != "weekly" || rec["interval"] != float64(2) || rec["trigger"] != "schedule" {
		t.Fatalf("recurrence stored: %v", up["recurrence"])
	}
	// …and GET returns it back.
	full := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
	if rec, ok = full["recurrence"].(map[string]any); !ok || rec["freq"] != "weekly" {
		t.Fatalf("recurrence in GET: %v", full["recurrence"])
	}

	// An invalid rule is treated as "no recurrence" (stored NULL, not 400).
	up = c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Повторяющаяся", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "hourly"},
	}), http.StatusOK)
	if up["recurrence"] != nil {
		t.Fatalf("invalid rule kept: %v", up["recurrence"])
	}

	// Tri-state: omitting recurrence PRESERVES the rule (it used to wipe it),
	// explicit null is what clears it.
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Повторяющаяся", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily"},
	}), http.StatusOK)
	up = c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Повторяющаяся", "due_date": due.Format(time.RFC3339),
	}), http.StatusOK)
	if rec, ok = up["recurrence"].(map[string]any); !ok || rec["freq"] != "daily" {
		t.Fatalf("omitted recurrence should be preserved: %v", up["recurrence"])
	}
	up = c.expect(t, c.patch("/tasks/"+id, map[string]any{"recurrence": nil}), http.StatusOK)
	if up["recurrence"] != nil {
		t.Fatalf("explicit null should clear recurrence: %v", up["recurrence"])
	}

	// Complete-triggered advance: completing the task reschedules it instead —
	// due moves +1 day, completion is cleared, journal logs "recurred".
	adv := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Повторяющаяся", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	if adv["completed_at"] != nil {
		t.Fatalf("recurring task stayed completed: %v", adv)
	}
	if got := parseTS(t, adv["due_date"]); !got.Equal(due.AddDate(0, 0, 1)) {
		t.Fatalf("advanced due = %v, want %v", got, due.AddDate(0, 0, 1))
	}
	if rec, ok = adv["recurrence"].(map[string]any); !ok || rec["freq"] != "daily" {
		t.Fatalf("rule lost on advance: %v", adv["recurrence"])
	}
	if !hasKind(eventKinds(t, c, id), "recurred") {
		t.Fatalf("no recurred journal event")
	}
}
