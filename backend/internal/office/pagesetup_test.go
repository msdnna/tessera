package office

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// A4 and its landscape twin, in the twips the OOXML attributes carry. Written
// as the numbers Word actually writes rather than computed from millimetres, so
// the test fails if the conversion drifts instead of drifting with it.
const (
	twA4W  = "11906" // 210 mm
	twA4H  = "16838" // 297 mm
	tw20mm = "1134"
	tw15mm = "850"
)

func docxWith(body string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		panic(err)
	}
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>` + body + `</w:body></w:document>`
	if _, err := w.Write([]byte(doc)); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func sectPr(width, height, left, right string) string {
	return `<w:sectPr><w:pgSz w:w="` + width + `" w:h="` + height + `"/>` +
		`<w:pgMar w:top="` + tw20mm + `" w:right="` + right + `" w:bottom="` + tw20mm + `" w:left="` + left + `"/>` +
		`</w:sectPr>`
}

// The case the task was filed for: a document whose second section is landscape
// so that a wide table fits. Both sections have to be found — the first sits
// inside the paragraph that ends it, the last directly in the body — and the
// landscape one has to win, because it is the one the wide content was laid out
// for.
func TestDocxPageSetupsFindsBothSections(t *testing.T) {
	raw := docxWith(
		`<w:p><w:pPr>` + sectPr(twA4W, twA4H, tw20mm, tw20mm) + `</w:pPr></w:p>` +
			`<w:p><w:r><w:t>wide table lives here</w:t></w:r></w:p>` +
			sectPr(twA4H, twA4W, tw15mm, tw15mm))

	got, err := PageSetups("docx", raw)
	if err != nil {
		t.Fatalf("PageSetups: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d sections, want 2: %+v", len(got), got)
	}
	if got[0] != (PageSetup{Width: 210, Height: 297, Left: 20, Right: 20, Top: 20, Bottom: 20}) {
		t.Errorf("first section = %+v, want A4 portrait with 20 mm margins", got[0])
	}
	if got[0].Landscape() {
		t.Error("first section reports landscape")
	}
	if !got[1].Landscape() {
		t.Errorf("second section %+v does not report landscape", got[1])
	}
	if !Differ(got) {
		t.Error("Differ says the two sections agree")
	}

	widest, ok := Widest(got)
	if !ok {
		t.Fatal("Widest found nothing")
	}
	if !widest.Landscape() {
		t.Errorf("Widest picked %+v, want the landscape section", widest)
	}
	// The point of the whole exercise: the printable column grows from 170 mm to
	// 267 mm, which is what stops the imported table hanging off the sheet.
	if col := widest.Width - widest.Left - widest.Right; col < 260 {
		t.Errorf("printable column is %g mm, want the landscape one (~267)", col)
	}
}

// A single-section document must not be reported as "sections differ" — that
// flag drives a warning shown to the user, and a warning on every ordinary
// import is a warning nobody reads.
func TestDocxSingleSectionDoesNotDiffer(t *testing.T) {
	got, err := PageSetups(".docx", docxWith(sectPr(twA4W, twA4H, tw20mm, tw20mm)))
	if err != nil {
		t.Fatalf("PageSetups: %v", err)
	}
	if Differ(got) {
		t.Errorf("Differ true for one section: %+v", got)
	}
	if p, ok := Widest(got); !ok || p.Width != 210 {
		t.Errorf("Widest = %+v, %v", p, ok)
	}
}

// Two sections that happen to share a geometry are the common case in files
// that use a section break for a header change; they are not "different
// orientations" and must not warn.
func TestDocxIdenticalSectionsDoNotDiffer(t *testing.T) {
	one := sectPr(twA4W, twA4H, tw20mm, tw20mm)
	got, err := PageSetups("docx", docxWith(`<w:p><w:pPr>`+one+`</w:pPr></w:p>`+one))
	if err != nil {
		t.Fatalf("PageSetups: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d sections, want 2", len(got))
	}
	if Differ(got) {
		t.Error("Differ true for two identical sections")
	}
}

// A .docx without sectPr, and the formats that have no page setup to give, are
// ordinary imports — the caller falls back to the editor's default rather than
// refusing the file, so this has to be a sentinel and not a hard error.
func TestPageSetupsAbsent(t *testing.T) {
	cases := []struct {
		name string
		ext  string
		raw  []byte
	}{
		{"docx without sectPr", "docx", docxWith(`<w:p><w:r><w:t>hi</w:t></w:r></w:p>`)},
		{"docx without document.xml", "docx", func() []byte {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			_, _ = zw.Create("word/settings.xml")
			_ = zw.Close()
			return buf.Bytes()
		}()},
		{"rtf", "rtf", []byte(`{\rtf1}`)},
		{"txt", ".txt", []byte("plain")},
		{"html", "html", []byte("<p>hi</p>")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PageSetups(tc.ext, tc.raw); !errors.Is(err, ErrNoPageSetup) {
				t.Fatalf("err = %v, want ErrNoPageSetup", err)
			}
		})
	}
}

// A truncated upload must come back as an error naming the file, not as a panic
// inside archive/zip taking the request down with it.
func TestDocxCorruptArchive(t *testing.T) {
	raw := docxWith(sectPr(twA4W, twA4H, tw20mm, tw20mm))
	_, err := PageSetups("docx", raw[:len(raw)/2])
	if err == nil {
		t.Fatal("truncated archive parsed without error")
	}
	if errors.Is(err, ErrNoPageSetup) {
		t.Fatalf("truncated archive reported as 'no geometry': %v", err)
	}
	if !strings.Contains(err.Error(), "docx") {
		t.Errorf("error %q does not say what failed to parse", err)
	}
}

// Malformed XML inside an otherwise valid zip: same requirement, and a
// different code path (the streaming decoder rather than the zip reader).
func TestDocxMalformedXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte(`<w:document><w:body><w:sectPr>`))
	_ = zw.Close()

	if _, err := PageSetups("docx", buf.Bytes()); err == nil {
		t.Fatal("unterminated XML parsed without error")
	}
}

// Values a file can legally hold but a sheet cannot show. Each of these reaches
// the document body if it is not refused here, and from there the exported
// @page rule.
func TestDocxRejectsImpossibleGeometry(t *testing.T) {
	cases := map[string]string{
		"zero width":        sectPr("0", twA4H, tw20mm, tw20mm),
		"absurd width":      sectPr("9999999", twA4H, tw20mm, tw20mm),
		"margins meet":      sectPr(twA4W, twA4H, "6000", "6000"),
		"unparseable width": sectPr("21cm", twA4H, tw20mm, tw20mm),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := PageSetups("docx", docxWith(body)); !errors.Is(err, ErrNoPageSetup) {
				t.Fatalf("err = %v, want ErrNoPageSetup", err)
			}
		})
	}
}

// Flat ODF is the format stage 2 will export through (#2827), and it is already
// an accepted import — so its geometry is read by the same entry point.
func TestFodtPageSetup(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?>
<office:document xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
 xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0">
<office:automatic-styles>
<style:page-layout style:name="pm1"><style:page-layout-properties
 fo:page-width="29.7cm" fo:page-height="21cm"
 fo:margin-top="2cm" fo:margin-bottom="2cm" fo:margin-left="1.5cm" fo:margin-right="1.5cm"/>
</style:page-layout></office:automatic-styles></office:document>`)

	got, err := PageSetups("fodt", raw)
	if err != nil {
		t.Fatalf("PageSetups: %v", err)
	}
	want := PageSetup{Width: 297, Height: 210, Left: 15, Right: 15, Top: 20, Bottom: 20}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want [%+v]", got, want)
	}
	if !got[0].Landscape() {
		t.Error("29.7×21 cm does not report landscape")
	}
}

// The .odt path differs from .fodt only in the zip around it, but the member it
// has to find is styles.xml — page geometry is not in content.xml, which is the
// file one would reach for first.
func TestOdtPageSetupFromStylesXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	c, _ := zw.Create("content.xml")
	_, _ = c.Write([]byte(`<office:document xmlns:office="urn:x"><office:body/></office:document>`))
	s, _ := zw.Create("styles.xml")
	_, _ = s.Write([]byte(`<office:document-styles xmlns:office="urn:x"
 xmlns:style="urn:y" xmlns:fo="urn:z"><style:page-layout-properties
 fo:page-width="21cm" fo:page-height="29.7cm" fo:margin-top="20mm"
 fo:margin-bottom="20mm" fo:margin-left="20mm" fo:margin-right="20mm"/></office:document-styles>`))
	_ = zw.Close()

	got, err := PageSetups("odt", buf.Bytes())
	if err != nil {
		t.Fatalf("PageSetups: %v", err)
	}
	want := PageSetup{Width: 210, Height: 297, Left: 20, Right: 20, Top: 20, Bottom: 20}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want [%+v]", got, want)
	}
}

func TestOdfLengthUnits(t *testing.T) {
	cases := map[string]float64{
		"21cm":   210,
		"210mm":  210,
		"8.27in": 210.058,
		"595pt":  209.9027,
		"0.79in": 20.066,
		"":       0,
		"21":     0, // no unit — not a length
		"50%":    0, // legal XSL-FO, meaningless for a page
		"21em":   0, // font-relative, likewise
		"-5cm":   0, // negative margins do not survive to a sheet
	}
	for in, want := range cases {
		if got := odfLength(in); want == 0 && got != 0 {
			t.Errorf("odfLength(%q) = %g, want 0", in, got)
		} else if want != 0 && (got < want-0.01 || got > want+0.01) {
			t.Errorf("odfLength(%q) = %g, want ~%g", in, got, want)
		}
	}
}

// Rounding is not cosmetic: twips do not divide evenly into millimetres, so A4
// reads as 209.99999999999997 without it, and that number would travel into the
// stored body, the exported @page rule and every comparison between two of them.
func TestPageSetupRoundsToTenths(t *testing.T) {
	got, err := PageSetups("docx", docxWith(sectPr(twA4W, twA4H, tw20mm, tw20mm)))
	if err != nil {
		t.Fatalf("PageSetups: %v", err)
	}
	if got[0].Width != 210 || got[0].Height != 297 {
		t.Fatalf("A4 came out as %g×%g mm, want exactly 210×297", got[0].Width, got[0].Height)
	}
}
