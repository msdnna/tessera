package main

import (
	"net/http"
	"testing"
)

// Every orderable list in Tessera is numbered on its own float8 ladder, and for
// tasks there are TWO of them: a card is ordered among the top-level cards of
// its column, a subtask among its parent's children. Both ladders start at the
// same positionGap and step by it.
//
// So a task that changes parent changes the list it belongs to — and the number
// it carries has no meaning in the one it joins. `SetTaskParent` used not to
// touch `position` at all, which put the first subtask of a parent (65536) on
// the exact position of its column's first card once detached — the dead slot
// `positionExhausted` describes — and a later sibling somewhere in the middle of
// the column, wherever its index among its old siblings happened to point. The
// mirror image too: a card dropped into a card's subtask list arrived among the
// children at whatever rung the column had given it.
//
// The board's own drag never showed it, because that one path sends the move
// right behind the setParent (`planColDrop` emits both, in that order). The
// other three doors — the modal's «Открепить от родителя», its parent picker,
// and a card dropped onto a card's subtask list — send setParent alone, so the
// invariant has to hold in the server. It is the same answer the rest of the
// code gives when a task joins a list: the end of it (#2932).

func TestSetParentPutsTaskAtTheEndOfItsNewList(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)

	// Column ladder, in creation order: parent 65536, «Первая» 131072,
	// «Вторая» 196608.
	parent := mkTask(t, c, s.Board, todo, "Родитель")["id"].(string)
	first := mkTask(t, c, s.Board, todo, "Первая")["id"].(string)
	second := mkTask(t, c, s.Board, todo, "Вторая")["id"].(string)

	// Adopted in the reverse of their column order, so "the end of the subtask
	// list" and "the number the column gave it" disagree.
	c.expect(t, c.patch("/tasks/"+second+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+first+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)

	subs := idsIn(t, c, "/boards/"+s.Board+"/subtasks")
	assertSameOrder(t, "subtasks after two attaches", filterIDs(subs, first, second),
		[]string{second, first})
}

func TestDetachedSubtaskLandsAtTheEndOfTheColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)

	parent := mkTask(t, c, s.Board, todo, "Родитель")["id"].(string)
	card := mkTask(t, c, s.Board, todo, "Карточка")["id"].(string)
	child := mkTask(t, c, s.Board, todo, "Ребёнок")["id"].(string)

	// As a subtask the child sits on the parent's ladder — at its first rung,
	// which is also where the column's first card stands.
	c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)
	// One more card, filling the rung the child vacated on the column ladder.
	last := mkTask(t, c, s.Board, todo, "Последняя")["id"].(string)

	c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": nil}), http.StatusOK)

	ids := idsIn(t, c, "/boards/"+s.Board+"/tasks")
	assertSameOrder(t, "cards after a detach", filterIDs(ids, parent, card, last, child),
		[]string{parent, card, last, child})
}

func TestArchiveWithDetachAppendsChildrenToTheColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)

	parent := mkTask(t, c, s.Board, todo, "Родитель")["id"].(string)
	card := mkTask(t, c, s.Board, todo, "Карточка")["id"].(string)
	// Born as subtasks (the «/subtask» quick action is the door that reaches
	// applyCreateSubtask), so they stand on the parent's ladder from the start —
	// its first two rungs, which are also the column's first two.
	for _, title := range []string{"/subtask Первый ребёнок", "/subtask Второй ребёнок"} {
		c.expect(t, c.post("/tasks/"+parent+"/comments", map[string]any{"body": title}), http.StatusOK)
	}
	children := idsIn(t, c, "/boards/"+s.Board+"/subtasks")
	if len(children) != 2 {
		t.Fatalf("subtasks after two /subtask commands: got %d, want 2", len(children))
	}
	firstChild, secondChild := children[0], children[1]

	// Archiving the parent while keeping the brood on the board hands two tasks
	// at once from one ladder to another; they keep their order and go after the
	// cards that were already standing there.
	c.expect(t, c.patch("/tasks/"+parent+"/archive?subtasks=detach", nil), http.StatusNoContent)

	ids := idsIn(t, c, "/boards/"+s.Board+"/tasks")
	assertSameOrder(t, "cards after archiving the parent with детач", filterIDs(ids, card, firstChild, secondChild),
		[]string{card, firstChild, secondChild})
}
