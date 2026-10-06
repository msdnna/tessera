// A recurrence written on a checklist item keeps the item in the checklist.
package main

import (
	"net/http"
	"testing"
	"time"
)

// The create_new duplicate of a recurring subtask must hang off the same parent.
//
// The recurrence editor is the same one for a card and for a checklist item — the
// modal opens on a subtask exactly the way it opens on a task, and the DueEditor
// inside it has no idea which it is. So "каждую неделю, создавать копию" is
// writable on an item of a checklist, and the duplicate used to be born as a root
// card: the parent was left showing only the spent occurrence with its box ticked,
// the next one stood alone in the board's first column under the same title, and
// nothing in the subtask counts, the progress bar or liftSubtasksFromDone could
// see it any more. From the checklist's side the item simply stopped coming back.
func TestRecurringSubtaskCloneKeepsItsParent(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	const itemTitle = "Свести цифры"

	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": s.col(t, 0),
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	item := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": itemTitle, "column_id": s.col(t, 0), "parent_id": parentID,
		"due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	itemID := item["id"].(string)

	c.expect(t, c.patch("/tasks/"+itemID, map[string]any{
		"title": itemTitle, "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	// The spent occurrence stays in the checklist, and the next one joins it there.
	detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	var cloneID string
	for _, raw := range detail["subtasks"].([]any) {
		m, ok := raw.(map[string]any)
		if ok && m["title"] == itemTitle && m["id"] != itemID {
			cloneID = m["id"].(string)
		}
	}
	if cloneID == "" {
		// Name where it actually went — a root card on the board — so the failure
		// reads as "it left the checklist", not "it was never created".
		for _, m := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
			if m["title"] == itemTitle {
				t.Fatalf("the next occurrence left the checklist and became a root card %s on the board", m["id"])
			}
		}
		t.Fatalf("no next occurrence of %q among the parent's subtasks: %v", itemTitle, detail["subtasks"])
	}

	clone := c.expect(t, c.get("/tasks/"+cloneID), http.StatusOK)
	if clone["parent_id"] != parentID {
		t.Fatalf("clone parent_id = %v, want %s", clone["parent_id"], parentID)
	}
	if got, want := parseTS(t, clone["due_date"]), due.AddDate(0, 0, 7); !got.Equal(want) {
		t.Fatalf("clone due = %v, want %v", got, want)
	}
	if clone["completed_at"] != nil {
		t.Fatalf("the next occurrence arrived already completed: %v", clone["completed_at"])
	}
	// A recurring *card* is still a card: the parentless case must not pick up a
	// parent from anywhere (it has none to inherit).
	root := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0),
		"due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	c.expect(t, c.patch("/tasks/"+root["id"].(string), map[string]any{
		"title": "Ежедневный статус", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)
	rootClone := findClone(t, c, s.Board, root["id"].(string), "Ежедневный статус")
	if rootClone["parent_id"] != nil {
		t.Fatalf("the duplicate of a root task grew a parent: %v", rootClone["parent_id"])
	}
}
