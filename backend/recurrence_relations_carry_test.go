// A repeating task's links have to reach the next occurrence (#2932).
//
// Every column of `tasks` is carried onto a create_new duplicate by now — the
// stage/quadrant pins, the due-notify overrides, each item's own repeat rule.
// The relations are not: they live in a table of their own (task_relations), and
// the clone is built by CreateTask, which knows nothing about it. The reschedule
// branch keeps them for free, as it keeps everything else — it edits the same row
// and the relation rows still point at it.
//
// What the user sees is worse than an empty «Связи» tab, because a relation is
// ONE directed row that BOTH ends display (ListTaskRelations reads either
// direction). So the link does not disappear — it stays attached to the spent
// occurrence: «Выкатить релиз» goes on saying it is blocked by last week's
// finished «Еженедельный отчёт», the Gantt keeps drawing the arrow to a card in
// «Готово», and the live occurrence standing on the board is linked to nothing.
// From the user's side the dependency silently stopped tracking the work.
//
// A relation the GitLab projection wrote is deliberately NOT copied: those edges
// are reprojected from the issue graph (gitlab_relations.go), and a duplicate's
// own copy would be a user-source row the sync can neither recognise nor remove.
package main

import (
	"net/http"
	"testing"
	"time"
)

// relKindByNumber maps a /relations payload to «other end's number → kind», the
// pair the «Связи» tab actually prints.
func relKindByNumber(t *testing.T, rows []map[string]any) map[float64]string {
	t.Helper()
	out := map[float64]string{}
	for _, r := range rows {
		n, _ := r["related_number"].(float64)
		k, _ := r["kind"].(string)
		out[n] = k
	}
	return out
}

func TestRecurrenceCarriesTaskRelations(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ── the create_new branch ──────────────────────────────────────────
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)
	origNum, _ := orig["number"].(float64)

	// One link made FROM the repeating task…
	spec := mkTask(t, c, s.Board, todo, "Регламент отчётности")
	specNum, _ := spec["number"].(float64)
	c.expect(t, c.post("/tasks/"+origID+"/relations", map[string]any{
		"number": specNum, "kind": "relates",
	}), http.StatusCreated)

	// …and one made TOWARDS it from another card: this is the half that rots.
	// «Выкатить релиз» is blocked by the report, and the row belongs to the
	// release — nothing on its side changes when the report repeats.
	release := mkTask(t, c, s.Board, todo, "Выкатить релиз")
	releaseID := release["id"].(string)
	releaseNum, _ := release["number"].(float64)
	c.expect(t, c.post("/tasks/"+releaseID+"/relations", map[string]any{
		"number": origNum, "kind": "blocked_by",
	}), http.StatusCreated)

	// A checklist item with a link of its own — the clone rebuilds the whole
	// checklist, so an item's relations go the same way the item's tags,
	// assignees and repeat rule already do.
	step := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свериться с бухгалтерией", "column_id": todo, "parent_id": origID,
	}), http.StatusCreated)
	stepID := step["id"].(string)
	c.expect(t, c.post("/tasks/"+stepID+"/relations", map[string]any{
		"number": specNum, "kind": "relates",
	}), http.StatusCreated)

	if rows := c.get("/tasks/" + origID + "/relations").listBody(t); len(rows) != 2 {
		t.Fatalf("fixture: the first occurrence shows %d relations, want 2\n%v", len(rows), rows)
	}

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	cloneID := clone["id"].(string)
	cloneNum, _ := clone["number"].(float64)

	kinds := relKindByNumber(t, c.get("/tasks/"+cloneID+"/relations").listBody(t))
	if len(kinds) != 2 {
		t.Fatalf("the next occurrence shows %d relations, want 2 — the links stayed on the spent copy\n%v", len(kinds), kinds)
	}
	if got := kinds[specNum]; got != "relates" {
		t.Fatalf("link to the reference doc came over as %q, want %q", got, "relates")
	}
	// The incoming half keeps its direction: seen from the report, the release
	// is the thing it blocks.
	if got := kinds[releaseNum]; got != "blocks" {
		t.Fatalf("link from «Выкатить релиз» came over as %q, want %q", got, "blocks")
	}

	// And the other end now points at the live occurrence, which is the whole
	// point of carrying the link.
	relSide := relKindByNumber(t, c.get("/tasks/"+releaseID+"/relations").listBody(t))
	if got := relSide[cloneNum]; got != "blocked_by" {
		t.Fatalf("«Выкатить релиз» does not see the new occurrence: %v", relSide)
	}

	cloneStep := subtaskByTitle(t, clone, "Свериться с бухгалтерией")
	stepKinds := relKindByNumber(t, c.get("/tasks/"+cloneStep["id"].(string)+"/relations").listBody(t))
	if got := stepKinds[specNum]; got != "relates" {
		t.Fatalf("the duplicate's checklist item lost its own link: %v", stepKinds)
	}

	// The spent occurrence keeps its links — the duplicate copied them, it did
	// not move them. Its «Связи» tab is the record of what that week depended on.
	if rows := c.get("/tasks/" + origID + "/relations").listBody(t); len(rows) != 2 {
		t.Fatalf("the first occurrence lost its relations to the duplicate: %v", rows)
	}

	// ── the reschedule branch: the control, green before the fix and after ──
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	c.expect(t, c.post("/tasks/"+dailyID+"/relations", map[string]any{
		"number": specNum, "kind": "relates",
	}), http.StatusCreated)
	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	if rows := c.get("/tasks/" + dailyID + "/relations").listBody(t); len(rows) != 1 {
		t.Fatalf("a reschedule dropped the task's relations: %v", rows)
	}
}

// TestRecurrenceAnnouncesCarriedRelations covers the half a re-fetch cannot
// supply: the Gantt keeps its edge list in a fetch of its own and re-reads it on
// `task.relation` alone, so a chart left open over the tick has to hear that the
// arrow moved to the new occurrence — `task.created` refreshes the bars only.
func TestRecurrenceAnnouncesCarriedRelations(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	origID := orig["id"].(string)
	release := mkTask(t, c, s.Board, todo, "Выкатить релиз")
	c.expect(t, c.post("/tasks/"+origID+"/relations", map[string]any{
		"number": release["number"], "kind": "blocks",
	}), http.StatusCreated)

	conn, status := dialWS(t, c.token, false)
	if conn == nil {
		t.Fatalf("handshake failed (status %d)", status)
	}
	defer conn.Close()

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	ev := awaitWSEvent(t, conn, "task.relation", 5*time.Second)
	if ev == nil {
		t.Fatal("the carried link was never announced — an open Gantt keeps its arrow on the spent occurrence")
	}
	data, _ := ev["data"].(map[string]any)
	if data["task_id"] == origID {
		t.Fatalf("the event names the spent occurrence, not the new one: %v", data)
	}
	if data["kind"] != "blocks" {
		t.Fatalf("kind = %v, want blocks: %v", data["kind"], data)
	}
}
