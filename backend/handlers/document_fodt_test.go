package handlers

import (
	"encoding/base64"
	"encoding/xml"
	"regexp"
	"strings"
	"testing"
)

func fodt(n docNode) string {
	return renderDocFODT("", docNode{Type: "doc", Content: []docNode{n}}, nil)
}

// fodtAuto is the automatic-styles block on its own.
//
// Assertions about what a document generated have to look here rather than at
// the whole file: fodtCommonStyles declares a dozen named paragraph styles and a
// link style with a colour on it, so a naive strings.Contains over the output
// passes on the fixture instead of on the rendering under test.
func fodtAuto(out string) string {
	i := strings.Index(out, "<office:automatic-styles>")
	j := strings.Index(out, "</office:automatic-styles>")
	if i < 0 || j < 0 {
		return ""
	}
	return out[i:j]
}

// fodtNodeRendering is the expected markup for one of every node type the schema
// allows, keyed by type so the coverage assertion can compare it against
// allowedDocNodes directly — same shape as nodeRendering next door, and for the
// same reason.
var fodtNodeRendering = map[string]struct {
	node docNode
	want string
}{
	"doc":            {docNode{Type: "doc"}, "<office:text>"},
	"text":           {txt("привет"), "привет"},
	"paragraph":      {docNode{Type: "paragraph", Content: []docNode{txt("a")}}, `<text:p text:style-name="Standard">a</text:p>`},
	"heading":        {docNode{Type: "heading", Attrs: map[string]any{"level": float64(3)}, Content: []docNode{txt("h")}}, `<text:h text:style-name="Heading_20_3" text:outline-level="3">h</text:h>`},
	"bulletList":     {docNode{Type: "bulletList"}, `<text:list text:style-name="L_Bullet">`},
	"orderedList":    {docNode{Type: "orderedList", Attrs: map[string]any{"start": float64(4)}}, `text:start-value="4"`},
	"listItem":       {docNode{Type: "listItem", Content: []docNode{txt("i")}}, "<text:list-item><text:p text:style-name=\"Standard\">i</text:p>"},
	"taskList":       {docNode{Type: "taskList"}, `<text:list text:style-name="L_Task">`},
	"taskItem":       {docNode{Type: "taskItem", Attrs: map[string]any{"checked": true}, Content: []docNode{txt("d")}}, "☑ d"},
	"blockquote":     {docNode{Type: "blockquote", Content: []docNode{{Type: "paragraph", Content: []docNode{txt("q")}}}}, `<text:p text:style-name="Quotations">q</text:p>`},
	"codeBlock":      {docNode{Type: "codeBlock", Content: []docNode{txt("x < y")}}, `<text:p text:style-name="Preformatted_20_Text">x &lt; y</text:p>`},
	"horizontalRule": {docNode{Type: "horizontalRule"}, `<text:p text:style-name="Horizontal_20_Line"/>`},
	"hardBreak":      {docNode{Type: "hardBreak"}, "<text:line-break/>"},
	"image":          {docNode{Type: "image", Attrs: map[string]any{"src": "https://x/y.png", "alt": "к"}}, `<draw:image xlink:href="https://x/y.png"`},
	"table":          {docNode{Type: "table", Content: []docNode{{Type: "tableRow", Content: []docNode{{Type: "tableCell"}}}}}, `<table:table table:name="Table1"`},
	"tableRow":       {docNode{Type: "table", Content: []docNode{{Type: "tableRow", Content: []docNode{{Type: "tableCell"}}}}}, "<table:table-row>"},
	"tableHeader":    {docNode{Type: "table", Content: []docNode{{Type: "tableRow", Content: []docNode{{Type: "tableHeader", Content: []docNode{txt("H")}}}}}}, `<text:p text:style-name="Table_20_Heading">H</text:p>`},
	"tableCell":      {docNode{Type: "table", Content: []docNode{{Type: "tableRow", Content: []docNode{{Type: "tableCell", Attrs: map[string]any{"colspan": float64(2)}}}}}}, `table:number-columns-spanned="2"`},
	// Unlike the HTML route, the geometry really does travel — that is the point
	// of this renderer. See TestRenderDocFODTSectionsGetOwnPageLayout.
	"sectionBreak": {
		docNode{Type: "sectionBreak", Attrs: map[string]any{"page": map[string]any{
			"w": float64(297), "h": float64(210),
			"ml": float64(15), "mr": float64(15), "mt": float64(20), "mb": float64(20),
		}}},
		`style:print-orientation="landscape"`,
	},
	"pdfEmbed": {
		docNode{Type: "pdfEmbed", Attrs: map[string]any{"src": "/api/documents/asset?doc=1", "name": "смета.pdf"}},
		`<text:a xlink:type="simple" xlink:href="/api/documents/asset?doc=1" text:style-name="Internet_20_Link">PDF: смета.pdf</text:a>`,
	},
}

// TestRenderDocFODTCoversSchema guards the drift this renderer risks. A node
// type added to the allow-list that nothing here renders does not fail: the
// export simply comes back without the user's table, and no error is raised
// anywhere.
func TestRenderDocFODTCoversSchema(t *testing.T) {
	for nodeType := range allowedDocNodes {
		if _, ok := fodtNodeRendering[nodeType]; !ok {
			t.Errorf("node type %q is allowed by the schema but has no .fodt rendering (add it to renderFODTNode and to fodtNodeRendering)", nodeType)
		}
	}
	for nodeType := range fodtNodeRendering {
		if !allowedDocNodes[nodeType] {
			t.Errorf("fodtNodeRendering covers %q, which the schema no longer allows", nodeType)
		}
	}
}

func TestRenderDocFODTNodes(t *testing.T) {
	for nodeType, tc := range fodtNodeRendering {
		got := fodt(tc.node)
		if nodeType == "doc" {
			got = renderDocFODT("", tc.node, nil)
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s rendered as %q, want it to contain %q", nodeType, got, tc.want)
		}
	}
}

// TestRenderDocFODTIsWellFormedXML is the check the HTML renderer never needed.
// HTML tolerates a stray tag; ODF does not — LibreOffice refuses a malformed
// .fodt outright, which would turn one unusual document into a failed export
// with nothing to go on.
func TestRenderDocFODTIsWellFormedXML(t *testing.T) {
	for nodeType, tc := range fodtNodeRendering {
		doc := docNode{Type: "doc", Content: []docNode{tc.node}}
		if nodeType == "doc" {
			doc = tc.node
		}
		out := renderDocFODT("Заголовок & <тест>", doc, nil)
		d := xml.NewDecoder(strings.NewReader(out))
		for {
			_, err := d.Token()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("%s produced malformed XML: %v\n%s", nodeType, err, out)
			}
		}
	}
}

// TestRenderDocFODTSectionsGetOwnPageLayout is stage 2.3's reason to exist: the
// HTML export could carry the break but not the geometry, so a landscape section
// came back portrait.
func TestRenderDocFODTSectionsGetOwnPageLayout(t *testing.T) {
	root := docNode{Type: "doc", Attrs: map[string]any{"page": map[string]any{
		"w": float64(210), "h": float64(297),
		"ml": float64(20), "mr": float64(20), "mt": float64(20), "mb": float64(20),
	}}, Content: []docNode{
		{Type: "paragraph", Content: []docNode{txt("книжная")}},
		{Type: "sectionBreak", Attrs: map[string]any{"page": map[string]any{
			"w": float64(297), "h": float64(210),
			"ml": float64(15), "mr": float64(15), "mt": float64(10), "mb": float64(10),
		}}},
		{Type: "paragraph", Content: []docNode{txt("альбомная")}},
	}}
	out := renderDocFODT("", root, nil)

	for _, want := range []string{
		`<style:page-layout style:name="Mpm1">`,
		`fo:page-width="210mm" fo:page-height="297mm" style:print-orientation="portrait"`,
		`<style:page-layout style:name="Mpm2">`,
		`fo:page-width="297mm" fo:page-height="210mm" style:print-orientation="landscape"`,
		`<style:master-page style:name="Standard" style:page-layout-name="Mpm1"/>`,
		`<style:master-page style:name="Section_20_2" style:page-layout-name="Mpm2"/>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q\n%s", want, out)
		}
	}

	// The binding: the paragraph that opens the second section carries the master
	// page, which is both the break and the switch to the landscape layout.
	style := regexp.MustCompile(`<style:style style:name="(P\d+)" style:family="paragraph" style:parent-style-name="Standard" style:master-page-name="Section_20_2"`).FindStringSubmatch(out)
	if style == nil {
		t.Fatalf("no paragraph style binds the second master page\n%s", out)
	}
	if !strings.Contains(out, `<text:p text:style-name="`+style[1]+`">альбомная</text:p>`) {
		t.Errorf("the second section's first paragraph does not use %s\n%s", style[1], out)
	}
	if strings.Contains(out, `<text:p text:style-name="`+style[1]+`">книжная</text:p>`) {
		t.Errorf("the first section's paragraph must not carry a master page\n%s", out)
	}
}

// A section that opens with something which cannot carry a master page still has
// to break — the alternative is a landscape band that silently starts on the
// previous sheet.
func TestRenderDocFODTSectionOpeningWithListStillBreaks(t *testing.T) {
	root := docNode{Type: "doc", Content: []docNode{
		{Type: "paragraph", Content: []docNode{txt("до")}},
		{Type: "sectionBreak", Attrs: map[string]any{"page": map[string]any{
			"w": float64(297), "h": float64(210),
			"ml": float64(15), "mr": float64(15), "mt": float64(10), "mb": float64(10),
		}}},
		{Type: "bulletList", Content: []docNode{{Type: "listItem", Content: []docNode{txt("после")}}}},
	}}
	out := renderDocFODT("", root, nil)
	style := regexp.MustCompile(`<style:style style:name="(P\d+)"[^>]*style:master-page-name="Section_20_2"`).FindStringSubmatch(out)
	if style == nil {
		t.Fatalf("the section carries no master page at all\n%s", out)
	}
	if !strings.Contains(out, `<text:p text:style-name="`+style[1]+`"/>`) {
		t.Errorf("expected an empty anchor paragraph carrying %s before the list\n%s", style[1], out)
	}
	if i, j := strings.Index(out, `<text:p text:style-name="`+style[1]+`"/>`), strings.Index(out, "<text:list "); i < 0 || j < 0 || i > j {
		t.Errorf("the anchor paragraph must come before the list\n%s", out)
	}
}

// A break with no geometry of its own is a plain page break, not a reset to A4:
// resetting would reformat the rest of a document laid out on a custom sheet.
func TestRenderDocFODTSectionWithoutGeometryInheritsPrevious(t *testing.T) {
	root := docNode{Type: "doc", Attrs: map[string]any{"page": map[string]any{
		"w": float64(297), "h": float64(210),
		"ml": float64(10), "mr": float64(10), "mt": float64(10), "mb": float64(10),
	}}, Content: []docNode{
		{Type: "sectionBreak"},
	}}
	out := renderDocFODT("", root, nil)
	if n := strings.Count(out, `fo:page-width="297mm" fo:page-height="210mm"`); n != 2 {
		t.Errorf("expected both layouts to be 297x210, got %d such layouts\n%s", n, out)
	}
}

func TestRenderDocFODTDefaultPageGeometry(t *testing.T) {
	out := renderDocFODT("", docNode{Type: "doc"}, nil)
	// Must match what an untouched document exported as before this renderer
	// existed, or every archived document is silently reformatted on re-export.
	want := `fo:page-width="210mm" fo:page-height="297mm" style:print-orientation="portrait" fo:margin-top="20mm" fo:margin-right="20mm" fo:margin-bottom="20mm" fo:margin-left="20mm"`
	if !strings.Contains(out, want) {
		t.Errorf("default geometry missing %q\n%s", want, out)
	}
}

func TestRenderDocFODTMarks(t *testing.T) {
	cases := []struct {
		name string
		mark docMark
		want string
	}{
		{"bold", docMark{Type: "bold"}, `fo:font-weight="bold"`},
		{"italic", docMark{Type: "italic"}, `fo:font-style="italic"`},
		{"strike", docMark{Type: "strike"}, `style:text-line-through-style="solid"`},
		{"underline", docMark{Type: "underline"}, `style:text-underline-style="solid"`},
		{"code", docMark{Type: "code"}, `style:font-name="Liberation Mono"`},
		{"link", docMark{Type: "link", Attrs: map[string]any{"href": "https://x/"}}, `<text:a xlink:type="simple" xlink:href="https://x/"`},
		{"textStyle", docMark{Type: "textStyle", Attrs: map[string]any{"color": "#ff0000", "fontSize": "16px"}}, `fo:color="#ff0000"`},
	}
	for _, tc := range cases {
		out := fodt(docNode{Type: "paragraph", Content: []docNode{txt("t", tc.mark)}})
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s rendered as %q, want it to contain %q", tc.name, out, tc.want)
		}
	}
}

// px is not an ODF length. Passing it through worked in LibreOffice and would
// have failed in anything stricter, so it is converted at the CSS ratio.
func TestRenderDocFODTConvertsPixelFontSize(t *testing.T) {
	out := fodt(docNode{Type: "paragraph", Content: []docNode{
		txt("t", docMark{Type: "textStyle", Attrs: map[string]any{"fontSize": "16px"}}),
	}})
	if !strings.Contains(out, `fo:font-size="12pt"`) {
		t.Errorf("16px should become 12pt\n%s", out)
	}
}

func TestRenderDocFODTRefusesUnsafeLinksAndBadColours(t *testing.T) {
	out := fodt(docNode{Type: "paragraph", Content: []docNode{
		txt("t", docMark{Type: "link", Attrs: map[string]any{"href": "javascript:alert(1)"}}),
	}})
	if strings.Contains(out, "javascript:") {
		t.Errorf("unsafe href reached the output\n%s", out)
	}
	// A colour that is not #rrggbb is dropped rather than guessed at: fo:color is
	// a closed form, and a reader meeting "red" may refuse the file.
	out = fodt(docNode{Type: "paragraph", Content: []docNode{
		txt("t", docMark{Type: "textStyle", Attrs: map[string]any{"color": "red"}}),
	}})
	if strings.Contains(fodtAuto(out), "fo:color") {
		t.Errorf("a non-hex colour should be dropped\n%s", out)
	}
	if strings.Contains(out, "<text:span") {
		t.Errorf("a mark that produced no property should produce no span\n%s", out)
	}
}

func TestRenderDocFODTEscapesText(t *testing.T) {
	out := fodt(docNode{Type: "paragraph", Content: []docNode{txt(`a < b & "c"`)}})
	if !strings.Contains(out, `a &lt; b &amp; "c"`) {
		t.Errorf("character data not escaped as expected\n%s", out)
	}
}

// Runs of spaces and newlines are what an ODF reader collapses, and code is
// exactly the content that cannot afford it.
func TestRenderDocFODTKeepsCodeLayout(t *testing.T) {
	out := fodt(docNode{Type: "codeBlock", Content: []docNode{txt("if x:\n    return 1")}})
	if strings.Count(out, `<text:p text:style-name="Preformatted_20_Text">`) != 2 {
		t.Errorf("each code line should be its own paragraph\n%s", out)
	}
	if !strings.Contains(out, `<text:s text:c="3"/>`) {
		t.Errorf("leading indent should survive as text:s\n%s", out)
	}
}

func TestRenderDocFODTBookmarksInternalLinkTargets(t *testing.T) {
	root := docNode{Type: "doc", Content: []docNode{
		{Type: "paragraph", Content: []docNode{txt("к разделу", docMark{Type: "link", Attrs: map[string]any{"href": "#b1"}})}},
		{Type: "heading", Attrs: map[string]any{"id": "b1", "level": float64(2)}, Content: []docNode{txt("Раздел")}},
		{Type: "heading", Attrs: map[string]any{"id": "b2", "level": float64(2)}, Content: []docNode{txt("Другой")}},
	}}
	out := renderDocFODT("", root, nil)
	if !strings.Contains(out, `<text:bookmark text:name="b1"/>`) {
		t.Errorf("the linked heading has no bookmark\n%s", out)
	}
	if strings.Contains(out, `text:name="b2"`) {
		t.Errorf("only linked blocks should get a bookmark\n%s", out)
	}
}

func TestRenderDocFODTBlockStyle(t *testing.T) {
	out := fodt(docNode{Type: "paragraph", Attrs: map[string]any{
		"textAlign": "center", "lineHeight": "1.5", "indent": float64(2),
	}, Content: []docNode{txt("t")}})
	for _, want := range []string{`fo:text-align="center"`, `fo:line-height="150%"`, `fo:margin-left="48pt"`} {
		if !strings.Contains(out, want) {
			t.Errorf("block style missing %q\n%s", want, out)
		}
	}
}

// Identical blocks must share one automatic style: LibreOffice lists every one
// of them in the style picker of the exported file.
func TestRenderDocFODTReusesIdenticalStyles(t *testing.T) {
	para := docNode{Type: "paragraph", Attrs: map[string]any{"textAlign": "center"}, Content: []docNode{txt("t")}}
	out := renderDocFODT("", docNode{Type: "doc", Content: []docNode{para, para, para}}, nil)
	if n := strings.Count(fodtAuto(out), `style:family="paragraph"`); n != 1 {
		t.Errorf("expected one automatic paragraph style for three identical blocks, got %d\n%s", n, out)
	}
	if n := strings.Count(out, `<text:p text:style-name="P1">t</text:p>`); n != 3 {
		t.Errorf("all three paragraphs should use P1, got %d\n%s", n, out)
	}
}

func TestRenderDocFODTKeepsCellFillAndBorder(t *testing.T) {
	out := fodt(docNode{Type: "table", Content: []docNode{{Type: "tableRow", Content: []docNode{
		{Type: "tableCell", Attrs: map[string]any{"backgroundColor": "#eef", "borderColor": "#123456"}, Content: []docNode{txt("c")}},
	}}}})
	if !strings.Contains(out, `fo:background-color="#eeeeff"`) {
		t.Errorf("cell fill lost (three-digit hex should expand)\n%s", out)
	}
	if !strings.Contains(out, `fo:border="0.018cm solid #123456"`) {
		t.Errorf("cell border lost\n%s", out)
	}
}

// A browser lays out around a colspan; ODF wants the swallowed positions spelled
// out, and without them every cell after a merge shifts a column left.
func TestRenderDocFODTWritesCoveredCells(t *testing.T) {
	table := docNode{Type: "table", Content: []docNode{
		{Type: "tableRow", Content: []docNode{
			{Type: "tableCell", Attrs: map[string]any{"colspan": float64(2)}, Content: []docNode{txt("шапка")}},
			{Type: "tableCell", Content: []docNode{txt("c")}},
		}},
		{Type: "tableRow", Content: []docNode{
			{Type: "tableCell", Attrs: map[string]any{"rowspan": float64(2)}, Content: []docNode{txt("бок")}},
			{Type: "tableCell", Content: []docNode{txt("b")}},
			{Type: "tableCell", Content: []docNode{txt("c")}},
		}},
		{Type: "tableRow", Content: []docNode{
			{Type: "tableCell", Content: []docNode{txt("b")}},
			{Type: "tableCell", Content: []docNode{txt("c")}},
		}},
	}}
	out := fodt(table)
	if n := strings.Count(out, "<table:covered-table-cell/>"); n != 2 {
		t.Errorf("expected two covered cells (one for the colspan, one for the rowspan), got %d\n%s", n, out)
	}
	// The rowspan's covered cell belongs to the third row, at its first column.
	rows := strings.Split(out, "<table:table-row>")
	if len(rows) != 4 {
		t.Fatalf("expected three rows\n%s", out)
	}
	if !strings.HasPrefix(rows[3], "<table:covered-table-cell/>") {
		t.Errorf("the third row must open with the cell the rowspan covers\n%s", rows[3])
	}
	if n := strings.Count(out, "<table:table-column"); n != 3 {
		t.Errorf("expected three declared columns, got %d\n%s", n, out)
	}
}

func TestRenderDocFODTColumnWidths(t *testing.T) {
	out := fodt(docNode{Type: "table", Content: []docNode{{Type: "tableRow", Content: []docNode{
		{Type: "tableCell", Attrs: map[string]any{"colwidth": []any{float64(96)}}, Content: []docNode{txt("a")}},
		{Type: "tableCell", Attrs: map[string]any{"colwidth": []any{float64(192)}}, Content: []docNode{txt("b")}},
	}}}})
	if !strings.Contains(out, `style:column-width="2.5cm"`) || !strings.Contains(out, `style:column-width="5.1cm"`) {
		t.Errorf("column widths not carried over from colwidth\n%s", out)
	}
}

// A data: URI is not something LibreOffice's ODF import fetches: left in
// xlink:href the picture arrives as an empty box, so the bytes have to become
// the frame's own.
func TestRenderDocFODTEmbedsInlinedImageBytes(t *testing.T) {
	// 1x1 transparent PNG.
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	if _, err := base64.StdEncoding.DecodeString(png); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	out := renderDocFODT("", docNode{Type: "doc", Content: []docNode{
		{Type: "image", Attrs: map[string]any{"src": "data:image/png;base64," + png}},
	}}, nil)
	if !strings.Contains(out, "<office:binary-data>"+png+"</office:binary-data>") {
		t.Errorf("inlined image did not become binary data\n%s", out)
	}
	if strings.Contains(out, "xlink:href=\"data:") {
		t.Errorf("a data: URI must not be left in xlink:href\n%s", out)
	}
	// The size comes off the PNG header, because an ODF frame without one is at
	// the mercy of whatever the reader guesses.
	if !strings.Contains(out, `svg:width="0cm"`) && !strings.Contains(out, "svg:width=") {
		t.Errorf("frame carries no size\n%s", out)
	}
}

func TestRenderDocFODTUsesExplicitImageSize(t *testing.T) {
	out := fodt(docNode{Type: "image", Attrs: map[string]any{
		"src": "https://x/y.png", "width": float64(192), "height": float64(96),
	}})
	if !strings.Contains(out, `svg:width="5.1cm" svg:height="2.5cm"`) {
		t.Errorf("explicit width/height not converted\n%s", out)
	}
}

func TestRenderDocFODTResolveImageDropsEmpty(t *testing.T) {
	out := renderDocFODT("", docNode{Type: "doc", Content: []docNode{
		{Type: "image", Attrs: map[string]any{"src": "/x.png"}},
	}}, func(string) string { return "" })
	if strings.Contains(out, "draw:frame") {
		t.Errorf("resolveImage returning \"\" should drop the picture\n%s", out)
	}
}

func TestRenderDocFODTWritesTitle(t *testing.T) {
	out := renderDocFODT("Протокол", docNode{Type: "doc"}, nil)
	if !strings.Contains(out, `<text:h text:style-name="Title" text:outline-level="1">Протокол</text:h>`) {
		t.Errorf("title missing\n%s", out)
	}
	if !strings.Contains(out, "<dc:title>Протокол</dc:title>") {
		t.Errorf("document metadata missing the title\n%s", out)
	}
}
