package main

import (
	"net/http"
	"strconv"
	"testing"
)

// taskNumber reads the per-workspace #N of a task.
func taskNumber(t *testing.T, c *client, taskID string) float64 {
	t.Helper()
	m := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	n, ok := m["number"].(float64)
	if !ok {
		t.Fatalf("task %s has no number: %v", taskID, m)
	}
	return n
}

// TestTransferProjectRenumbersTasks pins the one thing a project transfer used to
// carry across the workspace boundary unchanged: a task's #N belongs to a
// *workspace* (workspaces.task_counter issues it), and the arriving tasks kept
// the numbers the source workspace had handed them.
//
// Nothing in the schema can catch that. tasks has no workspace_id — the number is
// scoped through boards → projects → workspaces — so there is no unique index to
// violate, and GetTaskByNumber is a `:one` with neither LIMIT nor ORDER BY: with
// two matches pgx scans whichever row the plan produced first and reports no
// error. So "#42" stops naming a task and starts naming a coin flip, for the
// workspace's *own* task as much as for the arriving one: a #-link, `/blocks #42`,
// `/parent #42` — each can land on the wrong task, and /parent lands a subtree there.
//
// The target's counter is untouched by the transfer too, so it goes on issuing
// numbers the arrivals already hold: the collisions keep being minted long after
// the transfer is forgotten.
//
// TransferTask states the invariant plainly for the single-task door — it refuses
// a target board in another workspace outright — and the project door is the only
// one that carries tasks across at all.
func TestTransferProjectRenumbersTasks(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// Both workspaces are fresh, so each counter issues #1 first: the resident
	// task and the arriving one collide on the cheapest possible setup.
	resident := mkTask(t, owner, dst.Board, dst.col(t, 0), "родная задача")["id"].(string)
	moving := mkTask(t, owner, src.Board, src.col(t, 0), "переезжающая задача")["id"].(string)
	child := owner.expect(t, owner.post("/boards/"+src.Board+"/tasks", map[string]any{
		"title": "её подзадача", "column_id": src.col(t, 0), "parent_id": moving,
	}), http.StatusCreated)["id"].(string)

	owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	byNumber := func(n float64) string {
		t.Helper()
		m := owner.expect(t, owner.get("/workspaces/"+dst.WS+"/tasks/by-number/"+
			strconv.FormatFloat(n, 'f', -1, 64)), http.StatusOK)
		id, _ := m["id"].(string)
		return id
	}

	t.Run("no two tasks share a number", func(t *testing.T) {
		seen := map[float64]string{}
		for _, id := range []string{resident, moving, child} {
			n := taskNumber(t, owner, id)
			if other, dup := seen[n]; dup {
				t.Fatalf("tasks %s and %s both carry #%v in one workspace", other, id, n)
			}
			seen[n] = id
		}
	})

	t.Run("by-number resolves each task to itself", func(t *testing.T) {
		for _, id := range []string{resident, moving, child} {
			n := taskNumber(t, owner, id)
			if got := byNumber(n); got != id {
				t.Fatalf("#%v resolves to %s, want %s", n, got, id)
			}
		}
	})

	t.Run("the counter keeps issuing free numbers", func(t *testing.T) {
		// Both boards now live in the target workspace and draw from its counter.
		for _, s := range []stack{dst, src} {
			fresh := mkTask(t, owner, s.Board, s.col(t, 0), "следующая задача")["id"].(string)
			n := taskNumber(t, owner, fresh)
			if got := byNumber(n); got != fresh {
				t.Fatalf("task created after the transfer got #%v, which resolves to %s", n, got)
			}
		}
	})

	t.Run("the arrivals keep their relative order", func(t *testing.T) {
		// Renumbering is a re-stamp, not a shuffle: a task created earlier in the
		// project still carries the lower number afterwards.
		if taskNumber(t, owner, moving) >= taskNumber(t, owner, child) {
			t.Fatalf("#%v (parent) should sort before #%v (its subtask)",
				taskNumber(t, owner, moving), taskNumber(t, owner, child))
		}
	})
}
