// Учёт времени (#2965): entries on a task, the own and subtree sums in the task
// detail, /spend in a comment, and who may remove an entry.
package main

import (
	"net/http"
	"testing"
)

func TestTimeEntries(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	mate := signup(t)
	st := mkStack(t, owner)
	owner.expect(t, owner.post("/workspaces/"+st.WS+"/members",
		map[string]any{"email": mate.Email, "role": "member"}), http.StatusCreated)
	col := st.Columns[0]["id"].(string)
	parent := mkTask(t, owner, st.Board, col, "Учёт времени")["id"].(string)
	child := owner.expect(t, owner.post("/boards/"+st.Board+"/tasks", map[string]any{
		"column_id": col, "title": "Подзадача", "parent_id": parent,
	}), http.StatusCreated)["id"].(string)

	e1 := owner.expect(t, owner.post("/tasks/"+parent+"/time",
		map[string]any{"minutes": 90, "note": "вёрстка", "spent_on": "2026-10-10"}), http.StatusCreated)
	if e1["minutes"].(float64) != 90 || e1["note"] != "вёрстка" {
		t.Fatalf("entry: %v", e1)
	}
	mate.expect(t, mate.post("/tasks/"+child+"/time", map[string]any{"minutes": 30}), http.StatusCreated)
	for _, bad := range []map[string]any{{"minutes": 0}, {"minutes": 100001}, {"minutes": 5, "spent_on": "вчера"}} {
		if r := owner.post("/tasks/"+parent+"/time", bad); r.Status != http.StatusBadRequest {
			t.Fatalf("bad entry %v: status %d", bad, r.Status)
		}
	}

	// /spend in a comment logs the comment author's time.
	owner.expect(t, owner.post("/tasks/"+parent+"/comments", map[string]any{"body": "/spend 1h15m"}), http.StatusOK)

	d := owner.expect(t, owner.get("/tasks/"+parent), http.StatusOK)
	if d["spent"].(float64) != 165 || d["spent_total"].(float64) != 195 {
		t.Fatalf("sums: spent=%v total=%v, want 165 / 195", d["spent"], d["spent_total"])
	}
	list := owner.expect(t, owner.get("/tasks/"+parent+"/time"), http.StatusOK)
	if entries, _ := list["entries"].([]any); len(entries) != 2 {
		t.Fatalf("entries: %v", list["entries"])
	}

	// The author or a workspace admin removes an entry; a member can't remove another's.
	e1ID := e1["id"].(string)
	if r := mate.del("/tasks/" + parent + "/time/" + e1ID); r.Status != http.StatusForbidden {
		t.Fatalf("member removing another's entry: status %d, want 403", r.Status)
	}
	owner.expect(t, owner.del("/tasks/"+parent+"/time/"+e1ID), http.StatusNoContent)
	if r := owner.del("/tasks/" + parent + "/time/" + e1ID); r.Status != http.StatusNotFound {
		t.Fatalf("remove twice: status %d, want 404", r.Status)
	}
	d = owner.expect(t, owner.get("/tasks/"+parent), http.StatusOK)
	if d["spent"].(float64) != 75 {
		t.Fatalf("after removal spent=%v, want 75", d["spent"])
	}
}
