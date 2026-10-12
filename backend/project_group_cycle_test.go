package main

import (
	"net/http"
	"testing"
)

// TestProjectGroupCannotBecomeItsOwnDescendant pins the re-parent guard on the
// sidebar tree of groups.
//
// The handler used to reject only the literal self-parent (parent_id == the
// group being moved), which leaves the two-step version wide open: move A under
// its own child B and the table holds A.parent=B, B.parent=A — a cycle with no
// root. There is no constraint or trigger on project_groups to catch it, and the
// clients build the tree from parent_id by recursion: the sidebar node renders
// itself for every child and walks ancestors in a while loop, so a cycle is not
// a cosmetic oddity but an unbounded recursion that takes the whole sidebar
// down. The move is broadcast to the workspace and the cycle lives in the
// database, so it reaches every member and survives a reload — with the sidebar
// being the only way to re-parent a group, there is no UI left to undo it.
//
// Documents already hold this line (DocumentAncestorIDs exists for exactly
// this); groups are the same shape of tree and get the same guard.
func TestProjectGroupCannotBecomeItsOwnDescendant(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	top := mkGroup(t, c, ws, "верхняя")
	mid := mkGroup(t, c, ws, "средняя")
	deep := mkGroup(t, c, ws, "глубокая")

	// top → mid → deep
	c.expect(t, c.patch("/groups/"+mid+"/move", map[string]any{"parent_id": top}), http.StatusOK)
	c.expect(t, c.patch("/groups/"+deep+"/move", map[string]any{"parent_id": mid}), http.StatusOK)

	t.Run("into its own direct child is refused", func(t *testing.T) {
		r := c.patch("/groups/"+top+"/move", map[string]any{"parent_id": mid})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("move into own child: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("into a deeper descendant is refused", func(t *testing.T) {
		r := c.patch("/groups/"+top+"/move", map[string]any{"parent_id": deep})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("move into own grandchild: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("itself is refused", func(t *testing.T) {
		r := c.patch("/groups/"+top+"/move", map[string]any{"parent_id": top})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("move into itself: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("the tree keeps a single root", func(t *testing.T) {
		got := c.get("/workspaces/" + ws + "/groups").listBody(t)
		parents := map[string]any{}
		for _, g := range got {
			parents[g["id"].(string)] = g["parent_id"]
		}
		if parents[top] != nil {
			t.Fatalf("top group parent_id = %v, want nil (root)", parents[top])
		}
		if parents[mid] != top {
			t.Fatalf("mid group parent_id = %v, want %s", parents[mid], top)
		}
		if parents[deep] != mid {
			t.Fatalf("deep group parent_id = %v, want %s", parents[deep], mid)
		}
	})

	t.Run("a legitimate move still works", func(t *testing.T) {
		// deep out to the root, then mid under it — re-parenting upward and
		// downward both stay allowed; only the cycle is refused.
		c.expect(t, c.patch("/groups/"+deep+"/move", map[string]any{"parent_id": nil}), http.StatusOK)
		got := c.expect(t, c.patch("/groups/"+mid+"/move", map[string]any{"parent_id": deep}), http.StatusOK)
		if got["parent_id"] != deep {
			t.Fatalf("mid parent_id = %v, want %s", got["parent_id"], deep)
		}
	})
}

// TestProjectGroupParentMustBeInSameWorkspace pins the other half of the
// parent_id the caller sends in the body: the route's :id is checked by
// loadGroup, but the parent is a second reference, and nothing tied it to the
// same workspace. Hanging a group off a stranger's group drops it out of its own
// workspace tree (the client only lists groups of one workspace, so a parent it
// cannot see makes the branch unreachable) while leaving workspace_id — and with
// it every authorization check — pointing at the old workspace.
func TestProjectGroupParentMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	victimWS := mkWorkspace(t, victim, "WS victim "+t.Name())
	victimGroup := mkGroup(t, victim, victimWS, "чужая группа")

	attacker := signup(t)
	attackerWS := mkWorkspace(t, attacker, "WS attacker "+t.Name())
	mine := mkGroup(t, attacker, attackerWS, "своя группа")

	t.Run("move under a foreign group is refused", func(t *testing.T) {
		r := attacker.patch("/groups/"+mine+"/move", map[string]any{"parent_id": victimGroup})
		if r.Status != http.StatusForbidden {
			t.Fatalf("move under foreign group: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("create under a foreign group is refused", func(t *testing.T) {
		r := attacker.post("/workspaces/"+attackerWS+"/groups",
			map[string]any{"name": "подсадка", "parent_id": victimGroup})
		if r.Status != http.StatusForbidden {
			t.Fatalf("create under foreign group: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("the group stays at its own root", func(t *testing.T) {
		got := attacker.get("/workspaces/" + attackerWS + "/groups").listBody(t)
		for _, g := range got {
			if g["id"] == mine && g["parent_id"] != nil {
				t.Fatalf("own group parent_id = %v, want nil", g["parent_id"])
			}
		}
	})

	t.Run("the foreign workspace gains nothing", func(t *testing.T) {
		got := victim.get("/workspaces/" + victimWS + "/groups").listBody(t)
		if len(got) != 1 {
			t.Fatalf("victim workspace has %d groups, want 1", len(got))
		}
	})
}
