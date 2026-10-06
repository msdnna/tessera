package main

import (
	"net/http"
	"testing"
)

// Attaching and detaching a tag belongs in the task's journal — the schema of
// task_events has listed `tagged | untagged` among its kinds since 0006_collab,
// but applyTag only broadcast and never wrote a row.
//
// The gap is worst on the product's own killer feature: a board GROUPED BY TAGS
// draws one column per tag, so dragging a card across it is an attach plus a
// detach and the card's column never changes — no `moved` entry either. The card
// visibly travelled and «История» stayed empty, while a due date, an estimate,
// an assignee or a comment each left a line.
//
// The entry records the tag by NAME: the M:N row dies with the tag, so an id
// would leave old history pointing at nothing.
func TestTagChangeIsJournalled(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	tag := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "срочно", "color": "#ff0000"}), http.StatusCreated)
	tagID := tag["id"].(string)

	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача с тегами")
	id := task["id"].(string)

	c.expect(t, c.post("/tasks/"+id+"/tags", map[string]any{"tag_id": tagID}), http.StatusNoContent)
	if data := lastTagEvent(t, c, id, "tagged"); data["tag"] != "срочно" {
		t.Fatalf("attach journal = %v, want the tag's name", data)
	}

	// Re-attaching the tag it already carries changes nothing (the insert is
	// ON CONFLICT DO NOTHING), so it must not add a second line — the same rule
	// AddTaskTagSourced already counts rows for on the GitLab side.
	c.expect(t, c.post("/tasks/"+id+"/tags", map[string]any{"tag_id": tagID}), http.StatusNoContent)
	if n := countEvents(t, c, id, "tagged"); n != 1 {
		t.Fatalf("tagged entries after a repeated attach = %d, want 1", n)
	}

	c.expect(t, c.del("/tasks/"+id+"/tags/"+tagID), http.StatusNoContent)
	if data := lastTagEvent(t, c, id, "untagged"); data["tag"] != "срочно" {
		t.Fatalf("detach journal = %v, want the tag's name", data)
	}

	// Detaching a tag the task does not carry deletes no row and says nothing.
	c.expect(t, c.del("/tasks/"+id+"/tags/"+tagID), http.StatusNoContent)
	if n := countEvents(t, c, id, "untagged"); n != 1 {
		t.Fatalf("untagged entries after a repeated detach = %d, want 1", n)
	}
}

// lastTagEvent returns the data payload of the newest journal entry of a kind.
func lastTagEvent(t *testing.T, c *client, taskID, kind string) map[string]any {
	t.Helper()
	events := c.get("/tasks/" + taskID + "/events").listBody(t)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i]["kind"] != kind {
			continue
		}
		data, _ := events[i]["data"].(map[string]any)
		if data == nil {
			t.Fatalf("%s event has no data: %v", kind, events[i])
		}
		return data
	}
	t.Fatalf("no %s event on task %s: %v", kind, taskID, events)
	return nil
}

func countEvents(t *testing.T, c *client, taskID, kind string) int {
	t.Helper()
	n := 0
	for _, e := range c.get("/tasks/" + taskID + "/events").listBody(t) {
		if e["kind"] == kind {
			n++
		}
	}
	return n
}
