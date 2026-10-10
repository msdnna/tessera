package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// What a deleted document leaves on disk (#2932).
//
// A document's images do not go to /api/uploads — they live under
// documents/<id>/ and are reached through an HMAC-signed URL (handlers
// .UploadDocumentAsset). Nothing in the database records them: the only pointer
// to a file is the src attribute inside the document's own body. Delete the
// document and that pointer goes with it, so whatever is left under
// documents/<id>/ is unreachable forever — no row names it, no sweep walks it.
// An imported .docx makes this plural rather than theoretical: the converter
// unpacks every image of the file into that one directory.
//
// The sweep is not a bare RemoveAll, though, and the second test below is why:
// a template copies a document's body verbatim, signed image URLs included, so
// the files can be shared with a template and with every document made from it.
// Both halves are pinned here.

// docAssetDir is where a document's images live (handlers.docAssetPath).
func docAssetDir(docID string) string {
	return filepath.Join(testUploadDir, "documents", docID)
}

// pngBytes is a body http.DetectContentType reads as image/png — the upload
// gates on the sniffed bytes, not on the filename.
func pngBytes(tail string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), tail...)
}

// mkDocument creates a document in a workspace and returns its id.
func mkDocument(t *testing.T, c *client, wsID, title string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": title}), http.StatusCreated)
	return m["id"].(string)
}

// putDocImages uploads images into a document and saves a body that embeds
// them, which is what the editor does on paste: upload, then write the returned
// signed URL into an image node.
func putDocImages(t *testing.T, c *client, docID string, names ...string) []string {
	t.Helper()
	nodes := []any{map[string]any{
		"type":    "paragraph",
		"content": []any{map[string]any{"type": "text", "text": "Иллюстрации"}},
	}}
	urls := make([]string, 0, len(names))
	for _, name := range names {
		r := uploadFile(t, c, "/documents/"+docID+"/assets", "file", name, pngBytes(name))
		if r.Status != http.StatusCreated {
			t.Fatalf("upload %s: status %d\n%s", name, r.Status, r.Body)
		}
		url, ok := r.mapBody(t)["url"].(string)
		if !ok || url == "" {
			t.Fatalf("upload %s returned no url: %s", name, r.Body)
		}
		urls = append(urls, url)
		nodes = append(nodes, map[string]any{
			"type":  "image",
			"attrs": map[string]any{"src": url, "alt": name},
		})
	}
	saveDocContent(t, c, docID, map[string]any{"type": "doc", "content": nodes})
	return urls
}

// saveDocContent writes a body, passing the document's current updated_at —
// the content save is guarded against a concurrent edit and refuses without it.
func saveDocContent(t *testing.T, c *client, docID string, content any) {
	t.Helper()
	cur := c.expect(t, c.get("/documents/"+docID), http.StatusOK)
	c.expect(t, c.patch("/documents/"+docID+"/content", map[string]any{
		"content": content, "updated_at": cur["updated_at"],
	}), http.StatusOK)
}

// TestDeletingADocumentTakesItsImagesWithIt: the bytes behind a deleted
// document's images must not outlive the body that named them.
//
// Deleting a document is the ordinary way to get rid of one — there is no
// archive, and recursive=true removes a whole subtree in a single gesture. Every
// image ever pasted into it sits under documents/<id>/, named only by the src
// attributes the delete has just removed, so anything left there is both
// invisible and permanent: the author has no way to learn the directory is
// still full, and no code path ever looks at it again.
func TestDeletingADocumentTakesItsImagesWithIt(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())

	doc := mkDocument(t, owner, ws, "Регламент с картинками")
	putDocImages(t, owner, doc, "scheme.png", "screenshot.png")

	dir := docAssetDir(doc)
	if n, ok := countFiles(t, dir); !ok || n != 2 {
		t.Fatalf("document images did not land: %d file(s) in %s (exists=%v)", n, dir, ok)
	}

	// A second document in the same workspace, so "swept" cannot quietly mean
	// "swept documents/". The directories are siblings under one root and the
	// cheapest wrong fix — clearing the root — would pass every other assertion.
	neighbour := mkDocument(t, owner, ws, "Соседний документ")
	putDocImages(t, owner, neighbour, "neighbour.png")

	deleteForGood(t, owner, "documents", doc, "")

	// The document is gone from the API...
	if got := owner.get("/documents/" + doc); got.Status != http.StatusNotFound {
		t.Fatalf("deleted document still readable: %d", got.Status)
	}
	// ...so nothing can name these files any more.
	if n, ok := countFiles(t, dir); ok {
		t.Fatalf("asset directory survived the document with %d file(s) no body points at", n)
	}
	// The document next door still has its image.
	if n, ok := countFiles(t, docAssetDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a document that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingADocumentKeepsImagesATemplateStillUses is the other half, and the
// reason the sweep cannot be a plain RemoveAll.
//
// Saving a document as a template copies its body verbatim (seedFromTemplate
// says so: copied, not referenced), and the body carries signed URLs pointing at
// the *original* document's directory. So do the documents later created from
// that template. Deleting the source document therefore must not take the files
// with it — the gallery tile and every protocol made from it would render broken
// images, and unlike the leak this failure is loud and unrecoverable.
func TestDeletingADocumentKeepsImagesATemplateStillUses(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())

	source := mkDocument(t, owner, ws, "Протокол совещания")
	putDocImages(t, owner, source, "logo.png")

	tpl := owner.expect(t, owner.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": source, "title": "Протокол"}), http.StatusCreated)
	made := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Протокол от 13.09", "template_id": tpl["id"]}), http.StatusCreated)

	deleteForGood(t, owner, "documents", source, "")

	if n, ok := countFiles(t, docAssetDir(source)); !ok || n != 1 {
		t.Fatalf("images a template still embeds were swept: %d file(s) left (exists=%v)", n, ok)
	}
	// And the copy really does point at them, which is what makes the sweep unsafe
	// rather than merely untidy.
	body := owner.expect(t, owner.get("/documents/"+made["id"].(string)), http.StatusOK)
	if !docBodyMentions(t, body, source) {
		t.Fatalf("document made from the template does not embed the source's assets: %#v", body["content"])
	}
}

// TestDeletingADocumentSubtreeTakesEveryChildsImages: recursive=true is one
// gesture that removes many documents, and a sweep wired only to the row the
// request names would leave every nested document's images behind.
func TestDeletingADocumentSubtreeTakesEveryChildsImages(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())

	parent := mkDocument(t, owner, ws, "Раздел")
	putDocImages(t, owner, parent, "cover.png")
	child := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Подраздел", "parent_id": parent}), http.StatusCreated)
	childID := child["id"].(string)
	putDocImages(t, owner, childID, "inner.png", "inner2.png")

	if n, ok := countFiles(t, docAssetDir(childID)); !ok || n != 2 {
		t.Fatalf("nested document images did not land: %d (exists=%v)", n, ok)
	}

	deleteForGood(t, owner, "documents", parent, "?recursive=true")

	if n, ok := countFiles(t, docAssetDir(parent)); ok {
		t.Fatalf("parent kept %d image file(s) after the subtree delete", n)
	}
	if n, ok := countFiles(t, docAssetDir(childID)); ok {
		t.Fatalf("nested document kept %d image file(s) after the subtree delete", n)
	}
}

// TestDeletingADocumentWithoutImagesIsFine pins the overwhelmingly common
// delete — a document nobody pasted anything into. A sweep that only works when
// there is something to sweep is one that fails the moment it matters.
func TestDeletingADocumentWithoutImagesIsFine(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())

	doc := mkDocument(t, owner, ws, "Документ без картинок")
	saveDocContent(t, owner, doc, docJSON("только текст"))

	deleteForGood(t, owner, "documents", doc, "")

	if n, ok := countFiles(t, docAssetDir(doc)); ok {
		t.Fatalf("directory appeared for a document with no images: %d entr(ies)", n)
	}
}

// docBodyMentions reports whether a document response embeds an asset of docID.
func docBodyMentions(t *testing.T, body map[string]any, docID string) bool {
	t.Helper()
	content, ok := body["content"].(map[string]any)
	if !ok {
		return false
	}
	nodes, _ := content["content"].([]any)
	for _, raw := range nodes {
		n, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		attrs, _ := n["attrs"].(map[string]any)
		src, _ := attrs["src"].(string)
		if src != "" && strings.Contains(src, "doc="+docID) {
			return true
		}
	}
	return false
}
