package office

import (
	"strings"
	"testing"
)

var (
	portrait  = PageSetup{Width: 210, Height: 297, Left: 20, Right: 20, Top: 20, Bottom: 20}
	landscape = PageSetup{Width: 297, Height: 210, Left: 10, Right: 10, Top: 15, Bottom: 15}
	a5        = PageSetup{Width: 148, Height: 210, Left: 10, Right: 10, Top: 10, Bottom: 10}
)

// The markup the sidecar actually returns, copied from a two-section .docx run
// through it: the boundary is an inline `page-break-before: always` on the
// paragraph that starts the new section, and there is nothing else left of it.
const (
	firstPara  = `<p class="western" style="line-height: 100%; margin-bottom: 0in">Portrait section body text.</p>`
	brokenPara = `<p class="western" style="line-height: 100%; margin-bottom: 0in; page-break-before: always">Landscape section body text.</p>`
)

func TestLayoutSectionsPutsGeometryAtTheBreak(t *testing.T) {
	out, ok := LayoutSections(firstPara+"\n"+brokenPara, []PageSetup{portrait, landscape})
	if !ok {
		t.Fatalf("layout refused a document with one break and two sections")
	}
	// The break carries the *second* section: the first one's geometry travels on
	// the document node, as it did before breaks existed.
	if !strings.Contains(out, `<div data-section-break="" data-page='{"w":297,"h":210,"ml":10,"mr":10,"mt":15,"mb":15}'></div>`) {
		t.Fatalf("break missing or carrying the wrong geometry:\n%s", out)
	}
	if strings.Contains(out, "page-break-before") {
		t.Fatalf("the marker declaration survived the hoist — the boundary is now in the body twice:\n%s", out)
	}
	// The paragraph itself stays where it was, with its text and its styling: it
	// is the first block of the new section, not part of the boundary.
	if !strings.Contains(out, "Landscape section body text.") || !strings.Contains(out, "margin-bottom: 0in") {
		t.Fatalf("the broken paragraph lost content or styling:\n%s", out)
	}
	if strings.Index(out, "data-section-break") > strings.Index(out, "Landscape section") {
		t.Fatalf("break landed after the paragraph it opens:\n%s", out)
	}
}

// Three sections, two breaks, in order — the case that fails silently if the
// geometries are matched to the wrong markers.
func TestLayoutSectionsKeepsOrder(t *testing.T) {
	html := firstPara + brokenPara + `<p style="page-break-before: always">Third.</p>`
	out, ok := LayoutSections(html, []PageSetup{portrait, landscape, a5})
	if !ok {
		t.Fatalf("layout refused two breaks and three sections")
	}
	second := strings.Index(out, `"w":297`)
	third := strings.Index(out, `"w":148`)
	if second < 0 || third < 0 {
		t.Fatalf("not both sections placed:\n%s", out)
	}
	if second > third {
		t.Fatalf("sections placed in the wrong order:\n%s", out)
	}
}

// The case the fallback exists for, and it is not hypothetical: a .docx with a
// manual page break inside its first section comes back from the sidecar with
// two markers and two sections, and nothing distinguishes the manual break from
// the section boundary. Laying out anyway would turn half the first section
// landscape.
func TestLayoutSectionsRefusesWhenBreaksOutnumberSections(t *testing.T) {
	html := firstPara +
		`<p style="page-break-before: always">Still the first section.</p>` +
		brokenPara
	out, ok := LayoutSections(html, []PageSetup{portrait, landscape})
	if ok {
		t.Fatalf("laid out a document whose breaks cannot be matched")
	}
	if out != html {
		t.Fatalf("refused but still rewrote the body")
	}
}

// The mirror case: a section boundary the conversion did not mark at all (a
// section starting on a table, say). Fewer markers than boundaries is just as
// unmatchable as more.
func TestLayoutSectionsRefusesWhenBreaksAreMissing(t *testing.T) {
	if _, ok := LayoutSections(firstPara, []PageSetup{portrait, landscape}); ok {
		t.Fatalf("laid out two sections over a document with no break in it")
	}
}

// A single-section document is laid out by definition and must come back byte
// for byte: it is the overwhelmingly common import, and a needless rewrite of
// its body would be a regression in every one of them. Its own manual page
// breaks are left alone — they are not section boundaries.
func TestLayoutSectionsLeavesSingleSectionAlone(t *testing.T) {
	html := firstPara + `<p style="page-break-before: always">Next page, same section.</p>`
	out, ok := LayoutSections(html, []PageSetup{portrait})
	if !ok {
		t.Fatalf("refused a single-section document")
	}
	if out != html {
		t.Fatalf("rewrote a single-section body:\n%s", out)
	}
}

// A page-break rule inside a <style> block is text, not a marker: the tag
// pattern cannot cross the `>` that ends <style>. Worth pinning, because a
// looser pattern would count the stylesheet as a boundary and push every
// section one place along.
func TestLayoutSectionsIgnoresStylesheetRules(t *testing.T) {
	html := `<style>p.brk { page-break-before: always; }</style>` + firstPara + brokenPara
	out, ok := LayoutSections(html, []PageSetup{portrait, landscape})
	if !ok {
		t.Fatalf("a stylesheet rule was counted as a section boundary")
	}
	if strings.Count(out, "data-section-break") != 1 {
		t.Fatalf("expected exactly one break:\n%s", out)
	}
}

func TestSectionsInDocumentOrder(t *testing.T) {
	for _, ext := range []string{"docx", ".docx", ".DOCX"} {
		if !SectionsInDocumentOrder(ext) {
			t.Errorf("%q: OOXML sections are in document order", ext)
		}
	}
	// ODF is refused on purpose — page layouts come back in declaration order,
	// so a positional match would place the bands wherever the file happened to
	// declare them.
	for _, ext := range []string{"odt", ".fodt", "doc", "html", ""} {
		if SectionsInDocumentOrder(ext) {
			t.Errorf("%q: order is not known to be document order", ext)
		}
	}
}
