package main

import (
	"net/http"
	"testing"
)

// TestTransferProjectSeversCrossWorkspaceRefs pins the references a project
// transfer left hanging across the workspace boundary it just created (#2932).
//
// Two of a task's links are authorized once, when they are made: a relation
// (AddRelation resolves the target through GetTaskByNumber scoped to the
// caller's workspace) and a milestone (applyMilestone refuses a milestone from
// another workspace — its comment spells out the harm). A project transfer
// re-stamps workspace_id under both of them and re-checks neither, so a link
// that was legal when it was made stays in place while its two ends end up in
// workspaces that share no members:
//
//   - ListTaskRelations joins tasks for related_number/related_title with no
//     workspace filter of its own, so both sides go on reading the other's task
//     titles — the one thing the relation door refuses to hand out.
//   - ListWorkspaceMilestones rolls a milestone up over `tasks t ON
//     t.milestone_id = m.id`, also unfiltered: the stranded task keeps inflating
//     the count and the Σ estimate of a milestone in a workspace it is not in.
//
// Parent links need no such cleanup: applyParent adopts the parent's board, so a
// parent and its subtree always share a project and travel together.
func TestTransferProjectSeversCrossWorkspaceRefs(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// A second project in the source workspace: it stays behind when the first
	// one leaves, and that is what puts the two ends of each link apart.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект")
	stayBoard := mkBoard(t, owner, stayProject, "Доска остающегося")
	stayCols := owner.get("/boards/" + stayBoard + "/columns").listBody(t)
	stayCol := stayCols[0]["id"].(string)

	moving := mkTask(t, owner, src.Board, src.col(t, 0), "переезжающая задача")
	staying := mkTask(t, owner, stayBoard, stayCol, "остающаяся задача")
	// A second task in the moving project: everything it shares with `moving`
	// crosses no boundary and must come out of the transfer untouched.
	travelling := mkTask(t, owner, src.Board, src.col(t, 0), "её попутчица")

	// Both directions of each link, so a one-sided cleanup cannot pass.
	movingMilestone := owner.expect(t, owner.post("/projects/"+src.Project+"/milestones",
		map[string]any{"title": "Этап переезжающего"}), http.StatusCreated)
	stayMilestone := owner.expect(t, owner.post("/projects/"+stayProject+"/milestones",
		map[string]any{"title": "Этап остающегося"}), http.StatusCreated)
	owner.expect(t, owner.post("/tasks/"+staying["id"].(string)+"/milestone",
		map[string]any{"milestone_id": movingMilestone["id"]}), http.StatusNoContent)
	owner.expect(t, owner.post("/tasks/"+moving["id"].(string)+"/milestone",
		map[string]any{"milestone_id": stayMilestone["id"]}), http.StatusNoContent)

	owner.expect(t, owner.post("/tasks/"+staying["id"].(string)+"/relations",
		map[string]any{"number": moving["number"], "kind": "blocks"}), http.StatusCreated)
	owner.expect(t, owner.post("/tasks/"+moving["id"].(string)+"/relations",
		map[string]any{"number": staying["number"], "kind": "relates"}), http.StatusCreated)

	// The control pair, both ends inside the moving project.
	owner.expect(t, owner.post("/tasks/"+travelling["id"].(string)+"/milestone",
		map[string]any{"milestone_id": movingMilestone["id"]}), http.StatusNoContent)
	owner.expect(t, owner.post("/tasks/"+travelling["id"].(string)+"/relations",
		map[string]any{"number": moving["number"], "kind": "relates"}), http.StatusCreated)

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("the transfer reports what it severed", func(t *testing.T) {
		if n, _ := res["unlinked_relations"].(float64); n != 2 {
			t.Fatalf("unlinked_relations = %v, want 2: %v", res["unlinked_relations"], res)
		}
		if n, _ := res["unpinned_milestones"].(float64); n != 2 {
			t.Fatalf("unpinned_milestones = %v, want 2: %v", res["unpinned_milestones"], res)
		}
	})

	t.Run("no relation survives the boundary", func(t *testing.T) {
		for _, id := range []string{staying["id"].(string), moving["id"].(string)} {
			rels := owner.get("/tasks/" + id + "/relations").listBody(t)
			if len(rels) != 0 {
				t.Fatalf("task %s still relates across workspaces: %v", id, rels)
			}
		}
	})

	t.Run("no milestone survives the boundary", func(t *testing.T) {
		for _, id := range []string{staying["id"].(string), moving["id"].(string)} {
			m := owner.expect(t, owner.get("/tasks/"+id), http.StatusOK)
			if m["milestone_id"] != nil {
				t.Fatalf("task %s still points at milestone %v in another workspace", id, m["milestone_id"])
			}
		}
	})

	t.Run("links inside the moving project are untouched", func(t *testing.T) {
		rels := owner.get("/tasks/" + travelling["id"].(string) + "/relations").listBody(t)
		if len(rels) != 1 || rels[0]["related_task_id"] != moving["id"] {
			t.Fatalf("the relation between two tasks that travelled together is gone: %v", rels)
		}
		m := owner.expect(t, owner.get("/tasks/"+travelling["id"].(string)), http.StatusOK)
		if m["milestone_id"] != movingMilestone["id"] {
			t.Fatalf("travelling task lost its own project's milestone: %v", m["milestone_id"])
		}
	})

	t.Run("rollups count only their own workspace", func(t *testing.T) {
		want := map[string]float64{
			movingMilestone["id"].(string): 1, // only the task that travelled with it
			stayMilestone["id"].(string):   0,
		}
		for _, ws := range []string{src.WS, dst.WS} {
			for _, m := range owner.get("/workspaces/" + ws + "/milestones").listBody(t) {
				id, _ := m["id"].(string)
				n, _ := m["task_count"].(float64)
				if n != want[id] {
					t.Fatalf("milestone %v in workspace %s counts %v tasks, want %v",
						m["title"], ws, n, want[id])
				}
			}
		}
	})
}
