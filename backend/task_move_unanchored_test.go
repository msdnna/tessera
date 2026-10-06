package main

import (
	"net/http"
	"testing"
)

// A move that names no neighbours is not a rare shape: it is what every caller
// outside a drag sends, because outside a drag there is nothing to aim between.
// The web context menu's «Переместить → колонка» is the only mover the list,
// calendar, timeline and gantt layouts have, and the MCP client's MoveTask says
// in its own comment that the position is "left to the server (appended)".
//
// It was not appended. positionBetween(nil, nil) is the constant positionGap —
// and positionGap is exactly where a column's FIRST card sits, since CreateTask
// numbers a column 65536, 131072, 196608 … So the card landed on top of the
// column instead of at its end, sharing a position with the card already there:
// the tie the rest of the move path exists to prevent (rejectSameNeighbour,
// positionExhausted, the rebalance behind it), none of which can fire when there
// is no slot to find a collapse in.

// handlers.positionGap, spelled out because it is unexported: the gap a list's
// ladder is numbered with, and therefore the position of its first card.
const firstCardPosition = 65536.0

// A top-level card moved with no neighbours lands at the END of the destination
// column, where both the task modal and Android already put it by hand.
func TestMoveTaskWithoutNeighboursAppendsToColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, doing := s.col(t, 0), s.col(t, 1)

	first := mkTask(t, c, s.Board, doing, "Первая")["id"].(string)
	second := mkTask(t, c, s.Board, doing, "Вторая")["id"].(string)
	mover := mkTask(t, c, s.Board, todo, "Переносимая")["id"].(string)

	moved := c.expect(t, c.patch("/tasks/"+mover+"/move",
		map[string]any{"column_id": doing}), http.StatusOK)
	if moved["column_id"] != doing {
		t.Fatalf("move landed in column %v, want %s", moved["column_id"], doing)
	}
	// The tie is the defect's fingerprint: the old behaviour put the mover on
	// `first`'s own position.
	pos, ok := moved["position"].(float64)
	if !ok {
		t.Fatalf("moved task has no numeric position: %v", moved["position"])
	}
	if pos <= firstCardPosition {
		t.Fatalf("position after an unanchored move = %v, want > %v (the column's ladder starts there)", pos, firstCardPosition)
	}

	ids := filterIDs(idsIn(t, c, "/boards/"+s.Board+"/tasks"), first, second, mover)
	assertSameOrder(t, "destination column after an unanchored move", ids,
		[]string{first, second, mover})

	// And it keeps appending: a second card moved in the same way stacks behind
	// the first, rather than both piling onto positionGap.
	next := mkTask(t, c, s.Board, todo, "Следующая")["id"].(string)
	c.expect(t, c.patch("/tasks/"+next+"/move", map[string]any{"column_id": doing}), http.StatusOK)
	ids = filterIDs(idsIn(t, c, "/boards/"+s.Board+"/tasks"), first, second, mover, next)
	assertSameOrder(t, "two unanchored moves in a row", ids, []string{first, second, mover, next})
}

// An empty destination column still starts its ladder at positionGap — the same
// number CreateTask gives the first card, so the two paths agree.
func TestMoveTaskWithoutNeighboursIntoEmptyColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, empty := s.col(t, 0), s.col(t, 2)

	mover := mkTask(t, c, s.Board, todo, "Единственная")["id"].(string)
	moved := c.expect(t, c.patch("/tasks/"+mover+"/move",
		map[string]any{"column_id": empty}), http.StatusOK)
	if got := moved["position"]; got != firstCardPosition {
		t.Fatalf("position in an empty column = %v, want %v", got, firstCardPosition)
	}
}

// A subtask is numbered among its PARENT's children, and changing column does not
// take it out of that list — so an unanchored move leaves the parent's order
// alone instead of throwing the child to the top of it.
func TestMoveSubtaskWithoutNeighboursKeepsSiblingOrder(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, doing := s.col(t, 0), s.col(t, 1)

	parent := mkTask(t, c, s.Board, todo, "Родитель")["id"].(string)
	mkChild := func(title string) map[string]any {
		return c.expect(t, c.post("/boards/"+s.Board+"/tasks",
			map[string]any{"title": title, "column_id": todo, "parent_id": parent}), http.StatusCreated)
	}
	one := mkChild("Первая подзадача")["id"].(string)
	two := mkChild("Вторая подзадача")["id"].(string)
	three := mkChild("Третья подзадача")
	last, lastPos := three["id"].(string), three["position"].(float64)

	moved := c.expect(t, c.patch("/tasks/"+last+"/move",
		map[string]any{"column_id": doing}), http.StatusOK)
	if moved["column_id"] != doing {
		t.Fatalf("subtask landed in column %v, want %s", moved["column_id"], doing)
	}
	if got := moved["position"]; got != lastPos {
		t.Fatalf("subtask position after an unanchored move = %v, want %v (unchanged)", got, lastPos)
	}
	ids := filterIDs(idsIn(t, c, "/boards/"+s.Board+"/subtasks"), one, two, last)
	assertSameOrder(t, "parent's children after a subtask changed column", ids,
		[]string{one, two, last})
}
