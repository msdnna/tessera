package main

import (
	"net/http"
	"sync"
	"testing"
)

// approvalRaceRounds is how many times each race below is replayed. One round
// wins or loses on scheduling, so a single attempt would make the unfixed
// behaviour show up only sometimes — the worst kind of test. A handful of rounds
// turns "this can happen" into "this happens", while a correct implementation
// passes every round of every run.
const approvalRaceRounds = 8

// TestDocumentApprovalRaiseIsSerialised holds the "one open route per document"
// rule to what its own comment promises.
//
// The rule is a read (is one already open?) followed by an insert. Run outside a
// transaction those two are not one decision: two clicks landing together both
// read "none open" and both write, and the document ends up collecting
// signatures on two revisions at once — exactly the state the check exists to
// prevent. The neighbouring feature, conference recordings, refuses this with a
// unique index; a route has no such index, so the serialisation has to come from
// the lock this test pins.
func TestDocumentApprovalRaiseIsSerialised(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, _ := mkLinkedDoc(t, owner)
	member := approvers(t, owner, s.WS, 1)[0]

	for round := 0; round < approvalRaceRounds; round++ {
		doc := owner.expect(t, owner.post("/workspaces/"+s.WS+"/documents",
			map[string]any{"title": "Приказ"}), http.StatusCreated)
		docID := doc["id"].(string)

		var (
			wg   sync.WaitGroup
			gate = make(chan struct{})
			out  = make([]resp, 2)
		)
		for i := range out {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-gate
				out[i] = owner.post("/documents/"+docID+"/approvals",
					map[string]any{"approvers": []any{member.UserID}})
			}(i)
		}
		close(gate)
		wg.Wait()

		created := 0
		for _, r := range out {
			switch r.Status {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
			default:
				t.Fatalf("round %d: raising a route answered %d: %s", round, r.Status, r.Body)
			}
		}
		if created != 1 {
			t.Fatalf("round %d: %d of 2 simultaneous requests raised a route, want 1", round, created)
		}
		open := 0
		for _, a := range docApprovals(t, owner, docID) {
			if a["status"] == "pending" {
				open++
			}
		}
		if open != 1 {
			t.Fatalf("round %d: document left with %d open routes, want 1", round, open)
		}
	}
}

// TestDocumentApprovalParallelSignaturesClose pins the other half of the same
// gap: signing is a read-modify-write over the whole route, because the route's
// status is recomputed from its step list.
//
// In parallel mode the last two approvers can sign at the same instant. Each one
// reads the steps before the other's signature is written, so each concludes
// "somebody is still pending" and neither closes the route — every step is signed
// and the protocol says pending forever. Nothing recomputes it later, and the
// rule tested above then refuses to raise a replacement, so the document can
// never be согласован again without a manual cancellation.
func TestDocumentApprovalParallelSignaturesClose(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, _ := mkLinkedDoc(t, owner)
	ms := approvers(t, owner, s.WS, 2)

	for round := 0; round < approvalRaceRounds; round++ {
		doc := owner.expect(t, owner.post("/workspaces/"+s.WS+"/documents",
			map[string]any{"title": "Регламент"}), http.StatusCreated)
		docID := doc["id"].(string)
		approval := owner.expect(t, owner.post("/documents/"+docID+"/approvals", map[string]any{
			"mode": "parallel", "approvers": []any{ms[0].UserID, ms[1].UserID},
		}), http.StatusCreated)
		id := approval["id"].(string)

		var (
			wg   sync.WaitGroup
			gate = make(chan struct{})
			out  = make([]resp, len(ms))
		)
		for i, m := range ms {
			wg.Add(1)
			go func(i int, m *client) {
				defer wg.Done()
				<-gate
				out[i] = m.post("/document-approvals/"+id+"/decide",
					map[string]any{"decision": "approved"})
			}(i, m)
		}
		close(gate)
		wg.Wait()

		for i, r := range out {
			if r.Status != http.StatusOK {
				t.Fatalf("round %d: approver %d got %d: %s", round, i, r.Status, r.Body)
			}
		}
		got := approvalByID(t, owner, docID, id)
		if got["status"] != "approved" {
			t.Fatalf("round %d: every step signed but the route reads %#v", round, got["status"])
		}
		if got["closed_at"] == nil {
			t.Fatalf("round %d: a fully signed route was left open-ended", round)
		}
	}
}
