package main

import (
	"net/http"
	"strings"
	"testing"
)

// PDF import and embedding (#2733).
//
// The point of these is the one property that separates PDF from every other
// import: it never touches the sidecar. So none of them wires a stub converter,
// and the first one asserts that absence directly — if PDF ever starts being
// routed through conversion, an install without CONVERTER_URL silently loses a
// feature that used to work, and nothing else in the suite would notice.

// pdfFixture is a payload http.DetectContentType calls application/pdf.
var pdfFixture = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n")

func TestDocumentImportPdfWorksWithoutConverter(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "Смета.pdf", pdfFixture)
	body := c.expect(t, res, http.StatusCreated)

	doc, ok := body["document"].(map[string]any)
	if !ok {
		t.Fatalf("no document in the import response: %#v", body)
	}
	if doc["title"] != "Смета" {
		t.Fatalf("title = %v, want Смета", doc["title"])
	}

	// No html: there is nothing to parse into blocks. The client builds the one
	// pdfEmbed block from this descriptor instead.
	if _, has := body["html"]; has {
		t.Fatalf("a PDF import returned html — it is stored, not converted: %#v", body)
	}
	pdf, ok := body["pdf"].(map[string]any)
	if !ok {
		t.Fatalf("no pdf descriptor in the response: %#v", body)
	}
	src, _ := pdf["src"].(string)
	if !strings.HasPrefix(src, "/api/documents/asset?") {
		t.Fatalf("src = %q, want a signed document-asset URL", src)
	}
	if pdf["name"] != "Смета.pdf" {
		t.Fatalf("name = %v, want the original file name", pdf["name"])
	}
	if pdf["size"] != float64(len(pdfFixture)) {
		t.Fatalf("size = %v, want %d", pdf["size"], len(pdfFixture))
	}

	// And the stored file is actually reachable through that signed URL.
	if got := c.get(strings.TrimPrefix(src, "/api")); got.Status != http.StatusOK {
		t.Fatalf("fetching the stored PDF gave %d, want 200", got.Status)
	}
}

// The extension is not the check. A .pdf that is really a zip would otherwise be
// stored and handed to pdf.js, which is a fetch that fails inside a viewer with
// no way to explain itself.
func TestDocumentImportPdfRejectsAFileThatIsNotOne(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "Смета.pdf", []byte("PK\x03\x04 not a pdf"))
	if res.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Status)
	}
	if n := len(c.get("/workspaces/"+ws+"/documents").listBody(t)); n != 0 {
		t.Fatalf("a refused PDF import left %d documents behind", n)
	}
}

// A pdfEmbed block has to survive the content validator, because the client
// saves the imported body through the ordinary endpoint like any other edit.
func TestDocumentContentAcceptsPdfEmbedBlock(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "Скан.pdf", pdfFixture)
	body := c.expect(t, res, http.StatusCreated)
	doc := body["document"].(map[string]any)
	pdf := body["pdf"].(map[string]any)
	id, _ := doc["id"].(string)

	content := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "pdfEmbed",
				"attrs": map[string]any{
					"src":  pdf["src"],
					"name": pdf["name"],
					"size": pdf["size"],
				},
			},
		},
	}
	saved := c.patch("/documents/"+id+"/content", map[string]any{
		"content":    content,
		"updated_at": doc["updated_at"],
	})
	c.expect(t, saved, http.StatusOK)

	// And it comes back out — a validator that silently strips the block would
	// leave the user with an empty document and no error.
	got := c.get("/documents/" + id).mapBody(t)
	stored, _ := got["content"].(map[string]any)
	nodes, _ := stored["content"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("stored content has %d blocks: %#v", len(nodes), stored)
	}
	if node, _ := nodes[0].(map[string]any); node["type"] != "pdfEmbed" {
		t.Fatalf("stored block is %v, want pdfEmbed", node["type"])
	}
}

// Attaching a PDF to a page that already has text goes through its own route,
// deliberately separate from the images-only asset route.
func TestDocumentPdfUploadRoute(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.post("/workspaces/"+ws+"/documents", map[string]any{"title": "Страница"}).mapBody(t)
	id, _ := doc["id"].(string)

	res := uploadFile(t, c, "/documents/"+id+"/pdf", "file", "Приложение.pdf", pdfFixture)
	body := c.expect(t, res, http.StatusCreated)
	if src, _ := body["src"].(string); !strings.HasPrefix(src, "/api/documents/asset?") {
		t.Fatalf("src = %v, want a signed document-asset URL", body["src"])
	}

	// The images-only contract of the asset route stays intact: widening it
	// would mean every paste and drop handler quietly starts taking PDFs.
	img := uploadFile(t, c, "/documents/"+id+"/assets", "file", "Приложение.pdf", pdfFixture)
	if img.Status != http.StatusBadRequest {
		t.Fatalf("the asset route accepted a PDF (status %d) — it is for images", img.Status)
	}
}
