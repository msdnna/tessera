package main

import (
	"net/http"
	"testing"
)

// TestTransferProjectClosesUnsignableApprovals pins the seventh cross-workspace
// link of a project transfer, and the only one of the seven with nothing to
// sever: the open approval route on a document that travels (#2932).
//
// CreateDocumentApproval refuses an approver who is not a member of the
// document's workspace, and its comment says why — "someone who cannot open it
// can only ever stall it". A transfer moves the document into another workspace
// and leaves the route pointing at the people the *source* workspace asked, so
// the door's own rule is broken from behind. Nothing here is a leak to cut: both
// ends of the route travelled together. What breaks is that the route can never
// be finished. loadDocumentApproval authorizes on the document's workspace, so
// the stranded approver is answered 403; no endpoint adds or replaces a step in
// a route already raised; and the one-open-route rule then refuses the new
// owners a route of their own, so the document is stuck behind «на
// согласовании» until a manager of a workspace that never raised it notices.
//
// The three controls matter as much as the finding: a route whose remaining
// approvers are members of both sides is signable exactly as before, a route on
// a document of the project that stayed is none of this transfer's business, and
// a step already signed is a signature rather than a debt — a route stalled only
// on somebody who can still open the document must survive and be signable
// afterwards.
func TestTransferProjectClosesUnsignableApprovals(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// The whole test turns on these two. The stranger is a member of the source
	// workspace only — legal as an approver when the route is raised, unable to
	// open the document the moment it lands in the target. The traveller is a
	// member of both, so the boundary the transfer moves passes straight through
	// them.
	stranger := approvers(t, owner, src.WS, 1)[0]
	traveller := approvers(t, owner, src.WS, 1)[0]
	owner.expect(t, owner.post("/workspaces/"+dst.WS+"/members",
		map[string]any{"email": traveller.Email}), http.StatusCreated)

	// A second project in the source workspace: it stays behind, and so do its
	// documents. The name carries t.Name() because a project slug is globally
	// unique and this file runs alongside the sibling transfer tests.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект "+t.Name())

	stranded := mkProjectDoc(t, owner, src.WS, src.Project, "Регламент со стороны "+t.Name())
	signable := mkProjectDoc(t, owner, src.WS, src.Project, "Регламент попутчика "+t.Name())
	halfSigned := mkProjectDoc(t, owner, src.WS, src.Project, "Регламент наполовину "+t.Name())
	stayDoc := mkProjectDoc(t, owner, src.WS, stayProject, "Регламент остающегося "+t.Name())

	strandedRoute := raiseApproval(t, owner, stranded, stranger.UserID)
	signableRoute := raiseApproval(t, owner, signable, traveller.UserID)
	stayRoute := raiseApproval(t, owner, stayDoc, stranger.UserID)
	// Sequential by default, so the stranger is asked first and the traveller
	// waits behind them. The stranger signs before the move: afterwards the only
	// step still owed is the traveller's, and they can still open the document.
	halfRoute := raiseApproval(t, owner, halfSigned, stranger.UserID, traveller.UserID)
	stranger.expect(t, stranger.post("/document-approvals/"+halfRoute+"/decide",
		map[string]any{"decision": "approved", "signature": "Пётр Иванов"}), http.StatusOK)

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("the transfer reports what it closed", func(t *testing.T) {
		if n, _ := res["cancelled_approvals"].(float64); n != 1 {
			t.Fatalf("cancelled_approvals = %v, want 1: %v", res["cancelled_approvals"], res)
		}
	})

	t.Run("a route left waiting on a stranger is closed", func(t *testing.T) {
		if got := approvalByID(t, owner, stranded, strandedRoute)["status"]; got != "cancelled" {
			t.Fatalf("the unsignable route is %q, want cancelled", got)
		}
	})

	t.Run("and the document is free to be sent round again", func(t *testing.T) {
		// The point of closing it rather than leaving it: while the dead route
		// hung, the one-open-route rule answered 409 to the new owners.
		owner.expect(t, owner.post("/documents/"+stranded+"/approvals",
			map[string]any{"approvers": []any{traveller.UserID}}), http.StatusCreated)
	})

	t.Run("a route whose approver travelled too is untouched", func(t *testing.T) {
		if got := approvalByID(t, owner, signable, signableRoute)["status"]; got != "pending" {
			t.Fatalf("a signable route came out of the transfer as %q, want pending", got)
		}
		traveller.expect(t, traveller.post("/document-approvals/"+signableRoute+"/decide",
			map[string]any{"decision": "approved"}), http.StatusOK)
	})

	t.Run("a signed step is not a debt", func(t *testing.T) {
		if got := approvalByID(t, owner, halfSigned, halfRoute)["status"]; got != "pending" {
			t.Fatalf("a route stalled on nobody came out as %q, want pending", got)
		}
		traveller.expect(t, traveller.post("/document-approvals/"+halfRoute+"/decide",
			map[string]any{"decision": "approved"}), http.StatusOK)
		if got := approvalByID(t, owner, halfSigned, halfRoute)["status"]; got != "approved" {
			t.Fatalf("the route finished as %q, want approved", got)
		}
	})

	t.Run("a route on the project that stayed is not this transfer's business", func(t *testing.T) {
		if got := approvalByID(t, owner, stayDoc, stayRoute)["status"]; got != "pending" {
			t.Fatalf("a route in the source workspace came out as %q, want pending", got)
		}
		stranger.expect(t, stranger.post("/document-approvals/"+stayRoute+"/decide",
			map[string]any{"decision": "approved"}), http.StatusOK)
	})
}

// raiseApproval sends a document round the given approvers, in that order, and
// returns the route's id.
func raiseApproval(t *testing.T, c *client, docID string, approverIDs ...string) string {
	t.Helper()
	ids := make([]any, 0, len(approverIDs))
	for _, id := range approverIDs {
		ids = append(ids, id)
	}
	a := c.expect(t, c.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": ids}), http.StatusCreated)
	return a["id"].(string)
}
