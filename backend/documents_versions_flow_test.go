package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// docVersions reads a document's journal.
func docVersions(t *testing.T, c *client, docID string) []map[string]any {
	t.Helper()
	return c.get("/documents/" + docID + "/versions").listBody(t)
}

// saveDoc writes content and returns the updated_at the next save has to carry.
func saveDoc(t *testing.T, c *client, docID, at, text string) string {
	t.Helper()
	res := c.expect(t, c.patch("/documents/"+docID+"/content", map[string]any{
		"content": docJSON(text), "updated_at": at,
	}), http.StatusOK)
	return res["updated_at"].(string)
}

// TestDocumentVersionsCoalesceOneSession is the property the whole journal rests
// on: autosave fires every few seconds, so a version per save would bury the
// history under hundreds of near-identical entries. Consecutive saves by the
// same person inside the session window extend one entry instead — and that
// entry always holds the document's current text, which is what compare and
// restore read.
func TestDocumentVersionsCoalesceOneSession(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Черновик"}), http.StatusCreated)
	id := doc["id"].(string)

	at := saveDoc(t, c, id, doc["updated_at"].(string), "первая мысль")
	at = saveDoc(t, c, id, at, "первая мысль и вторая")
	saveDoc(t, c, id, at, "первая мысль, вторая и третья")

	vs := docVersions(t, c, id)
	if len(vs) != 1 {
		t.Fatalf("three autosaves left %d versions, want 1 coalesced: %#v", len(vs), vs)
	}
	if vs[0]["preview"] != "первая мысль, вторая и третья" {
		t.Fatalf("the coalesced version holds %q, not the latest text", vs[0]["preview"])
	}
	if vs[0]["manual"] != false || vs[0]["revision"].(float64) != 1 {
		t.Fatalf("session version came back as %#v", vs[0])
	}
	if _, present := vs[0]["content"]; present {
		t.Fatal("the journal started shipping bodies — that is what makes it cheap to list")
	}
}

// TestDocumentVersionKeepsBaseline covers the state that would otherwise be the
// one state the journal never held: what the document looked like *before* the
// first save it recorded.
func TestDocumentVersionKeepsBaseline(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Регламент"}), http.StatusCreated)
	id := doc["id"].(string)

	// A document that was still empty gets no baseline: there is nothing to go
	// back to, and an empty entry at the bottom of every journal is noise.
	at := saveDoc(t, c, id, doc["updated_at"].(string), "исходный текст")
	if vs := docVersions(t, c, id); len(vs) != 1 {
		t.Fatalf("first save of an empty document left %d versions, want 1", len(vs))
	}

	// Now the document has text. Take a manual snapshot so the next save cannot
	// be folded into the session, then check the pre-edit state is still there.
	c.expect(t, c.post("/documents/"+id+"/versions",
		map[string]any{"label": "перед правкой"}), http.StatusCreated)
	saveDoc(t, c, id, at, "переписанный текст")

	vs := docVersions(t, c, id)
	if len(vs) != 3 {
		t.Fatalf("journal has %d entries, want session + snapshot + new session: %#v", len(vs), vs)
	}
	if vs[0]["preview"] != "переписанный текст" {
		t.Fatalf("newest entry is %q, not the current text", vs[0]["preview"])
	}
	var snapshot map[string]any
	for _, v := range vs {
		if v["label"] == "перед правкой" {
			snapshot = v
		}
	}
	if snapshot == nil {
		t.Fatalf("the manual snapshot is gone: %#v", vs)
	}
	if snapshot["manual"] != true || snapshot["preview"] != "исходный текст" {
		t.Fatalf("snapshot came back as %#v", snapshot)
	}
}

// TestDocumentVersionNewAuthorStartsEntry — a version is attributed to one
// person, so a colleague's save cannot be folded into the entry someone else
// opened. Without this the journal would credit an edit to whoever started the
// session rather than to whoever wrote the text.
func TestDocumentVersionNewAuthorStartsEntry(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	mate := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())
	owner.expect(t, owner.post("/workspaces/"+ws+"/members",
		map[string]any{"email": mate.Email}), http.StatusCreated)
	doc := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Совместный"}), http.StatusCreated)
	id := doc["id"].(string)

	at := saveDoc(t, owner, id, doc["updated_at"].(string), "абзац автора")
	saveDoc(t, mate, id, at, "абзац коллеги")

	vs := docVersions(t, owner, id)
	if len(vs) != 2 {
		t.Fatalf("two authors left %d versions, want one each: %#v", len(vs), vs)
	}
	if vs[0]["author_name"] == vs[1]["author_name"] {
		t.Fatalf("both entries credited to %v", vs[0]["author_name"])
	}
	if vs[0]["preview"] != "абзац коллеги" {
		t.Fatalf("newest entry is %q", vs[0]["preview"])
	}
}

// TestDocumentVersionRestore is the point of the feature: a version taken
// earlier can be put back, and the rollback is itself an entry in the journal —
// so the state it replaced is reachable in turn.
func TestDocumentVersionRestore(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Договор"}), http.StatusCreated)
	id := doc["id"].(string)

	at := saveDoc(t, c, id, doc["updated_at"].(string), "редакция от понедельника")
	snap := c.expect(t, c.post("/documents/"+id+"/versions",
		map[string]any{"label": "согласованная"}), http.StatusCreated)
	saveDoc(t, c, id, at, "редакция от вторника")

	// A single version comes back with its body — that is what compare and
	// preview read, and what the list deliberately omits.
	full := c.expect(t, c.get("/document-versions/"+snap["id"].(string)), http.StatusOK)
	if content, ok := full["content"].(map[string]any); !ok || content["type"] != "doc" {
		t.Fatalf("version content came back as %#v", full["content"])
	}

	restored := c.expect(t, c.post("/document-versions/"+snap["id"].(string)+"/restore", nil),
		http.StatusOK)
	if restored["preview"] != "редакция от понедельника" {
		t.Fatalf("restore left preview %q", restored["preview"])
	}
	body := string(c.get("/documents/" + id).Body)
	if !strings.Contains(body, "редакция от понедельника") {
		t.Fatalf("document was not rolled back: %s", body)
	}

	vs := docVersions(t, c, id)
	// JSON numbers arrive as float64; the label names the revision as an integer.
	wantLabel := "Откат к версии " + strconv.Itoa(int(snap["revision"].(float64)))
	if vs[0]["label"] != wantLabel {
		t.Fatalf("rollback did not head the journal: %#v", vs[0])
	}
	// The replaced text must still be reachable — a rollback nobody can undo is
	// a delete with extra steps.
	var tuesday bool
	for _, v := range vs {
		if v["preview"] == "редакция от вторника" {
			tuesday = true
		}
	}
	if !tuesday {
		t.Fatalf("the state the rollback replaced is gone: %#v", vs)
	}

	// Editing still works after a rollback: the client reloads and saves against
	// the timestamp the restore wrote.
	fresh := c.expect(t, c.get("/documents/"+id), http.StatusOK)
	saveDoc(t, c, id, fresh["updated_at"].(string), "правка после отката")
}

// TestDocumentVersionsRequireMembership — versions carry the document's text, so
// they must be exactly as closed as the document. They have no workspace of
// their own, which is the mistake this test exists to catch: authorization has
// to be resolved through the parent document.
func TestDocumentVersionsRequireMembership(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	stranger := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())
	doc := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Приватный"}), http.StatusCreated)
	id := doc["id"].(string)
	saveDoc(t, owner, id, doc["updated_at"].(string), "внутренний текст")
	vs := docVersions(t, owner, id)
	vid := vs[0]["id"].(string)

	for _, r := range []struct {
		what string
		resp resp
	}{
		{"list", stranger.get("/documents/" + id + "/versions")},
		{"read one", stranger.get("/document-versions/" + vid)},
		{"snapshot", stranger.post("/documents/"+id+"/versions", map[string]any{"label": "мой"})},
		{"restore", stranger.post("/document-versions/"+vid+"/restore", nil)},
	} {
		if r.resp.Status != http.StatusForbidden && r.resp.Status != http.StatusNotFound {
			t.Fatalf("%s by a non-member: status %d, want 403/404\n%s", r.what, r.resp.Status, r.resp.Body)
		}
	}
}
