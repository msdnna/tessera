// Linking a pair that is already linked changes nothing — and must say nothing (#2932).
//
// AddTaskRelation inserts `ON CONFLICT (task_id, related_task_id, kind) DO
// NOTHING`, so pressing «Связать» twice with the same kind, or repeating
// "/blocks #N", leaves the table exactly as it was. applyRelation journalled and
// broadcast regardless: both tasks' histories collected another "добавил(а)
// связь с #N" for a link that was already there, and every open Gantt re-read
// its edge list for an unchanged graph.
//
// The same reading holds when the pair is already on record from the OTHER end.
// A relation is one directed row, but both tasks show it — ListTaskRelations
// reads either direction and collapses the pair with DISTINCT ON, and the ✕
// takes both halves (DeleteRelationBothWays). So #10 "blocks" #20 and #20
// "blocked_by" #10 are the same statement: whoever adds the second one sees the
// «Связи» tab unchanged, while both journals grow an entry for it.
//
// applyUnlink already draws this line — it journals only when
// DeleteRelationsBetween actually removed rows. This is the same rule read from
// the other side, and it is the rule the journal is written under everywhere
// else: an entry means something happened.
package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// relPair is a linked pair of tasks with the ids and numbers both ends need.
type relPair struct {
	aID, bID   string
	aNum, bNum float64
	aRaw, bRaw any // the JSON numbers, as the relations endpoint wants them
}

func mkRelPair(t *testing.T, u *client, board, col string) relPair {
	t.Helper()
	a := mkTask(t, u, board, col, "A")
	b := mkTask(t, u, board, col, "Б")
	aNum, _ := a["number"].(float64)
	bNum, _ := b["number"].(float64)
	return relPair{
		aID: a["id"].(string), bID: b["id"].(string),
		aNum: aNum, bNum: bNum,
		aRaw: a["number"], bRaw: b["number"],
	}
}

// link posts a relation and insists it was accepted. Re-linking is idempotent,
// not an error: the caller asked for a state, and that state holds.
func link(t *testing.T, u *client, fromID string, number any, kind string) {
	t.Helper()
	u.expect(t, u.post("/tasks/"+fromID+"/relations",
		map[string]any{"number": number, "kind": kind}), http.StatusCreated)
}

// relationRefs is journalRefs for the link entry — the "добавил(а) связь с #N"
// lines a task's history renders.
func relationRefs(t *testing.T, u *client, taskID string) []float64 {
	t.Helper()
	return journalRefs(t, u, taskID, "relation")
}

// Pressing «Связать» twice with the same kind: the table cannot take the second
// row, so neither history may claim it did.
func TestRelinkSameKindJournalsOnce(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	link(t, owner, p.aID, p.bRaw, "blocks")
	link(t, owner, p.aID, p.bRaw, "blocks")

	if refs := relationRefs(t, owner, p.aID); len(refs) != 1 {
		t.Fatalf("linking task has %d relation entries after re-linking the same pair, want 1 (%v) — its history claims a link that was already there", len(refs), refs)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 1 {
		t.Fatalf("referenced task has %d relation entries after re-linking the same pair, want 1 (%v)", len(refs), refs)
	}
	if rows := owner.get("/tasks/" + p.aID + "/relations").listBody(t); len(rows) != 1 {
		t.Fatalf("relations list has %d rows after re-linking, want 1 — nothing was added", len(rows))
	}
}

// The quick action goes through the same applyRelation, and a command repeated
// in a busy thread is the ordinary way this happens.
func TestQuickRelateTwiceJournalsOnce(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	for i := 0; i < 3; i++ {
		body := fmt.Sprintf("/relate #%d", int64(p.bNum))
		r := owner.post("/tasks/"+p.aID+"/comments", map[string]any{"body": body})
		if r.Status != http.StatusOK {
			t.Fatalf("/relate run %d: status %d, want 200\n%s", i+1, r.Status, r.Body)
		}
		if applied, failed := commandSummary(t, r); len(applied) != 1 || len(failed) != 0 {
			t.Fatalf("/relate run %d summary applied=%v failed=%v, want one applied", i+1, applied, failed)
		}
	}

	if refs := relationRefs(t, owner, p.aID); len(refs) != 1 {
		t.Fatalf("acting task has %d relation entries after three /relate, want 1 (%v)", len(refs), refs)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 1 {
		t.Fatalf("referenced task has %d relation entries after three /relate, want 1 (%v)", len(refs), refs)
	}
}

// Same statement, other end: #10 blocks #20 and #20 blocked_by #10. Both tabs
// already showed the link, so nothing changed for anyone looking at it.
func TestRelinkFromOtherEndJournalsOnce(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	link(t, owner, p.aID, p.bRaw, "blocks")
	link(t, owner, p.bID, p.aRaw, "blocked_by")

	if refs := relationRefs(t, owner, p.aID); len(refs) != 1 {
		t.Fatalf("blocker has %d relation entries after the same link was added from the other end, want 1 (%v) — the «Связи» tab showed it all along", len(refs), refs)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 1 {
		t.Fatalf("blocked task has %d relation entries after the same link was added from its own end, want 1 (%v)", len(refs), refs)
	}
	for _, side := range []struct{ name, id string }{{"blocker", p.aID}, {"blocked task", p.bID}} {
		if rows := owner.get("/tasks/" + side.id + "/relations").listBody(t); len(rows) != 1 {
			t.Fatalf("%s lists %d relations, want 1 — the pair is one link seen from two ends", side.name, len(rows))
		}
	}
}

// The symmetric kinds map to themselves, so «связана с» added from either end is
// the same row twice over.
func TestRelinkSymmetricKindFromOtherEndJournalsOnce(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	link(t, owner, p.aID, p.bRaw, "relates")
	link(t, owner, p.bID, p.aRaw, "relates")

	if refs := relationRefs(t, owner, p.aID); len(refs) != 1 {
		t.Fatalf("first task has %d relation entries after «relates» from both ends, want 1 (%v)", len(refs), refs)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 1 {
		t.Fatalf("second task has %d relation entries after «relates» from both ends, want 1 (%v)", len(refs), refs)
	}
}

// ── controls: a link that IS new still announces itself ──

// The first link writes one entry on each side, naming the other task.
func TestFirstLinkJournalsBothSides(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	link(t, owner, p.aID, p.bRaw, "blocks")

	if refs := relationRefs(t, owner, p.aID); len(refs) != 1 || refs[0] != p.bNum {
		t.Fatalf("linking task records %v, want [#%v]", refs, p.bNum)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 1 || refs[0] != p.aNum {
		t.Fatalf("referenced task records %v, want [#%v] — relations are one row, so the other side only learns from this entry", refs, p.aNum)
	}
}

// A different kind between the same pair is its own row and its own statement —
// «блокирует» and «дублирует» live side by side, and the ✕ takes one at a time.
func TestSecondKindSamePairStillJournals(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	link(t, owner, p.aID, p.bRaw, "blocks")
	link(t, owner, p.aID, p.bRaw, "duplicates")

	if refs := relationRefs(t, owner, p.aID); len(refs) != 2 {
		t.Fatalf("linking task has %d relation entries for two different kinds, want 2 (%v)", len(refs), refs)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 2 {
		t.Fatalf("referenced task has %d relation entries for two different kinds, want 2 (%v)", len(refs), refs)
	}
	if rows := owner.get("/tasks/" + p.aID + "/relations").listBody(t); len(rows) != 2 {
		t.Fatalf("relations list has %d rows for two kinds, want 2", len(rows))
	}
}

// Unlink, then link again: the link genuinely returns, and the history has to
// show it arriving a second time — otherwise the pair reads as still severed.
func TestRelinkAfterUnlinkJournalsAgain(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	link(t, owner, p.aID, p.bRaw, "blocks")
	rows := owner.get("/tasks/" + p.aID + "/relations").listBody(t)
	if len(rows) != 1 {
		t.Fatalf("relations list has %d rows, want 1", len(rows))
	}
	if r := owner.del("/relations/" + rows[0]["id"].(string)); r.Status != http.StatusNoContent {
		t.Fatalf("delete relation: status %d, want 204\n%s", r.Status, r.Body)
	}
	link(t, owner, p.aID, p.bRaw, "blocks")

	if refs := relationRefs(t, owner, p.aID); len(refs) != 2 {
		t.Fatalf("linking task has %d relation entries after unlink+link, want 2 (%v) — the second link really did happen", len(refs), refs)
	}
	if refs := relationRefs(t, owner, p.bID); len(refs) != 2 {
		t.Fatalf("referenced task has %d relation entries after unlink+link, want 2 (%v)", len(refs), refs)
	}
}

// A no-op link must not wake an open Gantt either: the edge list it would
// re-read is byte-for-byte the one it holds.
//
// Waiting for silence would pass on a dead socket, so the no-op is followed by a
// link that IS new and the NEXT task.relation frame is read: it has to be the
// real one. A phantom event for the repeat arrives first and names "blocks".
func TestRelinkBroadcastsNothing(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)
	p := mkRelPair(t, owner, st.Board, colID)

	conn, status := dialWS(t, owner.token, false)
	if conn == nil {
		t.Fatalf("owner handshake failed (status %d)", status)
	}
	defer conn.Close()

	link(t, owner, p.aID, p.bRaw, "blocks")
	ev := awaitWSEvent(t, conn, "task.relation", 5*time.Second)
	if ev == nil {
		t.Fatal("the first link broadcast no task.relation event")
	}

	link(t, owner, p.aID, p.bRaw, "blocks")     // no-op: must put nothing on the wire
	link(t, owner, p.aID, p.bRaw, "duplicates") // a real second link

	ev = awaitWSEvent(t, conn, "task.relation", 5*time.Second)
	if ev == nil {
		t.Fatal("the second, real link broadcast no task.relation event")
	}
	data, _ := ev["data"].(map[string]any)
	if data["kind"] != "duplicates" {
		t.Fatalf("next task.relation after the repeat names kind %v, want duplicates — re-linking an existing pair woke every open chart for an unchanged graph", data["kind"])
	}
}
