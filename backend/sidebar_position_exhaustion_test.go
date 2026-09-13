package main

import (
	"net/http"
	"testing"
)

// Columns, projects and project groups order themselves the same way cards do —
// a float8 midpoint between the two neighbours a drop names. Task cards already
// survive both ways that arithmetic runs out (see task_position_exhaustion_test.go);
// these three lists share the scheme, so they have to survive them too:
//
//   - the fast way: one request naming the same item as before_id and after_id
//     asks for a zero-width slot, and the midpoint lands *on* that item;
//   - the slow way: ~52 drops into the same gap exhaust the float64 mantissa and
//     the midpoint collapses onto a neighbour by itself.
//
// Either way two rows end up sharing a position, and a shared position is worse
// than a cosmetic rounding loss: `ORDER BY position` leaves their order to the
// planner (so the sidebar can reshuffle on reload), and the slot is dead — the
// midpoint of prev == next is prev again, so nothing can ever be dropped between
// them afterwards.

// positionsIn reads the `position` of every item a list endpoint returns, in the
// order the server returned them.
func positionsIn(t *testing.T, c *client, path string) []float64 {
	t.Helper()
	var out []float64
	for _, item := range c.get(path).listBody(t) {
		out = append(out, item["position"].(float64))
	}
	return out
}

// assertStrictlyAscending pins both halves of the invariant: no two items share a
// position, and the order the endpoint returned is the order the positions imply.
func assertStrictlyAscending(t *testing.T, what string, pos []float64) {
	t.Helper()
	for i := 1; i < len(pos); i++ {
		if pos[i] <= pos[i-1] {
			t.Fatalf("%s: positions not strictly ascending at %d: %v ... %v (full: %v)",
				what, i, pos[i-1], pos[i], pos)
		}
	}
}

func TestMoveColumnRejectsSameNeighbourTwice(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	anchor := s.col(t, 0)
	mover := s.col(t, 2)

	r := c.patch("/columns/"+mover+"/move", map[string]any{
		"before_id": anchor, "after_id": anchor,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("column move with before_id == after_id: got %d, want 400 (body %s)", r.Status, r.Body)
	}
	assertStrictlyAscending(t, "columns", positionsIn(t, c, "/boards/"+s.Board+"/columns"))
}

func TestMoveProjectRejectsSameNeighbourTwice(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	anchor := s.Project
	mover := mkProject(t, c, s.WS, s.Group, "второй")

	r := c.patch("/projects/"+mover+"/move", map[string]any{
		"group_id": s.Group, "before_id": anchor, "after_id": anchor,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("project move with before_id == after_id: got %d, want 400 (body %s)", r.Status, r.Body)
	}
	assertStrictlyAscending(t, "projects", positionsIn(t, c, "/workspaces/"+s.WS+"/projects"))
}

func TestMoveProjectGroupRejectsSameNeighbourTwice(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	anchor := s.Group
	mover := mkGroup(t, c, s.WS, "вторая")

	r := c.patch("/groups/"+mover+"/move", map[string]any{
		"before_id": anchor, "after_id": anchor,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("group move with before_id == after_id: got %d, want 400 (body %s)", r.Status, r.Body)
	}
	assertStrictlyAscending(t, "groups", positionsIn(t, c, "/workspaces/"+s.WS+"/groups"))
}

// TestMoveColumnSurvivesRepeatedMidpointInserts drops a column into the same gap
// far more times than float64 can halve it. Every column must keep a position of
// its own, and the list must still come back in the order the drops built.
func TestMoveColumnSurvivesRepeatedMidpointInserts(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	top := s.col(t, 0)
	bottom := s.col(t, 1)
	for i := 0; i < 60; i++ {
		id := c.expect(t, c.post("/boards/"+s.Board+"/columns",
			map[string]any{"name": "вставка"}), http.StatusCreated)["id"].(string)
		c.expect(t, c.patch("/columns/"+id+"/move", map[string]any{
			"before_id": top, "after_id": bottom,
		}), http.StatusOK)
		bottom = id
	}

	pos := positionsIn(t, c, "/boards/"+s.Board+"/columns")
	if len(pos) != 64 {
		t.Fatalf("board has %d columns, want 64 (4 seeded + 60 inserted)", len(pos))
	}
	assertStrictlyAscending(t, "columns", pos)
}

// TestMoveProjectSurvivesRepeatedMidpointInserts is the same drill for the
// sidebar's project list, whose positions are workspace-wide.
func TestMoveProjectSurvivesRepeatedMidpointInserts(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	top := s.Project
	bottom := mkProject(t, c, s.WS, s.Group, "низ")
	for i := 0; i < 60; i++ {
		id := mkProject(t, c, s.WS, s.Group, "вставка")
		c.expect(t, c.patch("/projects/"+id+"/move", map[string]any{
			"group_id": s.Group, "before_id": top, "after_id": bottom,
		}), http.StatusOK)
		bottom = id
	}

	pos := positionsIn(t, c, "/workspaces/"+s.WS+"/projects")
	if len(pos) != 62 {
		t.Fatalf("workspace has %d projects, want 62", len(pos))
	}
	assertStrictlyAscending(t, "projects", pos)
}

// TestMoveProjectGroupSurvivesRepeatedMidpointInserts — same, for groups.
func TestMoveProjectGroupSurvivesRepeatedMidpointInserts(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	top := s.Group
	bottom := mkGroup(t, c, s.WS, "низ")
	for i := 0; i < 60; i++ {
		id := mkGroup(t, c, s.WS, "вставка")
		c.expect(t, c.patch("/groups/"+id+"/move", map[string]any{
			"before_id": top, "after_id": bottom,
		}), http.StatusOK)
		bottom = id
	}

	pos := positionsIn(t, c, "/workspaces/"+s.WS+"/groups")
	if len(pos) != 62 {
		t.Fatalf("workspace has %d groups, want 62", len(pos))
	}
	assertStrictlyAscending(t, "groups", pos)
}
