package handlers

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Laying an imported file's sections back over the converted body (#2848).
//
// #2821 gave a document one sheet and told the user when that was a reduction;
// here the sections survive as breaks in the body, and the reduction is what
// happens only when the breaks cannot be matched.

// The two shapes the sidecar returns, measured on a real two-section .docx: the
// section boundary is an inline page-break on the paragraph that starts the new
// section, and a manual page break inside a section is written exactly the same
// way — which is why the fallback below exists.
const (
	convertedFirst  = `<p class="western" style="margin-bottom: 0in">Portrait.</p>`
	convertedBroken = `<p class="western" style="margin-bottom: 0in; page-break-before: always">Landscape.</p>`
)

// twoSectionDocx builds the smallest .docx that declares a portrait section
// followed by a landscape one — 210×297 mm then 297×210 mm, both in twips.
func twoSectionDocx(t *testing.T) []byte {
	t.Helper()
	const sect = `<w:sectPr><w:pgSz w:w="%W" w:h="%H"/>` +
		`<w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"/></w:sectPr>`
	portrait := strings.NewReplacer("%W", "11906", "%H", "16838").Replace(sect)
	landscape := strings.NewReplacer("%W", "16838", "%H", "11906").Replace(sect)
	body := `<w:p><w:pPr>` + portrait + `</w:pPr></w:p>` +
		`<w:p><w:r><w:t>wide table lives here</w:t></w:r></w:p>` + landscape

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	if _, err := w.Write([]byte(`<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` + body + `</w:body></w:document>`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// twoLayoutOdt is the same document as an .odt: two page layouts in styles.xml,
// which is where ODF keeps geometry — and in declaration order, not document
// order.
func twoLayoutOdt(t *testing.T) []byte {
	t.Helper()
	const styles = `<?xml version="1.0"?>
<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
 xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0">
<office:automatic-styles>
<style:page-layout style:name="pm1"><style:page-layout-properties fo:page-width="210mm" fo:page-height="297mm" fo:margin-left="20mm" fo:margin-right="20mm" fo:margin-top="20mm" fo:margin-bottom="20mm"/></style:page-layout>
<style:page-layout style:name="pm2"><style:page-layout-properties fo:page-width="297mm" fo:page-height="210mm" fo:margin-left="10mm" fo:margin-right="10mm" fo:margin-top="15mm" fo:margin-bottom="15mm"/></style:page-layout>
</office:automatic-styles></office:document-styles>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("styles.xml")
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	if _, err := w.Write([]byte(styles)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

func TestImportedPageSetupLaysSectionsOut(t *testing.T) {
	_, doc := importFixture(t)
	body, page, differ := importedPageSetup(
		convertedFirst+convertedBroken, ".docx", twoSectionDocx(t), doc, "two.docx")

	if !strings.Contains(body, `data-section-break`) {
		t.Fatalf("no section break in the imported body:\n%s", body)
	}
	if !strings.Contains(body, `"w":297`) {
		t.Fatalf("the break does not carry the landscape section:\n%s", body)
	}
	// The document node keeps the *first* section now, not the widest: the wide
	// one has a place of its own in the body, and putting it on the document
	// would turn the portrait half landscape.
	if page == nil || page.Width != 210 || page.Height != 297 {
		t.Fatalf("document geometry = %+v, want the first section (210×297)", page)
	}
	// Nothing was reduced, so nothing to warn about — the warning is what makes
	// the #2821 fallback honest, and firing it here would train the user to
	// ignore it.
	if differ {
		t.Fatalf("a document laid out section by section reported itself reduced")
	}
}

func TestImportedPageSetupFallsBackWhenBreaksDoNotMatch(t *testing.T) {
	_, doc := importFixture(t)
	html := convertedFirst +
		`<p style="page-break-before: always">Still the first section.</p>` +
		convertedBroken
	body, page, differ := importedPageSetup(html, ".docx", twoSectionDocx(t), doc, "manual.docx")

	if body != html {
		t.Fatalf("body rewritten despite an unmatchable layout:\n%s", body)
	}
	// #2821's behaviour, unchanged: the widest section, and the user told the
	// sheet is a reduction of the file.
	if page == nil || page.Width != 297 {
		t.Fatalf("fallback geometry = %+v, want the widest section (297 wide)", page)
	}
	if !differ {
		t.Fatalf("sections disagreed and the user was not told")
	}
}

// ODF is refused before the markers are even counted: PageSetups hands its
// layouts back in declaration order, so a positional match would place the
// landscape band wherever the file happened to declare it.
func TestImportedPageSetupLeavesOdtAlone(t *testing.T) {
	_, doc := importFixture(t)
	html := convertedFirst + convertedBroken
	body, page, differ := importedPageSetup(html, ".odt", twoLayoutOdt(t), doc, "two.odt")

	if body != html {
		t.Fatalf("laid sections out for a format whose order is not document order:\n%s", body)
	}
	if page == nil || page.Width != 297 {
		t.Fatalf("geometry = %+v, want the widest layout", page)
	}
	if !differ {
		t.Fatalf("the .odt reduction went unreported")
	}
}

// A format with no geometry to read is not an error and must not touch the
// body: .txt, .rtf and .html are legitimate imports that simply have nothing to
// say about pages.
func TestImportedPageSetupPassesThroughUnreadableFormats(t *testing.T) {
	_, doc := importFixture(t)
	html := convertedFirst + convertedBroken
	body, page, differ := importedPageSetup(html, ".txt", []byte("plain text"), doc, "notes.txt")

	if body != html || page != nil || differ {
		t.Fatalf("got (%q, %+v, %v), want the body untouched and no geometry", body, page, differ)
	}
}
