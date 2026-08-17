package main

import (
	"net/http"
	"testing"
)

// mkLinkedDoc builds a workspace with a board and a document in it, so a test can
// link the two without repeating the fixture chain.
func mkLinkedDoc(t *testing.T, c *client) (s stack, docID string) {
	t.Helper()
	s = mkStack(t, c)
	doc := c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Регламент"}), http.StatusCreated)
	return s, doc["id"].(string)
}

// approvers adds n extra members to a workspace and returns their clients.
func approvers(t *testing.T, owner *client, wsID string, n int) []*client {
	t.Helper()
	out := make([]*client, 0, n)
	for i := 0; i < n; i++ {
		m := signup(t)
		owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
			map[string]any{"email": m.Email}), http.StatusCreated)
		out = append(out, m)
	}
	return out
}

// TestDocumentTaskLinkBothDirections walks the relation the whole of point 2
// rests on. Both ends are first-class: a document shows its tasks and a task
// shows its documents, and each side has to carry enough of the other object to
// render a row and navigate to it — otherwise the panel needs a request per link.
func TestDocumentTaskLinkBothDirections(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s, docID := mkLinkedDoc(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Реализовать пункт 3")
	taskID := task["id"].(string)

	link := c.expect(t, c.post("/documents/"+docID+"/tasks", map[string]any{
		"task_id": taskID, "block_id": "b1", "quote": "Исполнитель обязан",
	}), http.StatusCreated)
	if link["block_id"] != "b1" || link["quote"] != "Исполнитель обязан" {
		t.Fatalf("link = %#v", link)
	}

	fromDoc := c.get("/documents/" + docID + "/tasks").listBody(t)
	if len(fromDoc) != 1 {
		t.Fatalf("document side listed %d links, want 1", len(fromDoc))
	}
	if fromDoc[0]["task_title"] != "Реализовать пункт 3" {
		t.Fatalf("document side did not carry the task title: %#v", fromDoc[0])
	}
	if fromDoc[0]["task_board_id"] != s.Board {
		t.Fatalf("document side did not carry the board to navigate to: %#v", fromDoc[0])
	}

	fromTask := c.get("/tasks/" + taskID + "/documents").listBody(t)
	if len(fromTask) != 1 {
		t.Fatalf("task side listed %d links, want 1", len(fromTask))
	}
	if fromTask[0]["document_title"] != "Регламент" || fromTask[0]["document_workspace_id"] != s.WS {
		t.Fatalf("task side did not carry the document: %#v", fromTask[0])
	}

	c.expect(t, c.del("/document-task-links/"+link["id"].(string)), http.StatusNoContent)
	if rest := c.get("/documents/" + docID + "/tasks").listBody(t); len(rest) != 0 {
		t.Fatalf("after unlinking the document still lists %d links", len(rest))
	}
	if rest := c.get("/tasks/" + taskID + "/documents").listBody(t); len(rest) != 0 {
		t.Fatalf("after unlinking the task still lists %d links", len(rest))
	}
}

// TestDocumentTaskLinkAnchorIsPartOfIdentity covers the two rules that decide
// what "the same link" means: an anchored link and a whole-document link to the
// same task coexist (a task may implement one clause of a spec it is also
// attached to), while re-linking the very same triple refreshes the quote instead
// of failing — the button sits in a panel that may be looking at a stale list.
func TestDocumentTaskLinkAnchorIsPartOfIdentity(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s, docID := mkLinkedDoc(t, c)
	taskID := mkTask(t, c, s.Board, s.col(t, 0), "Задача")["id"].(string)

	whole := c.expect(t, c.post("/documents/"+docID+"/tasks",
		map[string]any{"task_id": taskID}), http.StatusCreated)
	anchored := c.expect(t, c.post("/documents/"+docID+"/tasks",
		map[string]any{"task_id": taskID, "block_id": "b1", "quote": "пункт 3"}), http.StatusCreated)
	if whole["id"] == anchored["id"] {
		t.Fatal("the anchored link replaced the whole-document one instead of joining it")
	}
	if links := c.get("/documents/" + docID + "/tasks").listBody(t); len(links) != 2 {
		t.Fatalf("document lists %d links, want both the whole-document and the anchored one", len(links))
	}

	again := c.expect(t, c.post("/documents/"+docID+"/tasks",
		map[string]any{"task_id": taskID, "block_id": "b1", "quote": "пункт 3 в новой редакции"}), http.StatusCreated)
	if again["id"] != anchored["id"] {
		t.Fatal("re-linking the same triple made a second row instead of refreshing it")
	}
	if again["quote"] != "пункт 3 в новой редакции" {
		t.Fatalf("re-linking did not refresh the quote: %#v", again)
	}
	if links := c.get("/documents/" + docID + "/tasks").listBody(t); len(links) != 2 {
		t.Fatalf("re-linking left %d links, want 2", len(links))
	}
}

// TestDocumentTaskLinkRefusesForeignTask is the access rule of the relation.
// Membership is per workspace, so a cross-workspace link would render as a row
// nobody on either side can open — and it would leak the task's title to people
// who cannot see the task.
func TestDocumentTaskLinkRefusesForeignTask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkLinkedDoc(t, c)

	// A second workspace owned by the same user: the caller passes membership on
	// both sides, so what is being tested is the workspace match itself and not
	// authorization standing in for it.
	other := mkStack(t, c)
	foreign := mkTask(t, c, other.Board, other.col(t, 0), "Чужая задача")["id"].(string)

	c.expect(t, c.post("/documents/"+docID+"/tasks",
		map[string]any{"task_id": foreign}), http.StatusBadRequest)
	if links := c.get("/documents/" + docID + "/tasks").listBody(t); len(links) != 0 {
		t.Fatalf("a cross-workspace link was stored anyway: %#v", links)
	}
}

// TestDocumentApprovalPinsVersion is the decision the whole protocol table hangs
// on: a route is raised against a snapshot, never against the live document.
// "Документ согласован" has to mean a specific text was agreed — a route bound to
// the mutable row would be quietly invalidated by the next autosave.
func TestDocumentApprovalPinsVersion(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, docID := mkLinkedDoc(t, owner)
	member := approvers(t, owner, mustWorkspaceOf(t, owner, docID), 1)[0]

	at := owner.expect(t, owner.get("/documents/"+docID), http.StatusOK)["updated_at"].(string)
	owner.expect(t, owner.patch("/documents/"+docID+"/content", map[string]any{
		"content": docJSON("редакция, которую согласуем"), "updated_at": at,
	}), http.StatusOK)

	// Someone who cannot open the document can only ever stall the route. Checked
	// before a route exists, because the open-route conflict below is evaluated
	// first and would otherwise mask this.
	stranger := signup(t)
	owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{stranger.UserID}}), http.StatusBadRequest)
	owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{}}), http.StatusBadRequest)

	approval := owner.expect(t, owner.post("/documents/"+docID+"/approvals", map[string]any{
		"title": "Приказ", "approvers": []any{member.UserID},
	}), http.StatusCreated)
	if approval["status"] != "pending" || approval["mode"] != "sequential" {
		t.Fatalf("new route = %#v", approval)
	}

	// The snapshot must be manual: retention prunes automatic versions, and the
	// FK from document_approvals is RESTRICT — a prunable snapshot would
	// eventually make pruning the document's history fail outright.
	var pinned map[string]any
	for _, v := range docVersions(t, owner, docID) {
		if v["id"] == approval["version_id"] {
			pinned = v
		}
	}
	if pinned == nil {
		t.Fatalf("the route pinned version %v, which is not in the journal", approval["version_id"])
	}
	if pinned["manual"] != true {
		t.Fatalf("the pinned snapshot is prunable: %#v", pinned)
	}
	if pinned["preview"] != "редакция, которую согласуем" {
		t.Fatalf("the pinned snapshot holds %q, not the text sent for approval", pinned["preview"])
	}

	// A second open route would collect signatures on a second revision at the
	// same time, and the badge would stop having a single answer.
	owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{member.UserID}}), http.StatusConflict)
}

// TestDocumentApprovalSequentialRoute is the ordering rule end to end: the second
// approver is asked only once the first has signed, and the route closes itself
// when the last signature lands.
func TestDocumentApprovalSequentialRoute(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, docID := mkLinkedDoc(t, owner)
	ms := approvers(t, owner, s.WS, 2)
	first, second := ms[0], ms[1]

	approval := owner.expect(t, owner.post("/documents/"+docID+"/approvals", map[string]any{
		"title": "Регламент v2", "approvers": []any{first.UserID, second.UserID},
	}), http.StatusCreated)
	id := approval["id"].(string)

	// Out of turn, and from off the route entirely.
	second.expect(t, second.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved"}), http.StatusConflict)
	owner.expect(t, owner.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved"}), http.StatusForbidden)
	first.expect(t, first.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "maybe"}), http.StatusBadRequest)

	signed := first.expect(t, first.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved", "comment": "без замечаний"}), http.StatusOK)
	if signed["status"] != "approved" || signed["comment"] != "без замечаний" {
		t.Fatalf("first signature = %#v", signed)
	}
	// The signature defaults to the name the route recorded, so the two can never
	// disagree about who signed.
	if signed["signature"] == "" {
		t.Fatalf("signature came back empty: %#v", signed)
	}
	first.expect(t, first.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved"}), http.StatusConflict)

	if got := approvalByID(t, owner, docID, id); got["status"] != "pending" {
		t.Fatalf("route closed on the first of two signatures: %#v", got)
	}

	second.expect(t, second.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved", "signature": "Иванов И.И."}), http.StatusOK)

	done := approvalByID(t, owner, docID, id)
	if done["status"] != "approved" {
		t.Fatalf("route after the last signature = %#v", done)
	}
	if done["closed_at"] == nil {
		t.Fatal("an approved route was left open-ended")
	}
	steps, _ := done["steps"].([]any)
	if len(steps) != 2 {
		t.Fatalf("route came back with %d steps, want 2", len(steps))
	}
	last := steps[1].(map[string]any)
	if last["signature"] != "Иванов И.И." {
		t.Fatalf("the explicit signature was not recorded: %#v", last)
	}
	if last["approver_name"] == "" {
		t.Fatal("the route did not record who was asked — a protocol that turns anonymous is not a protocol")
	}
}

// TestDocumentApprovalRejectionClosesRoute covers the branch that ends a route
// early. Leaving the remaining approvers to sign a document that is already going
// back for changes wastes their reading, not just their time.
func TestDocumentApprovalRejectionClosesRoute(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, docID := mkLinkedDoc(t, owner)
	ms := approvers(t, owner, s.WS, 2)
	first, second := ms[0], ms[1]

	approval := owner.expect(t, owner.post("/documents/"+docID+"/approvals", map[string]any{
		"mode": "parallel", "approvers": []any{first.UserID, second.UserID},
	}), http.StatusCreated)
	id := approval["id"].(string)

	// Parallel mode: everyone was asked at once, so the second approver does not
	// have to wait for the first.
	second.expect(t, second.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "rejected", "comment": "не согласен с п.3"}), http.StatusOK)

	closed := approvalByID(t, owner, docID, id)
	if closed["status"] != "rejected" {
		t.Fatalf("one rejection did not close the route: %#v", closed)
	}
	// The first approver never signed, and now cannot: the route is closed.
	first.expect(t, first.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved"}), http.StatusConflict)

	// A closed route frees the document for a new one — that is the loop the
	// whole feature is for: rejected, rewritten, sent again.
	owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{first.UserID}}), http.StatusCreated)
}

// TestDocumentApprovalCancel covers withdrawal and who may do it. A route someone
// else is halfway through signing is not something a passer-by should be able to
// void — and cancelling keeps the protocol, because "sent for approval and pulled
// back" is exactly what a журнал согласований exists to record.
func TestDocumentApprovalCancel(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, docID := mkLinkedDoc(t, owner)
	ms := approvers(t, owner, s.WS, 2)
	approver, bystander := ms[0], ms[1]

	approval := owner.expect(t, owner.post("/documents/"+docID+"/approvals",
		map[string]any{"approvers": []any{approver.UserID}}), http.StatusCreated)
	id := approval["id"].(string)

	bystander.expect(t, bystander.post("/document-approvals/"+id+"/cancel", nil), http.StatusForbidden)
	if got := approvalByID(t, owner, docID, id); got["status"] != "pending" {
		t.Fatalf("a bystander voided the route: %#v", got)
	}

	owner.expect(t, owner.post("/document-approvals/"+id+"/cancel", nil), http.StatusOK)
	cancelled := approvalByID(t, owner, docID, id)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("cancel left the route as %#v", cancelled)
	}
	if cancelled["closed_at"] == nil {
		t.Fatal("a cancelled route was left open-ended")
	}
	owner.expect(t, owner.post("/document-approvals/"+id+"/cancel", nil), http.StatusConflict)
	approver.expect(t, approver.post("/document-approvals/"+id+"/decide",
		map[string]any{"decision": "approved"}), http.StatusConflict)

	// Cancelled, not deleted: the protocol list still holds it.
	if all := docApprovals(t, owner, docID); len(all) != 1 {
		t.Fatalf("cancelling removed the protocol from the journal: %#v", all)
	}
}

// docApprovals reads a document's protocols with their routes.
func docApprovals(t *testing.T, c *client, docID string) []map[string]any {
	t.Helper()
	return c.get("/documents/" + docID + "/approvals").listBody(t)
}

// approvalByID picks one protocol out of the document's list.
func approvalByID(t *testing.T, c *client, docID, id string) map[string]any {
	t.Helper()
	for _, a := range docApprovals(t, c, docID) {
		if a["id"] == id {
			return a
		}
	}
	t.Fatalf("approval %s is not in the document's protocol list", id)
	return nil
}

// mustWorkspaceOf reads a document's workspace, for tests that only kept the
// document id.
func mustWorkspaceOf(t *testing.T, c *client, docID string) string {
	t.Helper()
	doc := c.expect(t, c.get("/documents/"+docID), http.StatusOK)
	return doc["workspace_id"].(string)
}
