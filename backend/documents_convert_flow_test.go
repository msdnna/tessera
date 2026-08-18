package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tessera/internal/converter"
)

// Import/export of office documents against a stub sidecar (#2733).
//
// The stub stands in for LibreOffice, which cannot run in the test tier — but
// the part worth testing is not LibreOffice, it is everything around it: that a
// failed conversion leaves no half-made document, that the pictures it inlines
// become assets instead of a megabyte of base64 in jsonb, that the export sends
// it the document's own text, and that an install without a sidecar keeps the
// rest of the Documents section working.

// pngFixture is a payload http.DetectContentType calls image/png.
var pngFixture = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 24)...)

type stubConverter struct {
	srv      *httptest.Server
	lastHTML string
	fail     bool
}

func newStubConverter(t *testing.T) *stubConverter {
	t.Helper()
	s := &stubConverter{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"ok":true,"sources":["docx","odt"],"targets":["html","pdf","docx"]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		if s.fail {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"LibreOffice produced no output"}`))
			return
		}
		switch r.URL.Query().Get("to") {
		case "html":
			_, _ = w.Write([]byte(`<html><body><p>Импортированный текст</p><img src="data:image/png;base64,` +
				base64.StdEncoding.EncodeToString(pngFixture) + `"></body></html>`))
		default:
			s.lastHTML = string(body)
			_, _ = w.Write([]byte("%PDF-1.4 stub"))
		}
	}))

	// testAPI is process-wide, so the converter is put back afterwards; these
	// tests are deliberately not parallel for the same reason.
	testAPI.WireConverter(converter.New(s.srv.URL))
	t.Cleanup(func() {
		testAPI.WireConverter(converter.New(""))
		s.srv.Close()
	})
	return s
}

func TestDocumentImportCreatesDocumentAndStoresPictures(t *testing.T) {
	newStubConverter(t)
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "Договор.docx", []byte("PK stub docx"))
	body := c.expect(t, res, http.StatusCreated)

	doc, ok := body["document"].(map[string]any)
	if !ok {
		t.Fatalf("no document in the import response: %#v", body)
	}
	// The title comes from the file name minus its extension — asking the user
	// to retype what they just picked is busywork, and an empty title is
	// rejected by create.
	if doc["title"] != "Договор" {
		t.Fatalf("title = %v, want Договор", doc["title"])
	}

	html, _ := body["html"].(string)
	if !strings.Contains(html, "Импортированный текст") {
		t.Fatalf("converted markup not returned: %q", html)
	}
	// The picture must arrive as a signed asset URL. Left as a data: URI it
	// would be parsed into the body and eat the content ceiling.
	if strings.Contains(html, "data:image") {
		t.Fatalf("a data: URI survived into the returned body: %q", html)
	}
	if !strings.Contains(html, "/api/documents/asset?") {
		t.Fatalf("picture was not stored as a document asset: %q", html)
	}
	if body["images_dropped"] != float64(0) {
		t.Fatalf("images_dropped = %v", body["images_dropped"])
	}

	// The import is only half-done at this point: the client parses the HTML
	// with the editor schema and saves it through the ordinary content endpoint,
	// so the same validator guards an import as guards typing.
	list := c.get("/workspaces/" + ws + "/documents").listBody(t)
	if len(list) != 1 {
		t.Fatalf("workspace has %d documents after one import", len(list))
	}
}

func TestDocumentImportRejectsUnsupportedExtension(t *testing.T) {
	newStubConverter(t)
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "notes.md", []byte("# заголовок"))
	if res.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — .md is imported in the browser without a sidecar", res.Status)
	}
	if len(c.get("/workspaces/"+ws+"/documents").listBody(t)) != 0 {
		t.Fatal("a refused import still created a document")
	}
}

func TestDocumentImportLeavesNothingBehindWhenConversionFails(t *testing.T) {
	stub := newStubConverter(t)
	stub.fail = true
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "Битый.docx", []byte("garbage"))
	if res.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (the file is the problem, not the server)", res.Status)
	}
	// This is why the handler converts before it creates: an empty document
	// named after a file that failed to import is litter the user has to find
	// and clean up, and it looks like a partial success.
	if n := len(c.get("/workspaces/"+ws+"/documents").listBody(t)); n != 0 {
		t.Fatalf("a failed conversion left %d documents behind", n)
	}
}

func TestDocumentExportRendersStoredContent(t *testing.T) {
	stub := newStubConverter(t)
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Протокол"}), http.StatusCreated)
	id := doc["id"].(string)
	c.expect(t, c.patch("/documents/"+id+"/content", map[string]any{
		"content":    docJSON("Строка протокола"),
		"updated_at": doc["updated_at"],
	}), http.StatusOK)

	res := c.get("/documents/" + id + "/export?format=pdf")
	if res.Status != http.StatusOK {
		t.Fatalf("export status = %d: %s", res.Status, res.Body)
	}
	if !strings.HasPrefix(string(res.Body), "%PDF") {
		t.Fatalf("export body = %q", res.Body)
	}
	// A Cyrillic title has to survive the download header; without the RFC 5987
	// form the file lands on disk as a row of underscores.
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	// What the sidecar received is the point of the whole server-side renderer:
	// the export carries the document's own text and title, not a client's idea
	// of them.
	if !strings.Contains(stub.lastHTML, "Строка протокола") {
		t.Fatalf("rendered page did not reach the converter: %q", stub.lastHTML)
	}
	if !strings.Contains(stub.lastHTML, "Протокол") {
		t.Fatalf("document title missing from the rendered page: %q", stub.lastHTML)
	}
	if !strings.Contains(stub.lastHTML, `<meta charset="utf-8">`) {
		t.Fatal("no charset declaration — LibreOffice would guess and mangle the Cyrillic")
	}
}

func TestDocumentsWorkWithoutAConverter(t *testing.T) {
	// No stub: testAPI is left with a disabled converter, which is what an
	// install that did not deploy the sidecar looks like.
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Без конвертера"}), http.StatusCreated)
	id := doc["id"].(string)
	c.expect(t, c.patch("/documents/"+id+"/content", map[string]any{
		"content":    docJSON("Текст живёт и без LibreOffice"),
		"updated_at": doc["updated_at"],
	}), http.StatusOK)

	status := c.expect(t, c.get("/document-converter"), http.StatusOK)
	if status["available"] != false {
		t.Fatalf("converter reported available with none configured: %#v", status)
	}
	if status["reason"] == nil {
		t.Fatal("no reason given for the converter being unavailable")
	}

	if res := uploadFile(t, c, "/workspaces/"+ws+"/documents/import", "file", "x.docx", []byte("x")); res.Status != http.StatusServiceUnavailable {
		t.Fatalf("import status = %d, want 503", res.Status)
	}
	if res := c.get("/documents/" + id + "/export?format=pdf"); res.Status != http.StatusServiceUnavailable {
		t.Fatalf("pdf export status = %d, want 503", res.Status)
	}

	// HTML export is rendered here and needs nothing external, so it is the one
	// export an install without the sidecar still gets.
	res := c.get("/documents/" + id + "/export?format=html")
	if res.Status != http.StatusOK {
		t.Fatalf("html export status = %d: %s", res.Status, res.Body)
	}
	if !strings.Contains(string(res.Body), "Текст живёт и без LibreOffice") {
		t.Fatalf("html export body = %q", res.Body)
	}
}
