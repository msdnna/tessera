package main

import (
	"net/http"
	"strings"
	"testing"
)

// mkCommentedDoc builds a workspace with one document and returns both ids.
func mkCommentedDoc(t *testing.T, c *client) (wsID, docID string) {
	t.Helper()
	wsID = mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": "Регламент"}), http.StatusCreated)
	return wsID, doc["id"].(string)
}

// TestDocumentCommentThread walks the shape the panel is built on: a root
// anchored to a block, a reply that inherits the anchor, and a resolve that
// closes the thread and can be undone.
func TestDocumentCommentThread(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkCommentedDoc(t, c)

	root := c.expect(t, c.post("/documents/"+docID+"/comments", map[string]any{
		"block_id": "b1", "body": "Здесь нужен срок", "quote": "Исполнитель обязан",
	}), http.StatusCreated)
	if root["block_id"] != "b1" || root["quote"] != "Исполнитель обязан" {
		t.Fatalf("root comment = %#v", root)
	}
	if root["parent_id"] != nil || root["resolved_at"] != nil {
		t.Fatalf("a fresh root came back threaded or resolved: %#v", root)
	}

	// A reply says nothing about where it hangs: the server takes the anchor from
	// the thread it answers, so a client cannot file a reply against another block.
	reply := c.expect(t, c.post("/documents/"+docID+"/comments", map[string]any{
		"parent_id": root["id"], "block_id": "b9", "body": "Добавил", "quote": "чужая цитата",
	}), http.StatusCreated)
	if reply["block_id"] != "b1" {
		t.Fatalf("reply anchored to %v, want the root's block", reply["block_id"])
	}
	if reply["quote"] != "" {
		t.Fatalf("reply kept a quote: %v", reply["quote"])
	}

	// Threads are one level deep: answering a reply joins the same root instead of
	// nesting, so the panel never has to render a tree.
	deep := c.expect(t, c.post("/documents/"+docID+"/comments", map[string]any{
		"parent_id": reply["id"], "body": "И ещё",
	}), http.StatusCreated)
	if deep["parent_id"] != root["id"] {
		t.Fatalf("reply to a reply hung off %v, want the root", deep["parent_id"])
	}

	list := c.get("/documents/" + docID + "/comments").listBody(t)
	if len(list) != 3 {
		t.Fatalf("list has %d comments, want 3", len(list))
	}
	if list[0]["author_name"] == nil {
		t.Fatal("list came back without author names — the panel has nothing to show")
	}

	resolved := c.expect(t, c.patch("/document-comments/"+root["id"].(string)+"/resolve",
		map[string]any{}), http.StatusOK)
	if resolved["resolved_at"] == nil || resolved["resolved_by"] != c.UserID {
		t.Fatalf("resolve = %#v", resolved)
	}
	reopened := c.expect(t, c.patch("/document-comments/"+root["id"].(string)+"/resolve",
		map[string]any{"resolved": false}), http.StatusOK)
	if reopened["resolved_at"] != nil || reopened["resolved_by"] != nil {
		t.Fatalf("reopen left the thread resolved: %#v", reopened)
	}
}

// TestDocumentCommentResolveRootOnly pins the rule the table's CHECK also
// carries: resolution is a property of the thread, and a resolvable reply would
// give the client two disagreeing answers to "is this handled".
func TestDocumentCommentResolveRootOnly(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkCommentedDoc(t, c)
	root := c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"block_id": "b1", "body": "Замечание"}), http.StatusCreated)
	reply := c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"parent_id": root["id"], "body": "Ответ"}), http.StatusCreated)

	if r := c.patch("/document-comments/"+reply["id"].(string)+"/resolve", map[string]any{}); r.Status != http.StatusBadRequest {
		t.Fatalf("resolving a reply: status %d, want 400\n%s", r.Status, r.Body)
	}
}

// TestDocumentCommentDeleteCascades checks that deleting a root takes its
// replies with it — an orphaned reply would render as a thread with no question.
func TestDocumentCommentDeleteCascades(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkCommentedDoc(t, c)
	root := c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"block_id": "b1", "body": "Корень"}), http.StatusCreated)
	c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"parent_id": root["id"], "body": "Ответ"}), http.StatusCreated)

	if r := c.del("/document-comments/" + root["id"].(string)); r.Status != http.StatusNoContent {
		t.Fatalf("delete: status %d\n%s", r.Status, r.Body)
	}
	if list := c.get("/documents/" + docID + "/comments").listBody(t); len(list) != 0 {
		t.Fatalf("%d comments survived the root delete", len(list))
	}
}

// TestDocumentCommentDeletedWithDocument covers the FK the migration leans on:
// a deleted document must not leave its discussions behind in the table.
func TestDocumentCommentDeletedWithDocument(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkCommentedDoc(t, c)
	cm := c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"block_id": "b1", "body": "Замечание"}), http.StatusCreated)

	if r := c.del("/documents/" + docID); r.Status != http.StatusNoContent {
		t.Fatalf("delete document: status %d\n%s", r.Status, r.Body)
	}
	if r := c.get("/document-comments/" + cm["id"].(string)); r.Status < 400 {
		t.Fatalf("comment still reachable after its document was deleted: %d", r.Status)
	}
}

// TestDocumentCommentPermissions is the authorization matrix for the panel:
// a member may reply and resolve anyone's thread, but only the author may edit
// or delete their own words — and a non-member sees nothing at all.
func TestDocumentCommentPermissions(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	member := signup(t)
	outsider := signup(t)
	wsID, docID := mkCommentedDoc(t, owner)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": member.Email}), http.StatusCreated)

	root := owner.expect(t, owner.post("/documents/"+docID+"/comments",
		map[string]any{"block_id": "b1", "body": "Нужно уточнить"}), http.StatusCreated)
	id := root["id"].(string)

	// A member is a participant: replying and closing the thread are the point.
	member.expect(t, member.post("/documents/"+docID+"/comments",
		map[string]any{"parent_id": id, "body": "Уточнил"}), http.StatusCreated)
	member.expect(t, member.patch("/document-comments/"+id+"/resolve", map[string]any{}), http.StatusOK)

	// Rewriting or deleting what someone else said is not.
	if r := member.patch("/document-comments/"+id, map[string]any{"body": "подмена"}); r.Status != http.StatusForbidden {
		t.Fatalf("member editing another's comment: status %d, want 403", r.Status)
	}
	if r := member.del("/document-comments/" + id); r.Status != http.StatusForbidden {
		t.Fatalf("member deleting another's comment: status %d, want 403", r.Status)
	}

	edited := owner.expect(t, owner.patch("/document-comments/"+id,
		map[string]any{"body": "Нужно уточнить срок"}), http.StatusOK)
	if edited["body"] != "Нужно уточнить срок" {
		t.Fatalf("author edit = %#v", edited)
	}

	// The document is reachable by id alone, so membership is the only thing
	// standing between an outsider and someone else's review notes.
	if r := outsider.get("/documents/" + docID + "/comments"); r.Status != http.StatusForbidden {
		t.Fatalf("outsider list: status %d, want 403\n%s", r.Status, r.Body)
	}
	if r := outsider.post("/documents/"+docID+"/comments",
		map[string]any{"block_id": "b1", "body": "чужой"}); r.Status != http.StatusForbidden {
		t.Fatalf("outsider create: status %d, want 403", r.Status)
	}
	if r := outsider.patch("/document-comments/"+id+"/resolve", map[string]any{}); r.Status != http.StatusForbidden {
		t.Fatalf("outsider resolve: status %d, want 403", r.Status)
	}
}

// TestDocumentCommentCrossDocumentReply keeps a thread inside its document: the
// parent id comes from the client, and trusting it would let a comment appear on
// a document the reply was never written for.
func TestDocumentCommentCrossDocumentReply(t *testing.T) {
	t.Parallel()
	c := signup(t)
	wsID, docID := mkCommentedDoc(t, c)
	other := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": "Другой"}), http.StatusCreated)
	root := c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"block_id": "b1", "body": "Корень"}), http.StatusCreated)

	r := c.post("/documents/"+other["id"].(string)+"/comments",
		map[string]any{"parent_id": root["id"], "body": "Ответ не туда"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("cross-document reply: status %d, want 400\n%s", r.Status, r.Body)
	}
}

// TestDocumentCommentQuoteTruncated guards the one field a client can grow
// without limit: the quote is a reminder of what was annotated, not a copy of
// the document.
func TestDocumentCommentQuoteTruncated(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkCommentedDoc(t, c)
	// Cyrillic on purpose: a byte-wise cut would split a character and the body
	// would come back as broken UTF-8.
	long := strings.Repeat("я", 800)
	cm := c.expect(t, c.post("/documents/"+docID+"/comments", map[string]any{
		"block_id": "b1", "body": "Замечание", "quote": long,
	}), http.StatusCreated)
	quote, _ := cm["quote"].(string)
	if n := len([]rune(quote)); n != 500 {
		t.Fatalf("quote kept %d runes, want 500", n)
	}
	if !strings.HasPrefix(long, quote) {
		t.Fatal("quote is not a prefix of what was sent — the cut split a character")
	}
}

// TestDocumentCommentDocumentLevel covers the anchor-less thread: a remark about
// the document as a whole, which is what an empty block id means.
func TestDocumentCommentDocumentLevel(t *testing.T) {
	t.Parallel()
	c := signup(t)
	_, docID := mkCommentedDoc(t, c)
	cm := c.expect(t, c.post("/documents/"+docID+"/comments",
		map[string]any{"body": "В целом согласовано"}), http.StatusCreated)
	if cm["block_id"] != "" {
		t.Fatalf("block_id = %v, want empty", cm["block_id"])
	}
	if r := c.post("/documents/"+docID+"/comments", map[string]any{"block_id": "b1"}); r.Status != http.StatusBadRequest {
		t.Fatalf("empty body: status %d, want 400", r.Status)
	}
}
