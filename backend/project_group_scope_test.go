package main

import (
	"net/http"
	"testing"
)

// TestProjectGroupMustBeInSameWorkspace pins the group_id a caller sends in the
// body of a project create/move. The route's :id is checked (requireMember /
// loadProject), but group_id is a second reference to a row in a tree that
// belongs to one workspace by definition, and nothing tied the two together:
// the foreign key accepts any existing group, including one from a workspace
// the project has nothing to do with.
//
// That is the same hole already closed for a group's own parent_id
// (groupParentAllowed, #2932), and TransferProject spells the invariant out by
// clearing group_id on the way out — "groups belong to the source workspace".
// A project left pointing at a foreign group vanishes: the sidebar lists the
// groups of one workspace and files each project under the matching node
// (projectsInGroup), so a group_id that matches no node and is not null puts
// the project — with all its boards and tasks — in no bucket at all. It is
// broadcast to the workspace, survives a reload, and the sidebar is the only UI
// that could drag it back out.
//
// The attacker needs nothing but a second workspace of their own, which every
// account can create.
func TestProjectGroupMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	attacker := signup(t)
	otherWS := mkWorkspace(t, attacker, "WS other "+t.Name())
	foreignGroup := mkGroup(t, attacker, otherWS, "группа другого пространства")

	shared := signup(t)
	sharedWS := mkWorkspace(t, shared, "WS shared "+t.Name())
	homeGroup := mkGroup(t, shared, sharedWS, "своя группа")
	project := mkProject(t, shared, sharedWS, homeGroup, "общий проект")

	// The attacker is a plain member of the shared workspace.
	shared.expect(t, shared.post("/workspaces/"+sharedWS+"/members",
		map[string]any{"email": attacker.Email, "role": "member"}), http.StatusCreated)

	t.Run("move into a foreign group is refused", func(t *testing.T) {
		r := attacker.patch("/projects/"+project+"/move", map[string]any{"group_id": foreignGroup})
		if r.Status != http.StatusForbidden {
			t.Fatalf("move into foreign group: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("create in a foreign group is refused", func(t *testing.T) {
		r := attacker.post("/workspaces/"+sharedWS+"/projects",
			map[string]any{"name": "подсадка", "group_id": foreignGroup})
		if r.Status != http.StatusForbidden {
			t.Fatalf("create in foreign group: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("the project stays where the sidebar can find it", func(t *testing.T) {
		got := shared.get("/workspaces/" + sharedWS + "/projects").listBody(t)
		for _, p := range got {
			if p["id"] == project && p["group_id"] != homeGroup {
				t.Fatalf("project group_id = %v, want %s", p["group_id"], homeGroup)
			}
		}
	})

	t.Run("moves inside the workspace still work", func(t *testing.T) {
		sibling := mkGroup(t, shared, sharedWS, "соседняя группа")
		got := shared.expect(t, shared.patch("/projects/"+project+"/move",
			map[string]any{"group_id": sibling}), http.StatusOK)
		if got["group_id"] != sibling {
			t.Fatalf("project group_id = %v, want %s", got["group_id"], sibling)
		}
		// Ungrouping (group_id: null) is a legitimate move to the root.
		got = shared.expect(t, shared.patch("/projects/"+project+"/move",
			map[string]any{"group_id": nil}), http.StatusOK)
		if got["group_id"] != nil {
			t.Fatalf("project group_id = %v, want nil (root)", got["group_id"])
		}
	})
}
