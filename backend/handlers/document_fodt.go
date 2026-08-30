package handlers

import (
	"encoding/base64"
	"fmt"
	"image"
	"regexp"
	"strconv"
	"strings"

	// Registered for their DecodeConfig only: an <image> node carries the size the
	// editor showed, but a picture dropped in by an import may not, and an ODF
	// frame without a size is at the mercy of whatever the reader guesses.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Rendering a stored document body to flat ODF (.fodt), the export route for
// every format that goes through the sidecar (#2849, stage 2.3 of #2827).
//
// The HTML renderer next door stays — format=html still uses it, and it needs no
// sidecar — but it cannot be the source of a .docx any more. HTML has exactly
// one page geometry: LibreOffice's importer drops named @page rules (measured in
// #2821), so a document with a landscape section came out of the export entirely
// portrait, with the break in the right place and the sheet the wrong shape.
//
// ODF has the missing concept. A page layout holds the geometry, a master page
// names a layout, and a paragraph whose style carries style:master-page-name
// both breaks the page and switches to that master — so N sections become N
// layouts and the k-th section's first paragraph points at the k-th master.
// Verified end to end against the sidecar during planning: a two-section .fodt
// converts to a .docx with two <w:sectPr> and w:orient="landscape" on the second.
//
// The duplication against renderDocHTML is the same bounded kind and gets the
// same guard: TestRenderDocFODTCoversSchema fails if a node type is added to the
// schema without a rendering here, because the way this breaks is silent — a
// user's table simply missing from their .docx.

// fodtNS is the namespace set of the root element. Only the prefixes this
// renderer actually writes are declared: a reader rejects an undeclared prefix,
// but an unused declaration is just noise in every exported file.
const fodtNS = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
	`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
	`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
	`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" ` +
	`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
	`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
	`xmlns:xlink="http://www.w3.org/1999/xlink" ` +
	`xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
	`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"`

// fodtCommonStyles is the named vocabulary the body refers to.
//
// Every visual decision docExportCSS makes for the HTML route is repeated here,
// because the two exports of one document have to look like the same document.
// Named styles rather than automatic ones on every block: a heading that is
// Heading_20_2 arrives in the .docx as a real "Heading 2" and stays in its table
// of contents, which a paragraph with a bold 14pt automatic style does not.
var fodtCommonStyles = `<office:font-face-decls>` +
	`<style:font-face style:name="Liberation Serif" svg:font-family="Liberation Serif" style:font-family-generic="roman"/>` +
	`<style:font-face style:name="Liberation Sans" svg:font-family="Liberation Sans" style:font-family-generic="swiss"/>` +
	`<style:font-face style:name="Liberation Mono" svg:font-family="Liberation Mono" style:font-family-generic="modern" style:font-pitch="fixed"/>` +
	`</office:font-face-decls>` + "\n" +
	`<office:styles>` +
	`<style:style style:name="Standard" style:family="paragraph">` +
	`<style:paragraph-properties fo:margin-bottom="0.212cm"/>` +
	`<style:text-properties style:font-name="Liberation Serif" fo:font-size="12pt"/></style:style>` +
	`<style:style style:name="Heading" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:margin-top="0.423cm" fo:margin-bottom="0.212cm" fo:keep-with-next="always"/>` +
	`<style:text-properties style:font-name="Liberation Sans" fo:font-weight="bold"/></style:style>` +
	fodtHeadingStyles +
	`<style:style style:name="Title" style:family="paragraph" style:parent-style-name="Heading">` +
	`<style:paragraph-properties fo:text-align="center"/>` +
	`<style:text-properties fo:font-size="22pt" fo:font-weight="bold"/></style:style>` +
	`<style:style style:name="Preformatted_20_Text" style:display-name="Preformatted Text" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:padding="0.211cm" fo:background-color="#f5f5f5"/>` +
	`<style:text-properties style:font-name="Liberation Mono" fo:font-size="10pt"/></style:style>` +
	`<style:style style:name="Quotations" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:margin-left="0.847cm" fo:padding-left="0.282cm" fo:border-left="0.071cm solid #cccccc"/></style:style>` +
	`<style:style style:name="Table_20_Contents" style:display-name="Table Contents" style:family="paragraph" style:parent-style-name="Standard"/>` +
	`<style:style style:name="Table_20_Heading" style:display-name="Table Heading" style:family="paragraph" style:parent-style-name="Table_20_Contents">` +
	`<style:paragraph-properties fo:text-align="center"/>` +
	`<style:text-properties fo:font-weight="bold"/></style:style>` +
	`<style:style style:name="Horizontal_20_Line" style:display-name="Horizontal Line" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:margin-top="0.212cm" fo:margin-bottom="0.212cm" fo:border-bottom="0.018cm solid #999999" fo:padding="0cm"/>` +
	`<style:text-properties fo:font-size="6pt"/></style:style>` +
	`<style:style style:name="Internet_20_Link" style:display-name="Internet Link" style:family="text">` +
	`<style:text-properties fo:color="#0000ee" style:text-underline-style="solid" style:text-underline-width="auto" style:text-underline-color="font-color"/></style:style>` +
	fodtListStyles +
	`</office:styles>`

// fodtHeadingStyles are the six outline levels. Sizes step down the way the
// browser's defaults do, so an exported heading is recognisable as the one the
// editor showed.
const fodtHeadingStyles = `<style:style style:name="Heading_20_1" style:display-name="Heading 1" style:family="paragraph" style:parent-style-name="Heading" style:default-outline-level="1">` +
	`<style:text-properties fo:font-size="18pt"/></style:style>` +
	`<style:style style:name="Heading_20_2" style:display-name="Heading 2" style:family="paragraph" style:parent-style-name="Heading" style:default-outline-level="2">` +
	`<style:text-properties fo:font-size="16pt"/></style:style>` +
	`<style:style style:name="Heading_20_3" style:display-name="Heading 3" style:family="paragraph" style:parent-style-name="Heading" style:default-outline-level="3">` +
	`<style:text-properties fo:font-size="14pt"/></style:style>` +
	`<style:style style:name="Heading_20_4" style:display-name="Heading 4" style:family="paragraph" style:parent-style-name="Heading" style:default-outline-level="4">` +
	`<style:text-properties fo:font-size="12pt"/></style:style>` +
	`<style:style style:name="Heading_20_5" style:display-name="Heading 5" style:family="paragraph" style:parent-style-name="Heading" style:default-outline-level="5">` +
	`<style:text-properties fo:font-size="11pt"/></style:style>` +
	`<style:style style:name="Heading_20_6" style:display-name="Heading 6" style:family="paragraph" style:parent-style-name="Heading" style:default-outline-level="6">` +
	`<style:text-properties fo:font-size="10pt"/></style:style>`

// fodtListStyles declares the three list shapes the schema has: a bulleted list,
// a numbered one, and the checklist, whose "bullet" is the ☐/☑ already written
// into the item's text and therefore must not get a second marker.
var fodtListStyles = buildFODTListStyles()

// fodtListLevels is how deep a list style declares levels. ProseMirror imposes
// no nesting limit of its own, but a level the style does not declare falls back
// to the reader's default rather than failing, and ten is past the point where a
// document is readable anyway.
const fodtListLevels = 10

func buildFODTListStyles() string {
	var b strings.Builder
	// The bullet cycles the way the HTML lists do in LibreOffice's own defaults.
	bullets := []string{"•", "◦", "▪"}
	b.WriteString(`<text:list-style style:name="L_Bullet">`)
	for i := 1; i <= fodtListLevels; i++ {
		fmt.Fprintf(&b, `<text:list-level-style-bullet text:level="%d" text:bullet-char="%s">%s</text:list-level-style-bullet>`,
			i, bullets[(i-1)%len(bullets)], fodtListLevelProps(i))
	}
	b.WriteString(`</text:list-style>`)
	b.WriteString(`<text:list-style style:name="L_Number">`)
	for i := 1; i <= fodtListLevels; i++ {
		fmt.Fprintf(&b, `<text:list-level-style-number text:level="%d" style:num-suffix="." style:num-format="1">%s</text:list-level-style-number>`,
			i, fodtListLevelProps(i))
	}
	b.WriteString(`</text:list-style>`)
	// An empty num-format is the ODF way to say "a list with no marker" — the
	// items still indent and nest, which is what a checklist wants.
	b.WriteString(`<text:list-style style:name="L_Task">`)
	for i := 1; i <= fodtListLevels; i++ {
		fmt.Fprintf(&b, `<text:list-level-style-number text:level="%d" style:num-format="">%s</text:list-level-style-number>`,
			i, fodtListLevelProps(i))
	}
	b.WriteString(`</text:list-style>`)
	return b.String()
}

// fodtListLevelProps indents each level by a further 0.635 cm and hangs the
// marker in that space, which is the label-alignment layout LibreOffice writes
// for its own lists.
func fodtListLevelProps(level int) string {
	margin := fmt.Sprintf("%.3fcm", 0.635*float64(level+1))
	return `<style:list-level-properties text:list-level-position-and-space-mode="label-alignment">` +
		`<style:list-level-label-alignment text:label-followed-by="listtab" fo:text-indent="-0.635cm" fo:margin-left="` + margin + `"/>` +
		`</style:list-level-properties>`
}

// fodtStyle is one automatic style: a family, an optional parent, and the
// properties element that goes inside it.
type fodtStyle struct {
	name   string
	family string
	parent string
	props  string
	// extra goes on the <style:style> element itself rather than in the
	// properties child — style:master-page-name is the one that matters, and it
	// is an attribute of the style, not of its paragraph properties.
	extra string
}

// fodtStyles hands out automatic style names, reusing one for identical
// properties.
//
// Deduplicating is not an optimisation: a document where every paragraph is
// centred would otherwise declare a hundred identical styles, and LibreOffice
// shows each of them in the style picker of the exported file.
type fodtStyles struct {
	byKey map[string]string
	list  []fodtStyle
	seq   map[string]int
}

func newFODTStyles() *fodtStyles {
	return &fodtStyles{byKey: map[string]string{}, seq: map[string]int{}}
}

// fodtStylePrefix is the name prefix per family, following the convention
// LibreOffice's own output uses so an exported file reads like a native one.
var fodtStylePrefix = map[string]string{
	"paragraph":    "P",
	"text":         "T",
	"table":        "Ta",
	"table-column": "co",
	"table-cell":   "ce",
	"graphic":      "fr",
}

// fodtStyleProps is the properties element each family carries.
var fodtStyleProps = map[string]string{
	"paragraph":    "style:paragraph-properties",
	"text":         "style:text-properties",
	"table":        "style:table-properties",
	"table-column": "style:table-column-properties",
	"table-cell":   "style:table-cell-properties",
	"graphic":      "style:graphic-properties",
}

func (s *fodtStyles) add(family, parent, props, extra string) string {
	key := family + "|" + parent + "|" + props + "|" + extra
	if name, ok := s.byKey[key]; ok {
		return name
	}
	prefix := fodtStylePrefix[family]
	s.seq[prefix]++
	name := fmt.Sprintf("%s%d", prefix, s.seq[prefix])
	s.byKey[key] = name
	s.list = append(s.list, fodtStyle{name: name, family: family, parent: parent, props: props, extra: extra})
	return name
}

func (s *fodtStyles) render(b *strings.Builder) {
	for _, st := range s.list {
		b.WriteString(`<style:style style:name="` + st.name + `" style:family="` + st.family + `"`)
		if st.parent != "" {
			b.WriteString(` style:parent-style-name="` + st.parent + `"`)
		}
		b.WriteString(st.extra)
		if st.props == "" {
			b.WriteString("/>")
			continue
		}
		b.WriteString(">" + "<" + fodtStyleProps[st.family] + " " + st.props + "/>")
		b.WriteString("</style:style>")
	}
}

// fodtCtx is the walk's state. Beyond what the HTML renderer needs it tracks
// where the walk is (a frame has to sit inside a paragraph; a quoted paragraph
// takes a different parent style) and which master page the next block owes.
type fodtCtx struct {
	resolveImage func(string) string
	anchors      map[string]bool
	styles       *fodtStyles
	autoLists    []string

	// pendingMaster is the master page the next block must open, set by a
	// sectionBreak and cleared by whoever carries it. Empty means "continue on
	// the current page", which is every block except the first of a section.
	pendingMaster string
	sectionIdx    int

	inPara    bool
	inQuote   bool
	listDepth int
	frameSeq  int
	tableSeq  int
	listSeq   int
}

// takeMaster hands the pending master page to a block that can carry it, and
// clears it so the next block does not break the page a second time.
func (ctx *fodtCtx) takeMaster() string {
	m := ctx.pendingMaster
	ctx.pendingMaster = ""
	return m
}

// masterAttr is takeMaster as the attribute it becomes.
func (ctx *fodtCtx) masterAttr() string {
	if m := ctx.takeMaster(); m != "" {
		return ` style:master-page-name="` + m + `"`
	}
	return ""
}

// flushMaster opens the section with an empty paragraph, for the blocks that
// cannot carry the master page themselves.
//
// A list, a rule or a picture has no paragraph of its own to hang
// style:master-page-name on — a text:list is not a paragraph, and hanging it on
// the paragraph *inside* the first list item is not the documented binding and
// was not measured. An empty paragraph at the top of the section is a visible
// blank line, which is the honest cost of a break the reader will definitely
// honour; the common case (a section that starts with text or a heading) does
// not pay it.
func (ctx *fodtCtx) flushMaster(b *strings.Builder) {
	if ctx.pendingMaster == "" {
		return
	}
	name := ctx.styles.add("paragraph", "Standard", "", ctx.masterAttr())
	b.WriteString(`<text:p text:style-name="` + name + `"/>` + "\n")
}

// renderDocFODT turns a validated document body into a flat ODF file.
//
// resolveImage matches renderDocHTML: it is handed an <img src> and returns what
// to use in its place, which is how inlineDocAsset turns a signed asset URL into
// bytes this renderer can embed. Returning "" drops the image.
func renderDocFODT(title string, root docNode, resolveImage func(string) string) string {
	ctx := &fodtCtx{
		resolveImage: resolveImage,
		anchors:      internalLinkTargets(root),
		styles:       newFODTStyles(),
	}

	var body strings.Builder
	if strings.TrimSpace(title) != "" {
		body.WriteString(`<text:h text:style-name="Title" text:outline-level="1">` + fodtText(title) + "</text:h>\n")
	}
	renderFODTNodes(&body, root.Content, ctx)

	pages := docSectionPages(root)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<office:document ` + fodtNS + ` office:version="1.3" office:mimetype="application/vnd.oasis.opendocument.text">` + "\n")
	b.WriteString(`<office:meta><dc:title>` + fodtText(title) + `</dc:title></office:meta>` + "\n")
	b.WriteString(fodtCommonStyles + "\n")
	b.WriteString(`<office:automatic-styles>`)
	for i, page := range pages {
		b.WriteString(fodtPageLayout(fodtLayoutName(i), page))
	}
	for _, raw := range ctx.autoLists {
		b.WriteString(raw)
	}
	ctx.styles.render(&b)
	b.WriteString(`</office:automatic-styles>` + "\n")
	b.WriteString(`<office:master-styles>`)
	for i := range pages {
		b.WriteString(`<style:master-page style:name="` + fodtMasterName(i) + `" style:page-layout-name="` + fodtLayoutName(i) + `"/>`)
	}
	b.WriteString(`</office:master-styles>` + "\n")
	b.WriteString(`<office:body><office:text>` + "\n")
	b.WriteString(body.String())
	b.WriteString(`</office:text></office:body></office:document>` + "\n")
	return b.String()
}

// fodtMasterName names the master page of section i. The first is "Standard"
// because that is the master the Standard paragraph style already points at —
// naming it anything else would leave the document's default page unused and the
// first section would open with a break it did not ask for.
func fodtMasterName(i int) string {
	if i == 0 {
		return "Standard"
	}
	return fmt.Sprintf("Section_20_%d", i+1)
}

func fodtLayoutName(i int) string { return fmt.Sprintf("Mpm%d", i+1) }

// docSectionPages is the document's geometry, one entry per section, in document
// order: the doc node's own page followed by each sectionBreak's.
//
// A break with no usable geometry inherits the section before it rather than
// falling back to A4. The break is then a plain page break, which is what a user
// who inserted one without opening the page dialog meant — resetting to the
// default would silently reformat the rest of a document laid out on a custom
// sheet.
func docSectionPages(root docNode) []map[string]float64 {
	pages := []map[string]float64{docPageOf(root.Attrs, defaultDocPage)}
	var walk func([]docNode)
	walk = func(nodes []docNode) {
		for _, n := range nodes {
			if n.Type == "sectionBreak" {
				pages = append(pages, docPageOf(n.Attrs, pages[len(pages)-1]))
			}
			walk(n.Content)
		}
	}
	walk(root.Content)
	return pages
}

// docPageOf reads a validated page geometry off a node, falling back to what the
// caller says the surrounding document uses.
//
// Re-validated here for the same reason docPageCSS does it: a stored body can
// predate a validation rule, and these numbers become a page size.
func docPageOf(attrs map[string]any, fallback map[string]float64) map[string]float64 {
	given, ok := attrs["page"].(map[string]any)
	if !ok || checkDocPage(given) != nil {
		return fallback
	}
	page := map[string]float64{}
	for k, v := range given {
		page[k], _ = v.(float64)
	}
	return page
}

// fodtPageLayout writes one section's sheet and margins.
//
// style:print-orientation is derived rather than stored: the document model has
// no orientation flag, only a width and a height, and the sidecar needs the flag
// to put w:orient="landscape" in the .docx. Deriving it keeps the two from
// disagreeing — a landscape flag on a portrait sheet is a file that prints wrong.
func fodtPageLayout(name string, page map[string]float64) string {
	orientation := "portrait"
	if page["w"] > page["h"] {
		orientation = "landscape"
	}
	return fmt.Sprintf(`<style:page-layout style:name="%s"><style:page-layout-properties `+
		`fo:page-width="%smm" fo:page-height="%smm" style:print-orientation="%s" `+
		`fo:margin-top="%smm" fo:margin-right="%smm" fo:margin-bottom="%smm" fo:margin-left="%smm" `+
		`style:writing-mode="lr-tb"/></style:page-layout>`,
		name, mmValue(page["w"]), mmValue(page["h"]), orientation,
		mmValue(page["mt"]), mmValue(page["mr"]), mmValue(page["mb"]), mmValue(page["ml"]))
}

func renderFODTNodes(b *strings.Builder, nodes []docNode, ctx *fodtCtx) {
	for _, n := range nodes {
		renderFODTNode(b, n, ctx)
	}
}

func renderFODTNode(b *strings.Builder, n docNode, ctx *fodtCtx) {
	switch n.Type {
	case "text":
		b.WriteString(renderFODTText(n, ctx))
	case "paragraph":
		writeFODTPara(b, n, ctx, ctx.paraParent())
	case "heading":
		level := attrInt(n.Attrs, "level", 1)
		if level < 1 || level > 6 {
			level = 1
		}
		style := fodtParaStyle(ctx, fmt.Sprintf("Heading_20_%d", level), n.Attrs)
		fmt.Fprintf(b, `<text:h text:style-name="%s" text:outline-level="%d">`, style, level)
		writeFODTInline(b, n, ctx)
		b.WriteString("</text:h>\n")
	case "bulletList":
		renderFODTList(b, n, ctx, "L_Bullet")
	case "orderedList":
		renderFODTList(b, n, ctx, ctx.numberListStyle(attrInt(n.Attrs, "start", 1)))
	case "taskList":
		renderFODTList(b, n, ctx, "L_Task")
	case "listItem":
		b.WriteString("<text:list-item>")
		renderFODTItemBody(b, n, ctx, "")
		b.WriteString("</text:list-item>\n")
	case "taskItem":
		// The box travels as a character for the same reason it does in the HTML
		// export: an ODF form control round-trips into .docx as an empty field and
		// loses the one bit of state the item carries.
		box := "☐ "
		if checked, ok := n.Attrs["checked"].(bool); ok && checked {
			box = "☑ "
		}
		b.WriteString("<text:list-item>")
		renderFODTItemBody(b, n, ctx, box)
		b.WriteString("</text:list-item>\n")
	case "blockquote":
		was := ctx.inQuote
		ctx.inQuote = true
		renderFODTNodes(b, n.Content, ctx)
		ctx.inQuote = was
	case "codeBlock":
		renderFODTCode(b, n, ctx)
	case "horizontalRule":
		ctx.flushMaster(b)
		b.WriteString(`<text:p text:style-name="Horizontal_20_Line"/>` + "\n")
	case "sectionBreak":
		// The break itself writes nothing: in ODF a section boundary *is* the
		// master page the next block opens with, so all this node does is say which
		// one that is. The geometry was collected up front by docSectionPages,
		// which is why the counter here and the index there have to walk the tree
		// the same way.
		ctx.sectionIdx++
		ctx.pendingMaster = fodtMasterName(ctx.sectionIdx)
	case "hardBreak":
		b.WriteString("<text:line-break/>")
	case "image":
		renderFODTImage(b, n, ctx)
	case "pdfEmbed":
		renderFODTPdf(b, n, ctx)
	case "table":
		renderFODTTable(b, n, ctx)
	case "tableRow", "tableHeader", "tableCell":
		// Reached only for a stray cell outside a table, which the schema does not
		// produce. Rendering the children keeps the text rather than the wrapper.
		renderFODTNodes(b, n.Content, ctx)
	default:
		// Unreachable for stored content — validateDocContent refuses anything
		// outside the allow-list and TestRenderDocFODTCoversSchema keeps this switch
		// in step with it. Same fallback as the HTML renderer: lose the wrapper, not
		// the user's text.
		renderFODTNodes(b, n.Content, ctx)
	}
}

// paraParent is the named style a plain paragraph inherits from where the walk
// currently is.
func (ctx *fodtCtx) paraParent() string {
	if ctx.inQuote {
		return "Quotations"
	}
	return "Standard"
}

// writeFODTPara writes one <text:p> with its bookmark, style and content.
func writeFODTPara(b *strings.Builder, n docNode, ctx *fodtCtx, parent string) {
	b.WriteString(`<text:p text:style-name="` + fodtParaStyle(ctx, parent, n.Attrs) + `">`)
	writeFODTInline(b, n, ctx)
	b.WriteString("</text:p>\n")
}

// writeFODTInline writes a block's anchor and children, with inPara set so a
// picture inside knows it already has a paragraph around it.
func writeFODTInline(b *strings.Builder, n docNode, ctx *fodtCtx) {
	b.WriteString(fodtAnchor(n, ctx))
	was := ctx.inPara
	ctx.inPara = true
	renderFODTNodes(b, n.Content, ctx)
	ctx.inPara = was
}

// fodtParaStyle resolves a paragraph to a style name: the named parent when the
// block asks for nothing extra, otherwise an automatic style deriving from it.
func fodtParaStyle(ctx *fodtCtx, parent string, attrs map[string]any) string {
	props := fodtParaProps(attrs)
	master := ctx.masterAttr()
	if props == "" && master == "" {
		return parent
	}
	return ctx.styles.add("paragraph", parent, props, master)
}

// fodtParaProps maps the BlockStyle attributes onto ODF paragraph properties.
//
// The cell-only attributes (backgroundColor, borderColor) are deliberately not
// here: in ODF a cell's fill belongs to the table-cell style, and putting it on
// the paragraph would paint the text's box instead of the cell — a visibly
// different result from the HTML export, where one rule covers both.
func fodtParaProps(attrs map[string]any) string {
	if len(attrs) == 0 {
		return ""
	}
	var out []string
	if s, ok := attrs["textAlign"].(string); ok {
		if v := fodtAlign(s); v != "" {
			out = append(out, `fo:text-align="`+v+`"`)
		}
	}
	if s, ok := attrs["lineHeight"].(string); ok && s != "" {
		if v := fodtLineHeight(s); v != "" {
			out = append(out, `fo:line-height="`+v+`"`)
		}
	}
	if indent := attrInt(attrs, "indent", 0); indent > 0 {
		// The same 24pt a step buys on the HTML route, so a document indented in
		// the editor lands in the same place in either export.
		out = append(out, fmt.Sprintf(`fo:margin-left="%dpt"`, indent*24))
	}
	return strings.Join(out, " ")
}

// fodtAlign translates the CSS keyword the editor stores into the XSL-FO one ODF
// uses. An unknown value is dropped rather than passed through: fo:text-align is
// a closed vocabulary, and a reader that meets a word outside it may reject the
// file rather than ignore the attribute.
func fodtAlign(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "left", "start":
		return "start"
	case "right", "end":
		return "end"
	case "center":
		return "center"
	case "justify":
		return "justify"
	}
	return ""
}

// fodtLineHeight turns the CSS value into one fo:line-height accepts. A bare
// ratio ("1.5"), which is what the editor writes, is a percentage there.
func fodtLineHeight(s string) string {
	s = strings.TrimSpace(cssValue(s))
	if s == "" {
		return ""
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(int(v*100+0.5)) + "%"
	}
	if fodtLengthRe.MatchString(s) {
		return s
	}
	return ""
}

// fodtLengthRe is a length ODF will accept. Values reaching the style attributes
// come from stored content, and XML escaping alone would not stop "1cm" from
// arriving as something the reader refuses to parse.
var fodtLengthRe = regexp.MustCompile(`^\d+(\.\d+)?(cm|mm|in|pt|pc|px|%)$`)

// fodtColorRe is the only colour form ODF takes: #rrggbb. The editor stores
// exactly that, so anything else (a CSS name, rgb(), an 8-digit hex with alpha)
// is dropped rather than guessed at.
var fodtColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// fodtColor normalises a stored colour, expanding the three-digit form the
// editor's colour picker can produce.
func fodtColor(s string) string {
	s = strings.TrimSpace(s)
	if regexp.MustCompile(`^#[0-9a-fA-F]{3}$`).MatchString(s) {
		s = "#" + string([]byte{s[1], s[1], s[2], s[2], s[3], s[3]})
	}
	if !fodtColorRe.MatchString(s) {
		return ""
	}
	return strings.ToLower(s)
}

// fodtFontSize converts the size the editor stores to one ODF understands. px is
// not an ODF length in practice — LibreOffice reads it, but a stricter reader
// need not — so it is converted at the CSS ratio of 96 px to 72 pt.
func fodtFontSize(s string) string {
	s = strings.TrimSpace(cssValue(s))
	if strings.HasSuffix(s, "px") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
		if err != nil || v <= 0 {
			return ""
		}
		return mmValue(v*0.75) + "pt"
	}
	if fodtLengthRe.MatchString(s) {
		return s
	}
	return ""
}

// fodtAnchor writes the bookmark an internal link jumps to.
//
// Only for the blocks actually linked to, same as the HTML route: a bookmark on
// every paragraph would show up as hundreds of entries in LibreOffice's
// navigator of the exported file.
func fodtAnchor(n docNode, ctx *fodtCtx) string {
	id, _ := n.Attrs["id"].(string)
	if id == "" || !ctx.anchors[id] {
		return ""
	}
	return `<text:bookmark text:name="` + fodtAttr(id) + `"/>`
}

// renderFODTList writes a text:list, flushing any owed section break first.
func renderFODTList(b *strings.Builder, n docNode, ctx *fodtCtx, style string) {
	ctx.flushMaster(b)
	ctx.listDepth++
	b.WriteString(`<text:list text:style-name="` + style + `">` + "\n")
	renderFODTNodes(b, n.Content, ctx)
	b.WriteString("</text:list>\n")
	ctx.listDepth--
}

// numberListStyle is the shared numbered style, or a private one when the list
// does not start at 1.
//
// ODF puts the start value in the list *style*, not on the list, so "continue
// from 4" cannot be expressed by reusing L_Number — the second such list would
// silently renumber the first.
func (ctx *fodtCtx) numberListStyle(start int) string {
	if start <= 1 {
		return "L_Number"
	}
	ctx.listSeq++
	name := fmt.Sprintf("L_Start%d", ctx.listSeq)
	var b strings.Builder
	b.WriteString(`<text:list-style style:name="` + name + `">`)
	for i := 1; i <= fodtListLevels; i++ {
		startAttr := ""
		if i == 1 {
			startAttr = fmt.Sprintf(` text:start-value="%d"`, start)
		}
		fmt.Fprintf(&b, `<text:list-level-style-number text:level="%d"%s style:num-suffix="." style:num-format="1">%s</text:list-level-style-number>`,
			i, startAttr, fodtListLevelProps(i))
	}
	b.WriteString(`</text:list-style>`)
	ctx.autoLists = append(ctx.autoLists, b.String())
	return name
}

// renderFODTItemBody writes a list item's content.
//
// A list item in ODF holds blocks, not text, and the tree does not guarantee
// one: a bare text child (which an import can produce) has to be given a
// paragraph or the file is invalid. prefix is the checklist box, which belongs
// inside the item's first paragraph rather than before it.
func renderFODTItemBody(b *strings.Builder, n docNode, ctx *fodtCtx, prefix string) {
	blocks := false
	for _, c := range n.Content {
		if c.Type != "text" && c.Type != "hardBreak" && c.Type != "image" {
			blocks = true
			break
		}
	}
	if !blocks {
		b.WriteString(`<text:p text:style-name="` + ctx.paraParent() + `">` + fodtText(prefix))
		writeFODTInline(b, n, ctx)
		b.WriteString("</text:p>\n")
		return
	}
	b.WriteString(fodtAnchor(n, ctx))
	for i, c := range n.Content {
		if i == 0 && prefix != "" && c.Type == "paragraph" {
			// The box goes inside the item's own first paragraph so it sits on the
			// same line as the text, not on a line of its own above it.
			b.WriteString(`<text:p text:style-name="` + fodtParaStyle(ctx, ctx.paraParent(), c.Attrs) + `">` + fodtText(prefix))
			writeFODTInline(b, c, ctx)
			b.WriteString("</text:p>\n")
			prefix = ""
			continue
		}
		renderFODTNode(b, c, ctx)
	}
	if prefix != "" {
		b.WriteString(`<text:p text:style-name="` + ctx.paraParent() + `">` + fodtText(prefix) + "</text:p>\n")
	}
}

// renderFODTCode writes a code block as one paragraph per line.
//
// ODF has no <pre>: a paragraph is the unit of layout, and a newline inside one
// is whitespace the reader collapses. Splitting is therefore not a formatting
// choice but the only way the block's line structure survives.
func renderFODTCode(b *strings.Builder, n docNode, ctx *fodtCtx) {
	var code strings.Builder
	for _, c := range n.Content {
		if c.Type == "text" && c.Text != nil {
			code.WriteString(*c.Text)
		}
	}
	master := ctx.masterAttr()
	style := "Preformatted_20_Text"
	if master != "" {
		style = ctx.styles.add("paragraph", style, "", master)
	}
	anchor := fodtAnchor(n, ctx)
	for _, line := range strings.Split(strings.TrimSuffix(code.String(), "\n"), "\n") {
		b.WriteString(`<text:p text:style-name="` + style + `">` + anchor + fodtText(line) + "</text:p>\n")
		anchor = ""
		style = "Preformatted_20_Text"
	}
}

// renderFODTText applies a text node's marks.
//
// One span carrying every mark's properties, rather than the nested elements the
// HTML renderer writes: ProseMirror marks are a flat set on the node, so the
// nesting carries no information, and a chain of five spans is what makes an
// exported paragraph unreadable in LibreOffice's style panel. The link is the
// exception — text:a is an element, not a property.
func renderFODTText(n docNode, ctx *fodtCtx) string {
	if n.Text == nil {
		return ""
	}
	out := fodtText(*n.Text)
	var props []string
	href := ""
	linked := false
	for _, m := range n.Marks {
		switch m.Type {
		case "bold":
			props = append(props, `fo:font-weight="bold"`, `style:font-weight-asian="bold"`)
		case "italic":
			props = append(props, `fo:font-style="italic"`, `style:font-style-asian="italic"`)
		case "strike":
			props = append(props, `style:text-line-through-style="solid"`, `style:text-line-through-type="single"`)
		case "underline":
			props = append(props, `style:text-underline-style="solid"`, `style:text-underline-width="auto"`, `style:text-underline-color="font-color"`)
		case "code":
			props = append(props, `style:font-name="Liberation Mono"`, `fo:font-size="10pt"`, `fo:background-color="#f5f5f5"`)
		case "link":
			h, _ := m.Attrs["href"].(string)
			// safeDocHref refused javascript:/data: at write time; checked again here
			// because the renderer must not depend on when the row was written.
			if h == "" || !safeDocHref(h) {
				break
			}
			href, linked = h, true
		case "textStyle":
			props = append(props, fodtTextStyleProps(m.Attrs)...)
		}
	}
	if len(props) > 0 {
		out = `<text:span text:style-name="` + ctx.styles.add("text", "", strings.Join(props, " "), "") + `">` + out + "</text:span>"
	}
	if linked {
		out = `<text:a xlink:type="simple" xlink:href="` + fodtAttr(href) + `" text:style-name="Internet_20_Link">` + out + "</text:a>"
	}
	return out
}

func renderFODTImage(b *strings.Builder, n docNode, ctx *fodtCtx) {
	src, _ := n.Attrs["src"].(string)
	if ctx.resolveImage != nil {
		src = ctx.resolveImage(src)
	}
	if src == "" {
		return
	}
	if !ctx.inPara {
		ctx.flushMaster(b)
		b.WriteString(`<text:p text:style-name="` + ctx.paraParent() + `">`)
	}
	ctx.frameSeq++
	frame := ctx.styles.add("graphic", "", `style:vertical-pos="top" style:vertical-rel="baseline" style:horizontal-pos="center" style:horizontal-rel="paragraph" fo:padding="0cm" fo:border="none"`, "")

	w := attrInt(n.Attrs, "width", 0)
	h := attrInt(n.Attrs, "height", 0)
	data, mediaW, mediaH := fodtImageData(src)
	if w <= 0 || h <= 0 {
		w, h = mediaW, mediaH
	}

	fmt.Fprintf(b, `<draw:frame draw:style-name="%s" draw:name="Image%d" text:anchor-type="as-char"`, frame, ctx.frameSeq)
	if w > 0 && h > 0 {
		fmt.Fprintf(b, ` svg:width="%s" svg:height="%s"`, pxToCm(w), pxToCm(h))
	}
	b.WriteString(`>`)
	if data != "" {
		// An inlined asset becomes the frame's own bytes. The alternative, leaving
		// the data: URI in xlink:href, is not equivalent: LibreOffice's ODF import
		// does not fetch data: URIs and the picture arrives as an empty box.
		b.WriteString(`<draw:image><office:binary-data>` + data + `</office:binary-data></draw:image>`)
	} else {
		b.WriteString(`<draw:image xlink:href="` + fodtAttr(src) + `" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/>`)
	}
	if alt, ok := n.Attrs["alt"].(string); ok && alt != "" {
		b.WriteString(`<svg:title>` + fodtText(alt) + `</svg:title>`)
	}
	b.WriteString(`</draw:frame>`)
	if !ctx.inPara {
		b.WriteString("</text:p>\n")
	}
}

// fodtImageData splits a data: URI into the base64 ODF wants, and reads the
// picture's own dimensions.
//
// The size matters more here than in HTML: a browser lays an <img> out from the
// file, but an ODF frame is sized by its attributes, and one without them is at
// the mercy of the reader's guess — LibreOffice picks a default that has nothing
// to do with the picture. An image whose header does not decode still travels;
// it is only the size that is unknown.
func fodtImageData(src string) (data string, w, h int) {
	if !strings.HasPrefix(src, "data:") {
		return "", 0, 0
	}
	i := strings.Index(src, ";base64,")
	if i < 0 {
		return "", 0, 0
	}
	data = strings.TrimSpace(src[i+len(";base64,"):])
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return data, 0, 0
	}
	cfg, _, err := image.DecodeConfig(strings.NewReader(string(raw)))
	if err != nil {
		return data, 0, 0
	}
	return data, cfg.Width, cfg.Height
}

// pxToCm converts a CSS pixel count to the length ODF stores, at the 96 dpi CSS
// defines.
func pxToCm(px int) string {
	return mmValue(float64(px)/96*2.54) + "cm"
}

// renderFODTPdf writes the export stand-in for an embedded PDF, the same one the
// HTML route writes and for the same reason: there is no markup that splices
// another PDF into a document, and dropping the node silently is the failure this
// renderer's coverage test exists to prevent.
func renderFODTPdf(b *strings.Builder, n docNode, ctx *fodtCtx) {
	ctx.flushMaster(b)
	name, _ := n.Attrs["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "документ.pdf"
	}
	label := fodtText("PDF: " + name)
	src, _ := n.Attrs["src"].(string)
	b.WriteString(`<text:p text:style-name="` + ctx.paraParent() + `">`)
	if src != "" && safeDocHref(src) {
		b.WriteString(`<text:a xlink:type="simple" xlink:href="` + fodtAttr(src) + `" text:style-name="Internet_20_Link">` + label + `</text:a>`)
	} else {
		b.WriteString(label)
	}
	b.WriteString("</text:p>\n")
}

// fodtCellBorder is the grid the HTML export draws with docExportCSS, restated
// as the ODF property. Kept identical on purpose: a table that has visible lines
// in the PDF and none in the .docx is the kind of difference nobody reports and
// everybody notices.
const fodtCellBorder = "0.018cm solid #999999"

// renderFODTTable writes a table:table.
//
// The one thing HTML gets for free and ODF does not is the covered cell: a
// browser lays out around a colspan, but ODF wants the positions a span swallows
// spelled out as <table:covered-table-cell/>. Without them every cell after a
// merge shifts one column left, which is why this walks a grid rather than the
// rows alone.
func renderFODTTable(b *strings.Builder, n docNode, ctx *fodtCtx) {
	ctx.tableSeq++
	master := ctx.masterAttr()
	style := ctx.styles.add("table", "", `style:width="17cm" table:align="margins"`, master)

	widths := fodtColumnWidths(n)
	cols := fodtColumnCount(n)
	if len(widths) > cols {
		cols = len(widths)
	}

	fmt.Fprintf(b, `<table:table table:name="Table%d" table:style-name="%s">`+"\n", ctx.tableSeq, style)
	for i := 0; i < cols; i++ {
		if i < len(widths) && widths[i] > 0 {
			colStyle := ctx.styles.add("table-column", "", `style:column-width="`+pxToCm(widths[i])+`"`, "")
			b.WriteString(`<table:table-column table:style-name="` + colStyle + `"/>`)
			continue
		}
		b.WriteString(`<table:table-column/>`)
	}
	b.WriteString("\n")

	// carry[col] is how many further rows the span above still covers.
	carry := map[int]int{}
	for _, row := range n.Content {
		if row.Type != "tableRow" {
			continue
		}
		b.WriteString("<table:table-row>")
		col := 0
		for _, cell := range row.Content {
			col = fodtSkipCovered(b, carry, col)
			colspan := attrInt(cell.Attrs, "colspan", 1)
			rowspan := attrInt(cell.Attrs, "rowspan", 1)
			if colspan < 1 {
				colspan = 1
			}
			if rowspan < 1 {
				rowspan = 1
			}
			renderFODTCell(b, cell, ctx, colspan, rowspan)
			for k := 1; k < colspan; k++ {
				b.WriteString("<table:covered-table-cell/>")
			}
			if rowspan > 1 {
				for k := 0; k < colspan; k++ {
					carry[col+k] = rowspan - 1
				}
			}
			col += colspan
		}
		fodtSkipCovered(b, carry, col)
		b.WriteString("</table:table-row>\n")
	}
	b.WriteString("</table:table>\n")
}

// fodtSkipCovered writes the covered cells a span from an earlier row occupies
// at this point in the row, and returns the column the next real cell lands in.
func fodtSkipCovered(b *strings.Builder, carry map[int]int, col int) int {
	for carry[col] > 0 {
		b.WriteString("<table:covered-table-cell/>")
		carry[col]--
		col++
	}
	return col
}

func renderFODTCell(b *strings.Builder, n docNode, ctx *fodtCtx, colspan, rowspan int) {
	var props []string
	props = append(props, `fo:padding="0.106cm"`)
	border := fodtCellBorder
	if s, ok := n.Attrs["borderColor"].(string); ok {
		if c := fodtColor(s); c != "" {
			border = "0.018cm solid " + c
		}
	}
	props = append(props, `fo:border="`+border+`"`)
	if s, ok := n.Attrs["backgroundColor"].(string); ok {
		if c := fodtColor(s); c != "" {
			props = append(props, `fo:background-color="`+c+`"`)
		}
	}
	style := ctx.styles.add("table-cell", "", strings.Join(props, " "), "")

	b.WriteString(`<table:table-cell table:style-name="` + style + `" office:value-type="string"`)
	if colspan > 1 {
		fmt.Fprintf(b, ` table:number-columns-spanned="%d"`, colspan)
	}
	if rowspan > 1 {
		fmt.Fprintf(b, ` table:number-rows-spanned="%d"`, rowspan)
	}
	b.WriteString(">")

	parent := "Table_20_Contents"
	if n.Type == "tableHeader" {
		parent = "Table_20_Heading"
	}
	// A cell must hold blocks. An empty one still needs a paragraph — ODF has no
	// "cell with nothing in it", and readers differ on what they do with one.
	if len(n.Content) == 0 {
		b.WriteString(`<text:p text:style-name="` + parent + `"/>`)
	}
	for _, c := range n.Content {
		if c.Type == "paragraph" {
			writeFODTPara(b, c, ctx, parent)
			continue
		}
		if c.Type == "text" || c.Type == "hardBreak" || c.Type == "image" {
			b.WriteString(`<text:p text:style-name="` + parent + `">`)
			was := ctx.inPara
			ctx.inPara = true
			renderFODTNode(b, c, ctx)
			ctx.inPara = was
			b.WriteString("</text:p>")
			continue
		}
		renderFODTNode(b, c, ctx)
	}
	b.WriteString("</table:table-cell>")
}

// fodtColumnWidths reads the column widths TipTap stores on the cells of the
// first row. colwidth is per-cell and holds one entry per column the cell spans,
// so the row has to be flattened rather than indexed.
func fodtColumnWidths(table docNode) []int {
	for _, row := range table.Content {
		if row.Type != "tableRow" {
			continue
		}
		var out []int
		for _, cell := range row.Content {
			span := attrInt(cell.Attrs, "colspan", 1)
			if span < 1 {
				span = 1
			}
			list, _ := cell.Attrs["colwidth"].([]any)
			for i := 0; i < span; i++ {
				w := 0
				if i < len(list) {
					if f, ok := list[i].(float64); ok {
						w = int(f)
					}
				}
				out = append(out, w)
			}
		}
		return out
	}
	return nil
}

// fodtColumnCount is the widest row's column count, spans included — the number
// of <table:table-column> the table has to declare.
func fodtColumnCount(table docNode) int {
	best := 0
	carry := map[int]int{}
	for _, row := range table.Content {
		if row.Type != "tableRow" {
			continue
		}
		col := 0
		for _, cell := range row.Content {
			for carry[col] > 0 {
				carry[col]--
				col++
			}
			colspan := attrInt(cell.Attrs, "colspan", 1)
			rowspan := attrInt(cell.Attrs, "rowspan", 1)
			if colspan < 1 {
				colspan = 1
			}
			if rowspan > 1 {
				for k := 0; k < colspan; k++ {
					carry[col+k] = rowspan - 1
				}
			}
			col += colspan
		}
		for carry[col] > 0 {
			carry[col]--
			col++
		}
		if col > best {
			best = col
		}
	}
	return best
}

// fodtTextStyleProps maps the TextStyle mark onto ODF text properties.
func fodtTextStyleProps(attrs map[string]any) []string {
	var out []string
	if s, ok := attrs["color"].(string); ok {
		if c := fodtColor(s); c != "" {
			out = append(out, `fo:color="`+c+`"`)
		}
	}
	if s, ok := attrs["fontFamily"].(string); ok && strings.TrimSpace(s) != "" {
		out = append(out, `style:font-name="`+fodtAttr(cssValue(s))+`"`)
	}
	if s, ok := attrs["fontSize"].(string); ok {
		if v := fodtFontSize(s); v != "" {
			out = append(out, `fo:font-size="`+v+`"`)
		}
	}
	return out
}

// fodtAttr escapes a value for an XML attribute.
func fodtAttr(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
		"\n", " ", "\r", " ", "\t", " ",
	).Replace(s)
}

// fodtText escapes character data and restores the whitespace XML would
// otherwise collapse.
//
// ODF is not HTML twice over here: a run of spaces collapses to one unless it is
// written as <text:s>, and a newline is whitespace rather than a break. Both
// matter for the content this renderer actually meets — indented code and
// aligned table text — so both are spelled out rather than left to the reader.
func fodtText(s string) string {
	var b strings.Builder
	spaces := 0
	flush := func() {
		if spaces == 0 {
			return
		}
		b.WriteString(" ")
		if spaces > 1 {
			fmt.Fprintf(&b, `<text:s text:c="%d"/>`, spaces-1)
		}
		spaces = 0
	}
	for _, r := range s {
		switch r {
		case ' ':
			spaces++
			continue
		case '\t':
			flush()
			b.WriteString("<text:tab/>")
			continue
		case '\n':
			flush()
			b.WriteString("<text:line-break/>")
			continue
		case '\r':
			continue
		}
		flush()
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return b.String()
}
