package main

import (
	"net/http"
	"testing"
)

// TestTagMustBeInSameWorkspace pins the authorization of the tag_id a caller
// sends in the *body* of POST /tasks/:id/tags. loadTask vouches for the task in
// the route, not for the tag: an unchecked tag_id attaches a stranger's tag to
// the caller's own task, and the task detail then reads back that tag's name
// and colour — a cross-workspace read of data the caller was never shown.
//
// The quick-action arm (`/tag <name>`) resolves names inside the task's own
// project and was never affected; the hole is the REST endpoint.
func TestTagMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	vs := mkStack(t, victim)
	victimTag := victim.expect(t, victim.post("/projects/"+vs.Project+"/tags",
		map[string]any{"name": "секрет-релиза", "color": "#ff0000"}), http.StatusCreated)
	victimTagID := victimTag["id"].(string)

	attacker := signup(t) // member of nothing the victim owns
	as := mkStack(t, attacker)
	mine := mkTask(t, attacker, as.Board, as.col(t, 0), "своя задача")
	mineID := mine["id"].(string)

	t.Run("attaching a foreign tag is refused", func(t *testing.T) {
		r := attacker.post("/tasks/"+mineID+"/tags", map[string]any{"tag_id": victimTagID})
		if r.Status != http.StatusForbidden {
			t.Fatalf("attach foreign tag: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("the foreign tag name does not leak into the task", func(t *testing.T) {
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		if tags, _ := got["tags"].([]any); len(tags) != 0 {
			t.Fatalf("task gained %d tags: %v", len(tags), tags)
		}
	})

	t.Run("detaching a foreign tag is refused too", func(t *testing.T) {
		r := attacker.del("/tasks/" + mineID + "/tags/" + victimTagID)
		if r.Status != http.StatusForbidden {
			t.Fatalf("detach foreign tag: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("an unknown tag_id is a 400", func(t *testing.T) {
		r := attacker.post("/tasks/"+mineID+"/tags",
			map[string]any{"tag_id": "00000000-0000-0000-0000-000000000001"})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("attach unknown tag: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("a tag from the caller's own workspace still attaches", func(t *testing.T) {
		own := attacker.expect(t, attacker.post("/projects/"+as.Project+"/tags",
			map[string]any{"name": "свой"}), http.StatusCreated)
		ownID := own["id"].(string)
		attacker.expect(t, attacker.post("/tasks/"+mineID+"/tags",
			map[string]any{"tag_id": ownID}), http.StatusNoContent)
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		if tags, _ := got["tags"].([]any); len(tags) != 1 {
			t.Fatalf("own tag did not attach: %v", got["tags"])
		}
		attacker.expect(t, attacker.del("/tasks/"+mineID+"/tags/"+ownID), http.StatusNoContent)
	})
}

// TestMilestoneMustBeInSameWorkspace pins the same check for the milestone_id
// in the body of POST /tasks/:id/milestone. The rollup on the «Этапы» screen
// (ListWorkspaceMilestones) counts every task carrying the milestone id with no
// workspace filter of its own, so an unchecked milestone_id lets an outsider
// inflate a stranger's milestone progress — their task_count, done_count and
// estimate sum — from a task the victim can never see or unpin.
func TestMilestoneMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	vs := mkStack(t, victim)
	victimMs := victim.expect(t, victim.post("/projects/"+vs.Project+"/milestones",
		map[string]any{"title": "Релиз 1.0"}), http.StatusCreated)
	victimMsID := victimMs["id"].(string)

	attacker := signup(t)
	as := mkStack(t, attacker)
	mine := mkTask(t, attacker, as.Board, as.col(t, 0), "своя задача")
	mineID := mine["id"].(string)

	t.Run("pinning a foreign milestone is refused", func(t *testing.T) {
		r := attacker.post("/tasks/"+mineID+"/milestone",
			map[string]any{"milestone_id": victimMsID})
		if r.Status != http.StatusForbidden {
			t.Fatalf("pin foreign milestone: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("the task keeps no milestone", func(t *testing.T) {
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		if got["milestone_id"] != nil {
			t.Fatalf("milestone_id = %v, want nil", got["milestone_id"])
		}
	})

	t.Run("the victim's milestone rollup stays empty", func(t *testing.T) {
		list := victim.get("/workspaces/" + vs.WS + "/milestones").listBody(t)
		for _, m := range list {
			if m["id"] != victimMsID {
				continue
			}
			if n, _ := m["task_count"].(float64); n != 0 {
				t.Fatalf("victim milestone task_count = %v, want 0 (%v)", m["task_count"], m)
			}
			return
		}
		t.Fatalf("victim milestone %s missing from the workspace list: %v", victimMsID, list)
	})

	t.Run("an unknown milestone_id is a 400", func(t *testing.T) {
		r := attacker.post("/tasks/"+mineID+"/milestone",
			map[string]any{"milestone_id": "00000000-0000-0000-0000-000000000001"})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("pin unknown milestone: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("a milestone from the caller's own workspace still pins", func(t *testing.T) {
		own := attacker.expect(t, attacker.post("/projects/"+as.Project+"/milestones",
			map[string]any{"title": "Свой этап"}), http.StatusCreated)
		ownID := own["id"].(string)
		attacker.expect(t, attacker.post("/tasks/"+mineID+"/milestone",
			map[string]any{"milestone_id": ownID}), http.StatusNoContent)
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		if got["milestone_id"] != ownID {
			t.Fatalf("milestone_id = %v, want %s", got["milestone_id"], ownID)
		}
		attacker.expect(t, attacker.del("/tasks/"+mineID+"/milestone"), http.StatusNoContent)
	})
}
