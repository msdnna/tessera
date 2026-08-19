package handlers

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Reporting of what an import did with the pictures LibreOffice inlined
// (#2755). The count alone was not actionable: a document that came back
// "15 images dropped" gave no way to tell a format we refuse from a ceiling we
// hit from a write that failed, and the log said nothing at all.

func TestStoreImportedImagesKeepsLibreOfficeWrappedPayload(t *testing.T) {
	h, doc := importFixture(t)
	// LibreOffice wraps the base64 it inlines at 76 columns and the sidecar
	// passes that through, so the wrapped form is what the regex and the decoder
	// have to survive. A figure per section is the normal shape of a converted
	// Word file — #2755 arrived with fifteen.
	raw := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 4096)...)
	encoded := base64.StdEncoding.EncodeToString(raw)
	var wrapped strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		wrapped.WriteString(encoded[i:end] + "\n")
	}
	var page strings.Builder
	for i := 0; i < 15; i++ {
		page.WriteString(`<p><img src="data:image/png;base64,` + wrapped.String() + `" width="480"></p>`)
	}

	out, stats := h.storeImportedImages(doc, page.String())
	if stats.dropped != 0 || stats.saved != 15 {
		t.Fatalf("saved %d, dropped %d (%s); want 15 saved, 0 dropped", stats.saved, stats.dropped, stats.summary())
	}
	if strings.Contains(out, "data:image") {
		t.Fatal("a wrapped data: URI survived into the stored body")
	}
	// The attributes around src carry the size LibreOffice computed; losing them
	// while rewriting would rescale every figure in the document.
	if n := strings.Count(out, `width="480"`); n != 15 {
		t.Fatalf("rewriting src dropped %d sibling attributes", 15-n)
	}
}

func TestImportImageStatsReportReasons(t *testing.T) {
	h, doc := importFixture(t)
	// An EMF drawing is what a Word file pasted from Visio or Excel is full of:
	// the data: URI still declares image/png, and only the bytes say otherwise.
	// This is the drop a user cannot act on unless they are told which it was.
	emf := append([]byte("\x01\x00\x00\x00 EMF"), make([]byte, 64)...)
	page := dataImg("image/png", pngBytes) + dataImg("image/png", emf) + dataImg("image/png", emf)

	_, stats := h.storeImportedImages(doc, page)
	if stats.saved != 1 || stats.dropped != 2 {
		t.Fatalf("saved %d, dropped %d; want 1 saved, 2 dropped", stats.saved, stats.dropped)
	}
	if got := stats.counts()["unsupported_type"]; got != 2 {
		t.Fatalf("counts() = %v, want unsupported_type: 2", stats.counts())
	}
	if !strings.Contains(stats.summary(), "формат картинки не поддерживается — 2") {
		t.Fatalf("summary() = %q — the warning has to name the reason", stats.summary())
	}

	// A clean import must not hand the client an empty reason to render.
	_, clean := h.storeImportedImages(doc, dataImg("image/png", pngBytes))
	if clean.summary() != "" || clean.counts() != nil {
		t.Fatalf("a clean import reported %q / %v", clean.summary(), clean.counts())
	}
}

func TestImportImageStatsLogSampleIsBounded(t *testing.T) {
	// One malformed document must not write a log line per picture: the sample
	// is what identifies the offender, the aggregate is what counts them.
	stats := importImageStats{}
	for i := 0; i < maxDropLogLines*3; i++ {
		stats.drop(dropUnsupported, "image/png", 128)
	}
	if len(stats.details) != maxDropLogLines {
		t.Fatalf("kept %d log details, want %d", len(stats.details), maxDropLogLines)
	}
	if suffix := logSuffix(stats); !strings.HasSuffix(suffix, "…") {
		t.Fatalf("a truncated sample must say so: %q", suffix)
	}
}
