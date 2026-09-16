package main

import (
	"context"
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

// idsIn reads the `id` of every item a list endpoint returns, in the order the
// server returned them.
func idsIn(t *testing.T, c *client, path string) []string {
	t.Helper()
	var out []string
	for _, item := range c.get(path).listBody(t) {
		out = append(out, item["id"].(string))
	}
	return out
}

// assertSameOrder pins that two id lists are the same sequence.
func assertSameOrder(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d items, want %d (%v vs %v)", what, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: order differs at %d: got %v, want %v", what, i, got, want)
		}
	}
}

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

// TestListColumnsKeepsTiedOrderAcrossEdits pins the other half of the invariant:
// the guards above stop the API from *building* a tie, but a tie can still exist,
// and the board must not reshuffle around it.
//
// Ties arrive without anyone asking for them. CreateColumn reads
// MaxColumnPosition and then inserts — two separate statements, no transaction
// spanning them — so two people adding a column to one board at the same moment
// both land on max + gap. Two reorders into the same gap compute the same
// midpoint just as readily, and legacy rows predate the midpoint scheme entirely.
//
// With a tie in place `ORDER BY position` alone hands the pair's order to the
// planner, which sorts whatever the storage layer hands it — and that is not
// created_at order: the transaction that STARTED first stamps the earlier
// created_at, but the one that COMMITS first lands earlier physically. So the
// two disagree exactly in the race that makes the tie. RebalanceColumnPositions,
// the respace that eventually breaks the tie, numbers by (position, created_at,
// id) and promises to keep "the order they already had" — so whichever order the
// planner had been showing the readers, the respace overrode it and the pair
// traded places, permanently, for everyone.
func TestListColumnsKeepsTiedOrderAcrossEdits(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	path := "/boards/" + s.Board + "/columns"
	seeded := idsIn(t, c, path)
	if len(seeded) != 4 {
		t.Fatalf("seeded board has %d columns, want 4", len(seeded))
	}

	// The state the API refuses to build and a racing pair of creates does: the
	// third column shares the second's position, and — having opened its
	// transaction first — carries the earlier created_at despite committing last.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE board_columns SET position = tied.position,
		        created_at = tied.created_at - interval '1 second'
		   FROM (SELECT position, created_at FROM board_columns WHERE id = $1) tied
		  WHERE board_columns.id = $2`,
		seeded[1], seeded[2]); err != nil {
		t.Fatalf("plant the position tie: %v", err)
	}
	// created_at settles it: the tied pair comes back oldest-first, whatever the
	// planner would have preferred.
	want := []string{seeded[0], seeded[2], seeded[1], seeded[3]}
	assertSameOrder(t, "columns with a tie", idsIn(t, c, path), want)

	// An ordinary edit of a tied column — a colour change, nothing to do with
	// order — must not move it past its twin.
	c.expect(t, c.patch("/columns/"+seeded[1], map[string]any{
		"name": s.Columns[1]["name"], "color": "#e88080",
	}), http.StatusOK)
	assertSameOrder(t, "columns after recolouring a tied column", idsIn(t, c, path), want)

	// Dropping a column into the dead slot is what respaces the ladder. The order
	// it freezes must be the order the readers were shown.
	c.expect(t, c.patch("/columns/"+seeded[3]+"/move", map[string]any{
		"before_id": seeded[2], "after_id": seeded[1],
	}), http.StatusOK)
	assertSameOrder(t, "columns after the respace", idsIn(t, c, path),
		[]string{seeded[0], seeded[2], seeded[3], seeded[1]})
	assertStrictlyAscending(t, "columns", positionsIn(t, c, path))
}
