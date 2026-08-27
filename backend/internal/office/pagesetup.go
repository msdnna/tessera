// Package office reads page geometry out of an uploaded office file.
//
// It exists because the geometry does not survive the conversion (#2821). The
// import route hands the file to the LibreOffice sidecar and gets HTML back,
// and LibreOffice writes an @page rule for the *first* section only — a
// document whose second section is landscape arrives as portrait HTML with a
// bare `page-break-before: always` where the section boundary was. A table laid
// out for a 297 mm wide page then lands on a 210 mm one and hangs off the sheet,
// which is the symptom the task was filed for.
//
// So the geometry is read from the source bytes here, in parallel with the
// conversion, and travels beside the HTML rather than inside it.
package office

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// PageSetup is one section's page geometry in millimetres — the unit the
// document body stores and the unit @page is written in on export. The source
// files speak twips (OOXML) and CSS lengths (ODF); both are converted here so
// that nothing downstream has to know which format the document came from.
type PageSetup struct {
	Width  float64 `json:"w"`
	Height float64 `json:"h"`
	Left   float64 `json:"ml"`
	Right  float64 `json:"mr"`
	Top    float64 `json:"mt"`
	Bottom float64 `json:"mb"`
}

// Bounds a page may fall in. Wide enough for a plotter sheet and narrow enough
// that a misparsed unit (an inch value read as millimetres, say) is refused
// rather than stored — the same limits the content validator enforces, because
// a value read here ends up in a document body.
const (
	MinSide = 50.0
	MaxSide = 2000.0
)

// ErrNoPageSetup means the file parsed but said nothing about its pages. It is
// not a failure of the import: a .txt has no geometry, and neither does a .docx
// written by a generator that leaves sectPr out. The caller falls back to the
// editor's default rather than refusing the file.
var ErrNoPageSetup = errors.New("office: file declares no page geometry")

// Landscape reports whether the page is wider than it is tall — the one
// question the import warning and the toolbar's orientation toggle both ask.
func (p PageSetup) Landscape() bool { return p.Width > p.Height }

// Valid rejects a geometry that cannot be a page: a side outside the bounds
// above, or margins that leave no column to print in. Both are reachable from a
// malformed file, and neither should reach a document body.
func (p PageSetup) Valid() bool {
	for _, side := range []float64{p.Width, p.Height} {
		if math.IsNaN(side) || side < MinSide || side > MaxSide {
			return false
		}
	}
	for _, m := range []float64{p.Left, p.Right, p.Top, p.Bottom} {
		if math.IsNaN(m) || m < 0 {
			return false
		}
	}
	return p.Left+p.Right < p.Width && p.Top+p.Bottom < p.Height
}

// round trims the geometry to a tenth of a millimetre. Twips do not divide
// evenly into millimetres, so an A4 page reads as 209.99999999999997 mm without
// this, and that number then travels into the document body, into the exported
// @page rule and into every test that compares two of them.
func (p PageSetup) round() PageSetup {
	r := func(v float64) float64 { return math.Round(v*10) / 10 }
	return PageSetup{r(p.Width), r(p.Height), r(p.Left), r(p.Right), r(p.Top), r(p.Bottom)}
}

// PageSetups reads every section's geometry from a source file, in document
// order.
//
// ext is the file extension with or without its dot. A format whose geometry we
// cannot read (.doc, .rtf, .txt, .html) returns ErrNoPageSetup rather than an
// error: those are legitimate imports, they simply carry no page setup for us,
// and the caller treats "unknown" and "unreadable" the same way anyway.
func PageSetups(ext string, raw []byte) ([]PageSetup, error) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "docx":
		return docxPageSetups(raw)
	case "odt":
		return odtPageSetups(raw)
	case "fodt":
		// Flat ODF is the same XML without the zip around it.
		return odfPageSetups(bytes.NewReader(raw))
	}
	return nil, ErrNoPageSetup
}

// Widest picks the section whose printable column is the widest.
//
// One document gets one page geometry in this stage, so a multi-section file
// has to be reduced to a single answer, and "widest" is the only choice that
// cannot make the situation worse: the landscape section is there precisely
// because something in it did not fit a portrait page. Taking the first section
// instead — the more obvious "the document's main geometry" reading — would put
// the wide table back on the narrow sheet, which is the bug being fixed.
//
// The comparison is on the printable column (page minus horizontal margins)
// rather than on the sheet, because that is what content actually gets.
func Widest(list []PageSetup) (PageSetup, bool) {
	best, found := PageSetup{}, false
	for _, p := range list {
		if !p.Valid() {
			continue
		}
		if !found || p.Width-p.Left-p.Right > best.Width-best.Left-best.Right {
			best, found = p, true
		}
	}
	return best, found
}

// Differ reports whether the sections disagree about geometry — the fact the
// import warning is built on. Told honestly, the user knows the sheet they got
// is a reduction of the file rather than a faithful copy of it.
func Differ(list []PageSetup) bool {
	var first PageSetup
	found := false
	for _, p := range list {
		if !p.Valid() {
			continue
		}
		if !found {
			first, found = p, true
			continue
		}
		if p != first {
			return true
		}
	}
	return false
}

// --- OOXML -----------------------------------------------------------------

// twipsPerMM converts OOXML's twentieth of a point to millimetres.
const twipsPerMM = 1440.0 / 25.4

type ooxmlSect struct {
	PgSz  *ooxmlPgSz  `xml:"pgSz"`
	PgMar *ooxmlPgMar `xml:"pgMar"`
}

// The attributes are namespaced (w:w, w:h); encoding/xml matches on the local
// name when the struct tag names no namespace, which is what is wanted here —
// a .docx written against a different WordprocessingML namespace URI still
// parses.
type ooxmlPgSz struct {
	W string `xml:"w,attr"`
	H string `xml:"h,attr"`
}

type ooxmlPgMar struct {
	Left   string `xml:"left,attr"`
	Right  string `xml:"right,attr"`
	Top    string `xml:"top,attr"`
	Bottom string `xml:"bottom,attr"`
}

func docxPageSetups(raw []byte) ([]PageSetup, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("office: not a readable .docx: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("office: cannot read word/document.xml: %w", err)
		}
		defer func() { _ = rc.Close() }()
		return ooxmlSects(rc)
	}
	return nil, ErrNoPageSetup
}

// ooxmlSects walks the document for <w:sectPr> elements.
//
// Streaming rather than unmarshalling the whole body: sectPr appears in two
// places — once per section as the last child of the paragraph that ends it,
// and once at the end of <w:body> for the final section — so there is no single
// struct shape that finds them all, and the body of a large document is
// megabytes we have no other use for.
func ooxmlSects(r io.Reader) ([]PageSetup, error) {
	dec := xml.NewDecoder(r)
	var out []PageSetup
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("office: malformed document.xml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "sectPr" {
			continue
		}
		var s ooxmlSect
		if err := dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("office: malformed sectPr: %w", err)
		}
		if s.PgSz == nil {
			continue
		}
		p := PageSetup{
			Width:  twips(s.PgSz.W),
			Height: twips(s.PgSz.H),
		}
		if s.PgMar != nil {
			p.Left = twips(s.PgMar.Left)
			p.Right = twips(s.PgMar.Right)
			p.Top = twips(s.PgMar.Top)
			p.Bottom = twips(s.PgMar.Bottom)
		}
		p = p.round()
		if p.Valid() {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoPageSetup
	}
	return out, nil
}

// twips converts a twip attribute to millimetres, returning 0 for anything
// unparseable. A negative margin is legal OOXML (Word uses it for content that
// prints into the margin) but is not something a sheet can show, so it is
// clamped rather than propagated.
func twips(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v / twipsPerMM
}

// --- ODF -------------------------------------------------------------------

type odfPageLayout struct {
	Width  string `xml:"page-width,attr"`
	Height string `xml:"page-height,attr"`
	Left   string `xml:"margin-left,attr"`
	Right  string `xml:"margin-right,attr"`
	Top    string `xml:"margin-top,attr"`
	Bottom string `xml:"margin-bottom,attr"`
}

func odtPageSetups(raw []byte) ([]PageSetup, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("office: not a readable .odt: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "styles.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("office: cannot read styles.xml: %w", err)
		}
		defer func() { _ = rc.Close() }()
		return odfPageSetups(rc)
	}
	return nil, ErrNoPageSetup
}

// odfPageSetups reads <style:page-layout-properties>. ODF keeps page geometry
// in named page layouts rather than inline with the text, so these come back in
// declaration order — which is not document order. That is good enough for what
// the caller does with them (widest wins, and "do they differ" is order-free)
// and is the reason the multi-section work of stage 2 will need the .fodt
// rewrite rather than an extension of this.
func odfPageSetups(r io.Reader) ([]PageSetup, error) {
	dec := xml.NewDecoder(r)
	var out []PageSetup
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("office: malformed ODF styles: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "page-layout-properties" {
			continue
		}
		var l odfPageLayout
		if err := dec.DecodeElement(&l, &se); err != nil {
			return nil, fmt.Errorf("office: malformed page layout: %w", err)
		}
		p := PageSetup{
			Width:  odfLength(l.Width),
			Height: odfLength(l.Height),
			Left:   odfLength(l.Left),
			Right:  odfLength(l.Right),
			Top:    odfLength(l.Top),
			Bottom: odfLength(l.Bottom),
		}.round()
		if p.Valid() {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoPageSetup
	}
	return out, nil
}

// odfUnits are the length units ODF actually uses for page geometry, in
// millimetres each. Percentages and font-relative units are not in the list on
// purpose: they are legal XSL-FO but meaningless for a page size, and treating
// an unknown unit as "no value" is what keeps a nonsense geometry from passing
// Valid() by accident.
var odfUnits = map[string]float64{
	"mm": 1,
	"cm": 10,
	"in": 25.4,
	"pt": 25.4 / 72,
	"pc": 25.4 / 6,
	"px": 25.4 / 96,
}

func odfLength(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 3 {
		return 0
	}
	scale, ok := odfUnits[s[len(s)-2:]]
	if !ok {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s[:len(s)-2]), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v * scale
}
