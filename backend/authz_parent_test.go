package main

import (
	"net/http"
	"testing"
)

// TestParentMustBeInSameWorkspace pins the authorization of the parent_id a
// caller sends in the *body*. The route's :id param is checked by loadTask, but
// parent_id is a second reference to a task the caller may have no rights to —
// and re-parenting adopts the parent's board and column, so an unchecked
// parent_id physically moves the caller's task onto a stranger's board.
func TestParentMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	vs := mkStack(t, victim)
	victimTask := mkTask(t, victim, vs.Board, vs.col(t, 0), "чужая задача")
	victimTaskID := victimTask["id"].(string)

	attacker := signup(t) // member of nothing the victim owns
	as := mkStack(t, attacker)
	mine := mkTask(t, attacker, as.Board, as.col(t, 0), "своя задача")
	mineID := mine["id"].(string)

	t.Run("reparent onto a foreign task is refused", func(t *testing.T) {
		r := attacker.patch("/tasks/"+mineID+"/parent",
			map[string]any{"parent_id": victimTaskID})
		if r.Status != http.StatusForbidden {
			t.Fatalf("reparent onto foreign task: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("the task stays on its own board", func(t *testing.T) {
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		if got["board_id"] != as.Board {
			t.Fatalf("board_id = %v, want own board %s", got["board_id"], as.Board)
		}
		if got["parent_id"] != nil {
			t.Fatalf("parent_id = %v, want nil", got["parent_id"])
		}
	})

	t.Run("the foreign task gains no subtask", func(t *testing.T) {
		got := victim.expect(t, victim.get("/tasks/"+victimTaskID), http.StatusOK)
		subs, _ := got["subtasks"].([]any)
		if len(subs) != 0 {
			t.Fatalf("victim task gained %d subtasks: %v", len(subs), subs)
		}
	})

	t.Run("create with a foreign parent_id is refused", func(t *testing.T) {
		r := attacker.post("/boards/"+as.Board+"/tasks", map[string]any{
			"title":     "подсадка",
			"column_id": as.col(t, 0),
			"parent_id": victimTaskID,
		})
		if r.Status != http.StatusForbidden {
			t.Fatalf("create under foreign parent: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("an unknown parent_id is still a 400", func(t *testing.T) {
		r := attacker.patch("/tasks/"+mineID+"/parent",
			map[string]any{"parent_id": "00000000-0000-0000-0000-000000000001"})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("reparent onto unknown task: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("reparent within the same workspace still works", func(t *testing.T) {
		sibling := mkTask(t, attacker, as.Board, as.col(t, 1), "родитель")
		siblingID := sibling["id"].(string)
		got := attacker.expect(t, attacker.patch("/tasks/"+mineID+"/parent",
			map[string]any{"parent_id": siblingID}), http.StatusOK)
		if got["parent_id"] != siblingID {
			t.Fatalf("parent_id = %v, want %s", got["parent_id"], siblingID)
		}
	})
}
