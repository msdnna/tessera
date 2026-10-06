// A relation is visible from both of its ends (#2932).
//
// task_relations stores one directed row, and the list endpoint used to read
// `WHERE task_id = $1` only: the task that was linked FROM showed the relation,
// the task that was linked TO showed an empty «Связи» tab — while its own
// journal carried the entry and the Gantt drew the arrow (ListBoardDependencies
// reads both endpoints and normalises). The GitLab projection already works the
// other way on purpose: it mirrors the reverse edge "so the relation shows on
// both tasks" (gitlab_relations.go), so an imported link was visible from both
// sides and a hand-made one from one.
//
// These pin the read side (both ends, inverted kind, no duplicate when both rows
// exist) and the delete side (one ✕ drops the pair, from either end).
package main

import (
	"net/http"
	"testing"
)

// relKinds maps a relation list response to related-number → kind, which is what
// the tab actually renders.
func relKinds(t *testing.T, rows []map[string]any) map[float64]string {
	t.Helper()
	out := map[float64]string{}
	for _, r := range rows {
		num, _ := r["related_number"].(float64)
		kind, _ := r["kind"].(string)
		out[num] = kind
	}
	return out
}

func TestRelationVisibleFromBothTasks(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	blocker := mkTask(t, owner, st.Board, colID, "Блокер")
	blocked := mkTask(t, owner, st.Board, colID, "Заблокированная")
	blockerID := blocker["id"].(string)
	blockedID := blocked["id"].(string)
	blockerNum, _ := blocker["number"].(float64)
	blockedNum, _ := blocked["number"].(float64)

	owner.expect(t, owner.post("/tasks/"+blockerID+"/relations",
		map[string]any{"number": blocked["number"], "kind": "blocks"}), http.StatusCreated)

	// The side that created it — unchanged behaviour.
	from := relKinds(t, owner.get("/tasks/"+blockerID+"/relations").listBody(t))
	if from[blockedNum] != "blocks" {
		t.Fatalf("linking task shows %v for #%v, want blocks", from, blockedNum)
	}

	// The referenced side — the regression this pins.
	to := owner.get("/tasks/" + blockedID + "/relations").listBody(t)
	if len(to) != 1 {
		t.Fatalf("referenced task lists %d relations, want 1 — «Связи» is empty on the task that was blocked", len(to))
	}
	if got := relKinds(t, to)[blockerNum]; got != "blocked_by" {
		t.Fatalf("referenced task shows kind %q for #%v, want blocked_by (the relation reads the other way round from there)", got, blockerNum)
	}
}

// A symmetric kind stays itself when read from the far end.
func TestSymmetricRelationKeepsKindOnBothTasks(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	a := mkTask(t, owner, st.Board, colID, "A")
	b := mkTask(t, owner, st.Board, colID, "B")
	aNum, _ := a["number"].(float64)

	owner.expect(t, owner.post("/tasks/"+a["id"].(string)+"/relations",
		map[string]any{"number": b["number"], "kind": "relates"}), http.StatusCreated)

	rows := owner.get("/tasks/" + b["id"].(string) + "/relations").listBody(t)
	if len(rows) != 1 || relKinds(t, rows)[aNum] != "relates" {
		t.Fatalf("far end shows %v, want one 'relates' row for #%v", rows, aNum)
	}
}

// Both directed rows on record (what the GitLab projection writes, and what two
// users linking the same pair from either end produce) must still read as ONE
// relation per end — not the same link listed twice.
func TestMirroredRelationIsNotListedTwice(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	a := mkTask(t, owner, st.Board, colID, "A")
	b := mkTask(t, owner, st.Board, colID, "B")
	aID, bID := a["id"].(string), b["id"].(string)

	owner.expect(t, owner.post("/tasks/"+aID+"/relations",
		map[string]any{"number": b["number"], "kind": "blocks"}), http.StatusCreated)
	owner.expect(t, owner.post("/tasks/"+bID+"/relations",
		map[string]any{"number": a["number"], "kind": "blocked_by"}), http.StatusCreated)

	for _, side := range []struct{ name, id string }{{"blocker", aID}, {"blocked", bID}} {
		rows := owner.get("/tasks/" + side.id + "/relations").listBody(t)
		if len(rows) != 1 {
			t.Fatalf("%s lists %d relations, want 1 — the mirrored row doubles the link in «Связи»", side.name, len(rows))
		}
	}
}

// The ✕ on a relation means "drop this link", so it must work from the end that
// did not create it — and take the mirrored row with it, or the row the user
// just deleted reappears on the next read.
func TestDeleteRelationFromReferencedSideDropsThePair(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	a := mkTask(t, owner, st.Board, colID, "A")
	b := mkTask(t, owner, st.Board, colID, "B")
	aID, bID := a["id"].(string), b["id"].(string)

	owner.expect(t, owner.post("/tasks/"+aID+"/relations",
		map[string]any{"number": b["number"], "kind": "blocks"}), http.StatusCreated)
	owner.expect(t, owner.post("/tasks/"+bID+"/relations",
		map[string]any{"number": a["number"], "kind": "blocked_by"}), http.StatusCreated)

	rows := owner.get("/tasks/" + bID + "/relations").listBody(t)
	if len(rows) != 1 {
		t.Fatalf("referenced task lists %d relations, want 1", len(rows))
	}
	relID := rows[0]["id"].(string)
	if r := owner.del("/relations/" + relID); r.Status != http.StatusNoContent {
		t.Fatalf("delete from the referenced side: status %d, want 204\n%s", r.Status, r.Body)
	}

	for _, side := range []struct{ name, id string }{{"A", aID}, {"B", bID}} {
		if rows := owner.get("/tasks/" + side.id + "/relations").listBody(t); len(rows) != 0 {
			t.Fatalf("%s still lists %d relations after the link was deleted — the mirrored row survived", side.name, len(rows))
		}
	}
}
