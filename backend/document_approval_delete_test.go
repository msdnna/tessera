package main

import (
	"net/http"
	"testing"
)

// TestDeletingADocumentThatWasEverSentForApproval holds the document delete to
// the one thing it must always be able to do: remove the row.
//
// Raising an approval route pins the text that was agreed as a MANUAL version
// and points document_approvals.version_id at it with ON DELETE RESTRICT — the
// backstop that stops retention from pruning away a signed revision (0060).
// Deleting the document itself is a different matter: the delete is a single
// statement against `documents` and leaves the rest to the cascades, and both
// document_versions and document_approvals hang off documents with ON DELETE
// CASCADE. Postgres fires those two cascades in constraint order, and
// document_versions' FK is the older one (0057 against 0060) — so the snapshots
// go first, while the approval rows that still reference them are alive, and the
// immediate RESTRICT check aborts the whole delete.
//
// The user-visible shape: any document that was ever sent for approval becomes
// undeletable — the ✕ answers 500, forever, with nothing on screen saying why.
// The same statement runs under a project and a workspace delete, so one such
// document blocks those too.
func TestDeletingADocumentThatWasEverSentForApproval(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, docID := mkLinkedDoc(t, owner)
	member := approvers(t, owner, s.WS, 1)[0]

	owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{member.UserID}}), http.StatusCreated)
	if got := docApprovals(t, owner, docID); len(got) != 1 {
		t.Fatalf("fixture: the route was not raised: %#v", got)
	}

	if r := owner.del("/documents/" + docID); r.Status != http.StatusNoContent {
		t.Fatalf("deleting a document that carries an approval answered %d: %s", r.Status, r.Body)
	}
	if r := owner.get("/documents/" + docID); r.Status != http.StatusNotFound {
		t.Fatalf("the document survived its own delete: %d %s", r.Status, r.Body)
	}
}

// TestDeletingADocumentTakesItsApprovalHistory is the same delete read from the
// other end: an approval that was signed and closed is still a row pointing at a
// pinned snapshot, so a *finished* protocol has to come away with the document
// too — not just an open one.
func TestDeletingADocumentTakesItsApprovalHistory(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, docID := mkLinkedDoc(t, owner)
	member := approvers(t, owner, s.WS, 1)[0]

	a := owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{member.UserID}}), http.StatusCreated)
	approvalID := a["id"].(string)
	member.expect(t, member.post("/document-approvals/"+approvalID+"/decide",
		map[string]any{"decision": "approved", "signature": "Пётр Иванов"}), http.StatusOK)

	if r := owner.del("/documents/" + docID); r.Status != http.StatusNoContent {
		t.Fatalf("deleting a document with a signed protocol answered %d: %s", r.Status, r.Body)
	}
}
