package main

import (
	"net/http"
	"testing"
)

// Card order is a float8 midpoint (positionBetween): dropping a card between two
// neighbours stores the average of their positions. Two ways that arithmetic runs
// out, both of which end with cards sharing one position — and a shared position
// is a dead slot, because the midpoint of prev == next is prev again, so nothing
// can ever be inserted there afterwards.
//
//   - the slow way: ~52 drops into the same gap exhaust the float64 mantissa;
//   - the fast way: one request that names the same task as before_id and
//     after_id.
//
// Both are covered here. positionsDistinct also pins the list order itself: with
// ties present, ORDER BY position alone leaves the sequence up to the planner.
func positionsOf(t *testing.T, c *client, boardID string) []float64 {
	t.Helper()
	var out []float64
	for _, task := range c.get("/boards/" + boardID + "/tasks").listBody(t) {
		out = append(out, task["position"].(float64))
	}
	return out
}

// TestMovePositionSurvivesRepeatedMidpointInserts drops a card into the same gap
// far more times than float64 can halve it and insists every card keeps its own
// position, strictly ascending.
func TestMovePositionSurvivesRepeatedMidpointInserts(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	top := mkTask(t, c, s.Board, col, "верх")["id"].(string)
	bottom := mkTask(t, c, s.Board, col, "низ")["id"].(string)

	// Each pass drops a fresh card between the same top card and whatever card
	// currently sits right below it — the gap halves every time.
	for i := 0; i < 60; i++ {
		id := mkTask(t, c, s.Board, col, "вставка")["id"].(string)
		c.expect(t, c.patch("/tasks/"+id+"/move", map[string]any{
			"column_id": col, "before_id": top, "after_id": bottom,
		}), http.StatusOK)
		bottom = id
	}

	pos := positionsOf(t, c, s.Board)
	if len(pos) != 62 {
		t.Fatalf("board has %d tasks, want 62", len(pos))
	}
	for i := 1; i < len(pos); i++ {
		if pos[i] <= pos[i-1] {
			t.Fatalf("positions not strictly ascending at %d: %v ... %v (full: %v)",
				i, pos[i-1], pos[i], pos)
		}
	}
}

// TestMoveRejectsSameNeighbourTwice covers the one-request version: naming one
// task as both neighbours asks for a position equal to that task's own.
func TestMoveRejectsSameNeighbourTwice(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	anchor := mkTask(t, c, s.Board, col, "якорь")["id"].(string)
	mover := mkTask(t, c, s.Board, col, "гость")["id"].(string)

	r := c.patch("/tasks/"+mover+"/move", map[string]any{
		"column_id": col, "before_id": anchor, "after_id": anchor,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("move with before_id == after_id: got %d, want 400 (body %s)", r.Status, r.Body)
	}

	pos := positionsOf(t, c, s.Board)
	if len(pos) != 2 || pos[0] == pos[1] {
		t.Fatalf("positions collided after the refused move: %v", pos)
	}
}
