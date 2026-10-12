// Dropping a link is journalled on both tasks (#2932).
//
// Linking two tasks writes an entry into each one's history on purpose — the
// relation is a single directed row, so without the mirrored entry the
// referenced task's journal would never mention the link at all (applyRelation).
// Unlinking had no such symmetry: "/unlink #N" recorded only the acting side,
// and DELETE /relations/:id — the ✕ in the «Связи» tab, which is how either end
// drops a link on web and on the phone — recorded nothing, on either task. A
// link added and then removed therefore stayed in both histories as an arrival
// with no departure, and the reader had no way to tell a live relation from one
// that was taken away.
//
// `relation_removed` is not a new event kind: both clients have rendered it
// since "/unlink" shipped (`utils/taskFeed.js`, `util/TaskJournal.kt`) — it was
// simply unreachable from the button. This is the journal half of the omission
// task_relation_events_test.go pins for the socket.
package main

import (
	"fmt"
	"net/http"
	"testing"
)

// journalRefs collects the related-task numbers a task's journal records under
// one event kind — what the history actually renders ("удалил(а) связь с #N").
func journalRefs(t *testing.T, u *client, taskID, kind string) []float64 {
	t.Helper()
	out := []float64{}
	for _, e := range u.get("/tasks/" + taskID + "/events").listBody(t) {
		if e["kind"] != kind {
			continue
		}
		data, _ := e["data"].(map[string]any)
		ref, _ := data["related"].(float64)
		out = append(out, ref)
	}
	return out
}

// The ✕ on a relation row. Both tasks announced the link; both must announce
// that it is gone.
func TestDeleteRelationJournalsBothTasks(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	blocker := mkTask(t, owner, st.Board, colID, "Блокер")
	blocked := mkTask(t, owner, st.Board, colID, "Заблокированная")
	blockerID, blockedID := blocker["id"].(string), blocked["id"].(string)
	blockerNum, _ := blocker["number"].(float64)
	blockedNum, _ := blocked["number"].(float64)

	owner.expect(t, owner.post("/tasks/"+blockerID+"/relations",
		map[string]any{"number": blocked["number"], "kind": "blocks"}), http.StatusCreated)

	rows := owner.get("/tasks/" + blockerID + "/relations").listBody(t)
	if len(rows) != 1 {
		t.Fatalf("linking task lists %d relations, want 1", len(rows))
	}
	relID := rows[0]["id"].(string)
	if r := owner.del("/relations/" + relID); r.Status != http.StatusNoContent {
		t.Fatalf("delete relation: status %d, want 204\n%s", r.Status, r.Body)
	}

	for _, side := range []struct {
		name   string
		id     string
		wantTo float64
	}{
		{"the task the link was made from", blockerID, blockedNum},
		{"the referenced task", blockedID, blockerNum},
	} {
		refs := journalRefs(t, owner, side.id, "relation_removed")
		if len(refs) != 1 {
			t.Fatalf("%s has %d relation_removed entries, want 1 — its history still shows the link being added and nothing after it",
				side.name, len(refs))
		}
		if refs[0] != side.wantTo {
			t.Fatalf("%s names #%v in the unlink entry, want #%v", side.name, refs[0], side.wantTo)
		}
	}
}

// The quick action drops every kind between the pair at once; the referenced
// task's history has to say so too, exactly as it said the links were added.
func TestQuickUnlinkJournalsBothTasks(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	a := mkTask(t, owner, st.Board, colID, "A")
	b := mkTask(t, owner, st.Board, colID, "B")
	aID, bID := a["id"].(string), b["id"].(string)
	aNum, _ := a["number"].(float64)
	bNum, _ := b["number"].(float64)

	owner.expect(t, owner.post("/tasks/"+aID+"/relations",
		map[string]any{"number": b["number"], "kind": "blocks"}), http.StatusCreated)

	// Quick actions ride in a comment body; a pure-command comment applies and
	// leaves nothing behind (quickact_flow_test.go).
	r := owner.post("/tasks/"+aID+"/comments", map[string]any{"body": fmt.Sprintf("/unlink #%d", int64(bNum))})
	if r.Status != http.StatusOK {
		t.Fatalf("/unlink: status %d, want 200\n%s", r.Status, r.Body)
	}
	if applied, failed := commandSummary(t, r); len(applied) != 1 || applied[0] != "unlink" || len(failed) != 0 {
		t.Fatalf("/unlink summary applied=%v failed=%v, want [unlink] / []", applied, failed)
	}

	if refs := journalRefs(t, owner, aID, "relation_removed"); len(refs) != 1 || refs[0] != bNum {
		t.Fatalf("acting task records %v for the unlink, want [#%v]", refs, bNum)
	}
	if refs := journalRefs(t, owner, bID, "relation_removed"); len(refs) != 1 || refs[0] != aNum {
		t.Fatalf("the other end records %v for the unlink, want [#%v] — its journal announced the link and never took it back", refs, aNum)
	}
}
