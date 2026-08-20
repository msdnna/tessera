package handlers

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/converter"
	"tessera/internal/db"
)

// pngBytes is a payload http.DetectContentType calls image/png. Only the
// signature matters — the handler sniffs, it does not decode.
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)

func dataImg(mime string, raw []byte) string {
	return `<img src="data:` + mime + ";base64," + base64.StdEncoding.EncodeToString(raw) + `">`
}

func importFixture(t *testing.T) (*API, db.Document) {
	t.Helper()
	h := &API{uploadDir: t.TempDir(), assetKey: []byte("test-key")}
	return h, db.Document{ID: uuid.New(), WorkspaceID: uuid.New()}
}

func TestStoreImportedImagesWritesAssetsAndRewritesSrc(t *testing.T) {
	h, doc := importFixture(t)
	page := "<p>до</p>" + dataImg("image/png", pngBytes) + "<p>после</p>" + dataImg("image/png", pngBytes)

	out, stats := h.storeImportedImages(doc, page)
	if stats.dropped != 0 {
		t.Fatalf("dropped %d pictures from a valid document (%s)", stats.dropped, stats.summary())
	}
	if stats.saved != 2 {
		t.Fatalf("saved = %d, want 2", stats.saved)
	}
	if strings.Contains(out, "data:image") {
		t.Fatalf("a data: URI survived into the stored body: %q", out)
	}
	// Storing base64 in the body would eat the 4 MiB content ceiling with a
	// couple of screenshots, which is the whole reason the pictures are split
	// out into assets here rather than left inline.
	if n := strings.Count(out, "/api/documents/asset?"); n != 2 {
		t.Fatalf("expected 2 signed asset URLs, got %d in %q", n, out)
	}
	if !strings.Contains(out, "&amp;") {
		t.Fatal("the asset URL's & was not escaped for an HTML attribute")
	}

	entries, err := os.ReadDir(filepath.Join(h.uploadDir, "documents", doc.ID.String()))
	if err != nil {
		t.Fatalf("asset directory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("wrote %d files, want 2", len(entries))
	}
}

func TestStoreImportedImagesDropsNonImages(t *testing.T) {
	h, doc := importFixture(t)
	// The MIME type in a data: URI is declared by the file we just converted,
	// i.e. by something a user supplied. Trusting it would let an import write
	// arbitrary content into the assets directory under an image's name.
	page := dataImg("image/png", []byte("<html>not an image at all</html>"))

	out, stats := h.storeImportedImages(doc, page)
	if stats.dropped != 1 {
		t.Fatalf("dropped = %d, want 1", stats.dropped)
	}
	// The reason is the point of the count: "1 dropped" alone sent #2755 to an
	// hour of forensics before anyone knew whether it was the format, a ceiling
	// or a failed write.
	if stats.reasons[dropUnsupported] != 1 {
		t.Fatalf("reasons = %v, want unsupported_type", stats.reasons)
	}
	if strings.Contains(out, "<img") {
		t.Fatalf("dropped picture left an <img> behind: %q", out)
	}
	if _, err := os.Stat(filepath.Join(h.uploadDir, "documents", doc.ID.String())); !os.IsNotExist(err) {
		t.Fatal("a directory was created for a document with no valid pictures")
	}
}

func TestStoreImportedImagesDropsOversizedPicture(t *testing.T) {
	h, doc := importFixture(t)
	big := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxImportAssetBytes+1)...)

	_, stats := h.storeImportedImages(doc, dataImg("image/png", big))
	if stats.dropped != 1 {
		t.Fatalf("dropped = %d, want 1 (picture over the per-asset ceiling)", stats.dropped)
	}
	if stats.reasons[dropTooLarge] != 1 {
		t.Fatalf("reasons = %v, want too_large", stats.reasons)
	}
}

func TestStoreImportedImagesLeavesLinkedPicturesAlone(t *testing.T) {
	h, doc := importFixture(t)
	page := `<img src="https://example.org/logo.png">`
	out, stats := h.storeImportedImages(doc, page)
	if out != page || stats.dropped != 0 {
		t.Fatalf("an externally linked picture was rewritten: %q (dropped %d)", out, stats.dropped)
	}
}

func TestImportExtensionsExcludeBrowserHandledFormats(t *testing.T) {
	// .md and .json already import in the browser without a sidecar (D9,
	// docImport.js). Accepting them here too would make an import that works on
	// every install start depending on LibreOffice being deployed.
	for _, ext := range []string{".md", ".markdown", ".json"} {
		if docImportExts[ext] {
			t.Errorf("%s is routed through the converter", ext)
		}
	}
	for _, ext := range []string{".docx", ".odt", ".doc", ".rtf"} {
		if !docImportExts[ext] {
			t.Errorf("%s is not accepted for import", ext)
		}
	}
}

func TestExportFormatsFollowSidecarCapabilities(t *testing.T) {
	got := exportFormatsFor(converter.Info{Targets: []string{"pdf", "html"}})
	if strings.Join(got, ",") != "html,pdf" {
		t.Fatalf("got %v, want [html pdf] — docx must not be offered when the sidecar cannot produce it", got)
	}

	// html is rendered here, so it stays on offer even when the sidecar says
	// nothing at all: the cheapest useful export should not need the heaviest
	// dependency.
	got = exportFormatsFor(converter.Info{})
	if strings.Join(got, ",") != "html" {
		t.Fatalf("got %v, want [html]", got)
	}
}

func TestContentDispositionSurvivesCyrillic(t *testing.T) {
	got := contentDisposition("Протокол совещания.pdf")
	if !strings.Contains(got, "filename*=UTF-8''") {
		t.Fatalf("no RFC 5987 filename: %q", got)
	}
	if !strings.Contains(got, "%D0%9F") {
		t.Fatalf("title not percent-encoded: %q", got)
	}
	// The ASCII fallback must still be a quoted string a naive client can parse,
	// not a header broken by an embedded quote.
	if strings.Count(got, `"`) != 2 {
		t.Fatalf("malformed ASCII fallback: %q", got)
	}
	if got := contentDisposition(`ev"il.pdf`); strings.Count(got, `"`) != 2 {
		t.Fatalf("a quote in the title escaped into the header: %q", got)
	}
}
