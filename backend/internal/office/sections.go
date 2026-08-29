package office

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Laying the source file's sections back out over the converted HTML (#2848).
//
// PageSetups reads every section's geometry, but geometry alone only answers
// "how wide is the sheet" — stage #2821 reduced the list to one answer (Widest)
// because there was nowhere in the body to put the others. The editor now has a
// place: the sectionBreak node carries the geometry of everything after it, so
// the list can be spread over the document instead of collapsed into it.
//
// What the section boundary looks like after conversion is measured, not
// assumed: a two-section .docx through the sidecar comes back as
//
//	<p class="western" style="…; page-break-before: always">
//
// on the paragraph that *starts* the second section, and nothing else. The
// geometry is gone (LibreOffice keeps the @page rule of the first section only)
// and so is any name for the boundary, which leaves position as the single
// thing the two sides share: the k-th marker opens the k-th section.

// SectionsInDocumentOrder reports whether PageSetups returns a format's
// sections in the order they appear in the document — the assumption the whole
// positional match rests on.
//
// True for OOXML, where every <w:sectPr> sits at the section it ends, and false
// for ODF, where geometry lives in named page layouts declared in styles.xml
// and reaching document order means resolving paragraph style → master page →
// page layout through a second file. An .odt with mixed orientations therefore
// still imports the way #2821 left it: one sheet, sized to the widest section,
// with the honest warning. Guessing an order out of declaration order would put
// the landscape band in whatever place the file happened to declare it, which
// is worse than the reduction because it looks deliberate.
func SectionsInDocumentOrder(ext string) bool {
	return strings.EqualFold(strings.TrimPrefix(ext, "."), "docx")
}

// sectionMarkerRe matches the start tag of a block LibreOffice broke a page
// before.
//
// Deliberately loose about *which* attribute holds the declaration: the marker
// arrives inline today, and a tag matched on `page-break-before: always`
// anywhere in its attributes is still a page break if the sidecar starts
// writing it differently. The pattern cannot escape the tag it starts in —
// `[^>]*` stops at the first `>` — so a rule of the same name inside a <style>
// block is text this never reaches.
var sectionMarkerRe = regexp.MustCompile(`(?is)<[a-z][a-z0-9]*\b[^>]*?page-break-before\s*:\s*always[^>]*>`)

// pageBreakDeclRe is that declaration on its own, for removing it from the tag
// the break was hoisted out of. Left in place it would be a second boundary in
// the same spot — invisible in the editor, but written back out on export.
var pageBreakDeclRe = regexp.MustCompile(`(?is)\s*page-break-before\s*:\s*always\s*;?`)

// LayoutSections rewrites converted HTML so that each of the source file's
// sections opens with a break carrying its own geometry.
//
// Returns the rewritten HTML and whether the layout could be trusted. It cannot
// when the number of markers disagrees with the number of sections, and that is
// not a rare case: LibreOffice writes the same `page-break-before: always` for a
// *manual* page break inside a section, so a document with one of those has
// more markers than boundaries and no way to say which is which. Measured, not
// feared — a two-section file with one manual break really does come back with
// two identical markers.
//
// The caller then falls back to the single-sheet behaviour of #2821 (widest
// section, and the user told the layout was reduced). Laying the sections out
// positionally anyway would put an arbitrary half of the document in landscape,
// which reads as a bug in the import rather than as the compromise it is.
//
// A file with one section is laid out by definition: there is no boundary to
// place, and its geometry travels on the document node as it always has.
//
// setups must be in document order — see SectionsInDocumentOrder for the
// formats that give it.
func LayoutSections(html string, setups []PageSetup) (string, bool) {
	if len(setups) < 2 {
		return html, true
	}
	marks := sectionMarkerRe.FindAllStringIndex(html, -1)
	if len(marks) != len(setups)-1 {
		return html, false
	}
	var b strings.Builder
	prev := 0
	for i, m := range marks {
		b.WriteString(html[prev:m[0]])
		b.WriteString(sectionBreakHTML(setups[i+1]))
		// The marker's own tag stays — it is the first block of the new section,
		// with its text and its styling — minus the declaration that made it a
		// boundary.
		b.WriteString(pageBreakDeclRe.ReplaceAllString(html[m[0]:m[1]], ""))
		prev = m[1]
	}
	b.WriteString(html[prev:])
	return b.String(), true
}

// sectionBreakHTML renders one break the way the editor's schema parses it
// (frontend/src/utils/docExtensions/sectionBreak.js): a div marked
// `data-section-break`, geometry in `data-page` as the same six-key JSON the
// node stores.
//
// Single-quoted attribute because the value is JSON and JSON is full of double
// quotes; the geometry is six numbers, so there is nothing in it that could
// close the quote early.
func sectionBreakHTML(p PageSetup) string {
	raw, err := json.Marshal(p)
	if err != nil {
		// Six float64s do not fail to marshal. If that ever changes, a break with
		// no geometry is still better than losing the boundary: the section is
		// visible and the user can set its page from the toolbar.
		return `<div data-section-break=""></div>`
	}
	return `<div data-section-break="" data-page='` + string(raw) + `'></div>`
}
