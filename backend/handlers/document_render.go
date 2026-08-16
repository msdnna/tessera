package handlers

import (
	"encoding/base64"
	"fmt"
	"html"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Rendering a stored document body to HTML, which is what the LibreOffice
// sidecar takes as the source for a .docx or .pdf export (#2733).
//
// This is a second renderer — the editor already turns the same tree into HTML
// in the browser — and that was a deliberate choice, not an oversight. Export
// as a plain GET on the document is worth it: it works from the tile grid
// without opening the editor, from Android (D11 is read-only and has no TipTap),
// and from a script holding a token. The alternative, having the client POST
// the HTML it rendered, would make an export of a document depend on a client
// that can already display it, and would mean the server hands LibreOffice
// markup it never inspected.
//
// The duplication is bounded because the node and mark vocabulary is not open:
// it is the allow-list in document_schema.go, which the validator already
// rejects anything outside of. TestRenderDocHTMLCoversSchema fails if a type is
// added there without being given a rendering here — the drift this design
// risks is the one thing that is checked automatically.

// blockStyleAttrs are the attributes BlockStyle (frontend docExtensions/blockStyle.js)
// puts on a block, mapped to the CSS LibreOffice understands on import.
var blockStyleAttrs = map[string]string{
	"textAlign":  "text-align",
	"lineHeight": "line-height",
}

// docRenderCtx is what the whole walk needs to know beyond the node in front of
// it: how to resolve an image src, and which blocks are pointed at by an
// internal link and therefore have to carry an anchor.
type docRenderCtx struct {
	resolveImage func(string) string
	anchors      map[string]bool
}

// renderDocHTML turns a validated document body into a standalone HTML page.
//
// resolveImage is given an <img src> and returns what should be written in its
// place; it is how the caller inlines document assets the sidecar cannot fetch
// (see inlineDocAsset). Returning "" drops the image.
func renderDocHTML(title string, root docNode, resolveImage func(string) string) string {
	ctx := docRenderCtx{resolveImage: resolveImage, anchors: internalLinkTargets(root)}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html><head>\n")
	// The charset declaration is not decoration: without it LibreOffice reads the
	// file in a locale-dependent encoding and a Cyrillic document arrives as
	// mojibake in the exported PDF.
	b.WriteString(`<meta charset="utf-8">` + "\n")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>\n")
	b.WriteString("<style>\n" + docExportCSS + "</style>\n")
	b.WriteString("</head><body>\n")
	if strings.TrimSpace(title) != "" {
		b.WriteString("<h1 class=\"doc-title\">" + html.EscapeString(title) + "</h1>\n")
	}
	renderDocNodes(&b, root.Content, ctx)
	b.WriteString("\n</body></html>\n")
	return b.String()
}

// internalLinkTargets collects the blocks an internal link points at.
//
// An internal link is a bare `#<block id>` href — the frontend writes block ids
// rather than slugs of the heading text (utils/docToc.js), so that renaming a
// heading does not break the links to it. Anchors are written only for the
// blocks actually linked to, rather than on every block: the export is read by
// LibreOffice, and an id on every paragraph of a long document is markup nobody
// asked for. Doing it in one pass up front is what makes that possible — a link
// may well point at a heading further down the page than the link itself.
func internalLinkTargets(root docNode) map[string]bool {
	out := map[string]bool{}
	var walk func(docNode)
	walk = func(n docNode) {
		for _, m := range n.Marks {
			if m.Type != "link" {
				continue
			}
			href, _ := m.Attrs["href"].(string)
			if id := strings.TrimPrefix(href, "#"); len(href) > 1 && id != href {
				out[id] = true
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(root)
	return out
}

// anchorAttr writes the id a link inside the document jumps to.
//
// Escaped rather than trusted: the id comes from stored content, which the
// schema validates as a string but does not constrain to the hex the editor
// generates.
func anchorAttr(n docNode, ctx docRenderCtx) string {
	id, _ := n.Attrs["id"].(string)
	if id == "" || !ctx.anchors[id] {
		return ""
	}
	return ` id="` + html.EscapeString(id) + `"`
}

// docExportCSS is the minimum that makes an exported document look like a
// document rather than a browser default. LibreOffice's HTML import honours a
// small subset of CSS; anything fancier is silently ignored, so this stays
// short on purpose.
const docExportCSS = `body { font-family: "Liberation Serif", serif; font-size: 12pt; }
h1, h2, h3, h4, h5, h6 { font-family: "Liberation Sans", sans-serif; }
code, pre { font-family: "Liberation Mono", monospace; font-size: 10pt; }
pre { background: #f5f5f5; padding: 8pt; }
blockquote { margin-left: 24pt; border-left: 2pt solid #cccccc; padding-left: 8pt; }
table { border-collapse: collapse; }
td, th { border: 0.5pt solid #999999; padding: 3pt; }
img { max-width: 100%; }
`

func renderDocNodes(b *strings.Builder, nodes []docNode, ctx docRenderCtx) {
	for _, n := range nodes {
		renderDocNode(b, n, ctx)
	}
}

func renderDocNode(b *strings.Builder, n docNode, ctx docRenderCtx) {
	switch n.Type {
	case "text":
		b.WriteString(renderDocText(n))
	case "paragraph":
		wrap(b, "p", n, ctx)
	case "heading":
		level := attrInt(n.Attrs, "level", 1)
		if level < 1 || level > 6 {
			level = 1
		}
		wrapTag(b, fmt.Sprintf("h%d", level), n, ctx)
	case "bulletList":
		wrap(b, "ul", n, ctx)
	case "orderedList":
		start := attrInt(n.Attrs, "start", 1)
		extra := ""
		if start > 1 {
			extra = fmt.Sprintf(` start="%d"`, start)
		}
		wrapExtra(b, "ol", extra, n, ctx)
	case "listItem":
		wrap(b, "li", n, ctx)
	case "taskList":
		wrapExtra(b, "ul", ` class="doc-tasks"`, n, ctx)
	case "taskItem":
		// The checkbox is written as a character rather than an <input>: a form
		// control round-trips into .docx as an empty box with no state, which
		// loses exactly the bit of information the item carries.
		box := "☐ "
		if checked, ok := n.Attrs["checked"].(bool); ok && checked {
			box = "☑ "
		}
		b.WriteString("<li>" + box)
		renderDocNodes(b, n.Content, ctx)
		b.WriteString("</li>\n")
	case "blockquote":
		wrap(b, "blockquote", n, ctx)
	case "codeBlock":
		b.WriteString("<pre><code>")
		for _, c := range n.Content {
			if c.Type == "text" && c.Text != nil {
				b.WriteString(html.EscapeString(*c.Text))
			}
		}
		b.WriteString("</code></pre>\n")
	case "horizontalRule":
		b.WriteString("<hr>\n")
	case "hardBreak":
		b.WriteString("<br>")
	case "image":
		renderDocImage(b, n, ctx)
	case "pdfEmbed":
		renderDocPdf(b, n)
	case "table":
		wrap(b, "table", n, ctx)
	case "tableRow":
		wrap(b, "tr", n, ctx)
	case "tableHeader":
		wrapExtra(b, "th", cellAttrs(n), n, ctx)
	case "tableCell":
		wrapExtra(b, "td", cellAttrs(n), n, ctx)
	default:
		// Unreachable for stored content: validateDocContent refuses anything not
		// in the allow-list, and TestRenderDocHTMLCoversSchema keeps this switch in
		// step with it. Rendering the children rather than nothing means a future
		// node type loses its wrapper instead of losing the user's text.
		renderDocNodes(b, n.Content, ctx)
	}
}

func wrap(b *strings.Builder, tag string, n docNode, ctx docRenderCtx) {
	wrapExtra(b, tag, "", n, ctx)
}

// wrapTag is wrap for tags that take no style attribute of their own beyond the
// block style; kept separate only for readability at the call site.
func wrapTag(b *strings.Builder, tag string, n docNode, ctx docRenderCtx) {
	wrapExtra(b, tag, "", n, ctx)
}

func wrapExtra(b *strings.Builder, tag, extra string, n docNode, ctx docRenderCtx) {
	b.WriteString("<" + tag + anchorAttr(n, ctx) + extra + blockStyle(n.Attrs) + ">")
	renderDocNodes(b, n.Content, ctx)
	b.WriteString("</" + tag + ">\n")
}

// blockStyle renders the alignment/spacing/indent attributes as inline CSS.
func blockStyle(attrs map[string]any) string {
	if len(attrs) == 0 {
		return ""
	}
	var decls []string
	// Sorted so the output is deterministic — a test comparing rendered HTML
	// would otherwise flake on Go's map iteration order.
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		css, ok := blockStyleAttrs[k]
		if !ok {
			continue
		}
		if s, ok := attrs[k].(string); ok && s != "" {
			decls = append(decls, css+":"+cssValue(s))
		}
	}
	if indent := attrInt(attrs, "indent", 0); indent > 0 {
		decls = append(decls, fmt.Sprintf("margin-left:%dpt", indent*24))
	}
	if len(decls) == 0 {
		return ""
	}
	return ` style="` + html.EscapeString(strings.Join(decls, ";")) + `"`
}

func cellAttrs(n docNode) string {
	out := ""
	if v := attrInt(n.Attrs, "colspan", 1); v > 1 {
		out += fmt.Sprintf(` colspan="%d"`, v)
	}
	if v := attrInt(n.Attrs, "rowspan", 1); v > 1 {
		out += fmt.Sprintf(` rowspan="%d"`, v)
	}
	return out
}

// renderDocText applies the node's marks, innermost last so the nesting reads
// the way the editor produced it.
func renderDocText(n docNode) string {
	if n.Text == nil {
		return ""
	}
	out := html.EscapeString(*n.Text)
	for i := len(n.Marks) - 1; i >= 0; i-- {
		m := n.Marks[i]
		switch m.Type {
		case "bold":
			out = "<strong>" + out + "</strong>"
		case "italic":
			out = "<em>" + out + "</em>"
		case "strike":
			out = "<s>" + out + "</s>"
		case "underline":
			out = "<u>" + out + "</u>"
		case "code":
			out = "<code>" + out + "</code>"
		case "link":
			href, _ := m.Attrs["href"].(string)
			// safeDocHref already refused javascript:/data: at write time; this is
			// the second gate, because the renderer must not depend on when the row
			// was written to stay safe.
			if href == "" || !safeDocHref(href) {
				break
			}
			out = `<a href="` + html.EscapeString(href) + `">` + out + "</a>"
		case "textStyle":
			if style := textStyleCSS(m.Attrs); style != "" {
				out = `<span style="` + html.EscapeString(style) + `">` + out + "</span>"
			}
		}
	}
	return out
}

func textStyleCSS(attrs map[string]any) string {
	var decls []string
	for _, pair := range [][2]string{
		{"color", "color"},
		{"fontFamily", "font-family"},
		{"fontSize", "font-size"},
	} {
		if s, ok := attrs[pair[0]].(string); ok && s != "" {
			decls = append(decls, pair[1]+":"+cssValue(s))
		}
	}
	return strings.Join(decls, ";")
}

// cssValue strips the characters that would let an attribute value break out of
// the declaration it sits in. The values come from stored content, which is
// validated but not itself a CSS parser.
func cssValue(s string) string {
	return strings.NewReplacer(";", "", "\"", "", "'", "", "<", "", ">", "", "\n", " ", "\r", " ").Replace(s)
}

func renderDocImage(b *strings.Builder, n docNode, ctx docRenderCtx) {
	src, _ := n.Attrs["src"].(string)
	if ctx.resolveImage != nil {
		src = ctx.resolveImage(src)
	}
	if src == "" {
		return
	}
	b.WriteString(`<img src="` + html.EscapeString(src) + `"`)
	if alt, ok := n.Attrs["alt"].(string); ok && alt != "" {
		b.WriteString(` alt="` + html.EscapeString(alt) + `"`)
	}
	for _, k := range []string{"width", "height"} {
		if v := attrInt(n.Attrs, k, 0); v > 0 {
			fmt.Fprintf(b, ` %s="%d"`, k, v)
		}
	}
	b.WriteString(">\n")
}

// renderDocPdf writes the export stand-in for an embedded PDF.
//
// The pages themselves cannot come along: the sidecar builds a .docx or a .pdf
// out of this HTML, and there is no markup that says "splice another PDF in
// here". Dropping the node silently is the failure mode TestRenderDocHTMLCoversSchema
// exists to prevent, so the export carries a visible line with the file name and
// a link to it instead — a reader of the exported file can still get to it.
func renderDocPdf(b *strings.Builder, n docNode) {
	name, _ := n.Attrs["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "документ.pdf"
	}
	src, _ := n.Attrs["src"].(string)
	label := "PDF: " + html.EscapeString(name)
	b.WriteString(`<p class="doc-pdf">`)
	if src != "" {
		b.WriteString(`<a href="` + html.EscapeString(src) + `">` + label + `</a>`)
	} else {
		b.WriteString(label)
	}
	b.WriteString("</p>\n")
}

// attrInt reads a numeric attribute. ProseMirror JSON decodes numbers as
// float64, and attributes also arrive as strings from imported HTML, so both
// are accepted rather than silently falling back to the default.
func attrInt(attrs map[string]any, key string, def int) int {
	v, ok := attrs[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n := 0
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

// inlineDocAsset turns a document image URL into a data: URI by reading the
// file from disk.
//
// The sidecar cannot fetch these itself, and that is by design rather than a
// gap: document images are served through an HMAC-signed route precisely so
// that document content is not reachable by anything holding a URL (#2718), and
// handing that URL to another service would be the exact leak the signature
// exists to prevent. Inlining keeps the bytes on a path the request is already
// authorised for.
//
// A link to anything else — an external https:// image, or a URL whose
// signature does not check out — is left alone, and LibreOffice decides for
// itself whether it can fetch it.
func (h *API) inlineDocAsset(wsID uuid.UUID) func(string) string {
	return func(src string) string {
		if src == "" || !strings.Contains(src, "/api/documents/asset") {
			return src
		}
		u, err := url.Parse(src)
		if err != nil {
			return src
		}
		q := u.Query()
		docID, err := uuid.Parse(q.Get("doc"))
		if err != nil {
			return src
		}
		name := filepath.Base(q.Get("n"))
		if !docAssetNameRe.MatchString(name) {
			return src
		}
		// The signature is re-checked rather than trusted: the src came out of
		// stored content, and content is user input. Without this, a member of one
		// workspace could put another workspace's asset URL in their document and
		// have the export read the file for them.
		if q.Get("sig") != h.signAsset(wsID, docAssetRel(docID, name)) {
			return src
		}
		raw, err := os.ReadFile(h.docAssetPath(docID, name))
		if err != nil {
			return src
		}
		ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
		if ct == "" {
			ct = "application/octet-stream"
		}
		return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(raw)
	}
}
