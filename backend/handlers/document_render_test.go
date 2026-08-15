package handlers

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func txt(s string, marks ...docMark) docNode {
	v := s
	return docNode{Type: "text", Text: &v, Marks: marks}
}

func render(n docNode) string {
	return renderDocHTML("", docNode{Type: "doc", Content: []docNode{n}}, nil)
}

// nodeRendering is the expected markup for one of every node type the schema
// allows. It is a map keyed by node type rather than a slice of cases so the
// coverage assertion below can compare it against allowedDocNodes directly.
var nodeRendering = map[string]struct {
	node docNode
	want string
}{
	"doc":            {docNode{Type: "doc"}, ""},
	"text":           {txt("привет"), "привет"},
	"paragraph":      {docNode{Type: "paragraph", Content: []docNode{txt("a")}}, "<p>a</p>"},
	"heading":        {docNode{Type: "heading", Attrs: map[string]any{"level": float64(3)}, Content: []docNode{txt("h")}}, "<h3>h</h3>"},
	"bulletList":     {docNode{Type: "bulletList"}, "<ul></ul>"},
	"orderedList":    {docNode{Type: "orderedList", Attrs: map[string]any{"start": float64(4)}}, `<ol start="4">`},
	"listItem":       {docNode{Type: "listItem", Content: []docNode{txt("i")}}, "<li>i</li>"},
	"taskList":       {docNode{Type: "taskList"}, `<ul class="doc-tasks">`},
	"taskItem":       {docNode{Type: "taskItem", Attrs: map[string]any{"checked": true}, Content: []docNode{txt("d")}}, "<li>☑ d</li>"},
	"blockquote":     {docNode{Type: "blockquote", Content: []docNode{txt("q")}}, "<blockquote>q</blockquote>"},
	"codeBlock":      {docNode{Type: "codeBlock", Content: []docNode{txt("x < y")}}, "<pre><code>x &lt; y</code></pre>"},
	"horizontalRule": {docNode{Type: "horizontalRule"}, "<hr>"},
	"hardBreak":      {docNode{Type: "hardBreak"}, "<br>"},
	"image":          {docNode{Type: "image", Attrs: map[string]any{"src": "/x.png", "alt": "к"}}, `<img src="/x.png" alt="к">`},
	"table":          {docNode{Type: "table"}, "<table></table>"},
	"tableRow":       {docNode{Type: "tableRow"}, "<tr></tr>"},
	"tableHeader":    {docNode{Type: "tableHeader", Content: []docNode{txt("H")}}, "<th>H</th>"},
	"tableCell":      {docNode{Type: "tableCell", Attrs: map[string]any{"colspan": float64(2)}}, `<td colspan="2">`},
}

// TestRenderDocHTMLCoversSchema is the guard on the one thing this design
// risks. The renderer duplicates knowledge that also lives in the editor, and
// the way that goes wrong is quietly: a node type is added to the allow-list,
// the validator starts accepting it, and export drops it on the floor because
// nothing here knows the tag. Then a user's table or checklist is simply
// missing from their PDF, and no error is raised anywhere.
func TestRenderDocHTMLCoversSchema(t *testing.T) {
	for nodeType := range allowedDocNodes {
		if _, ok := nodeRendering[nodeType]; !ok {
			t.Errorf("node type %q is allowed by the schema but has no rendering (add it to renderDocNode and to nodeRendering)", nodeType)
		}
	}
	for nodeType := range nodeRendering {
		if !allowedDocNodes[nodeType] {
			t.Errorf("nodeRendering covers %q, which the schema no longer allows", nodeType)
		}
	}
}

func TestRenderDocHTMLNodes(t *testing.T) {
	for nodeType, tc := range nodeRendering {
		if nodeType == "doc" {
			continue // the root is the argument, not a child
		}
		got := render(tc.node)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s rendered as %q, want it to contain %q", nodeType, got, tc.want)
		}
	}
}

func TestRenderDocHTMLMarks(t *testing.T) {
	cases := []struct {
		name string
		mark docMark
		want string
	}{
		{"bold", docMark{Type: "bold"}, "<strong>t</strong>"},
		{"italic", docMark{Type: "italic"}, "<em>t</em>"},
		{"strike", docMark{Type: "strike"}, "<s>t</s>"},
		{"underline", docMark{Type: "underline"}, "<u>t</u>"},
		{"code", docMark{Type: "code"}, "<code>t</code>"},
		{"link", docMark{Type: "link", Attrs: map[string]any{"href": "https://e.org"}}, `<a href="https://e.org">t</a>`},
		{"textStyle", docMark{Type: "textStyle", Attrs: map[string]any{"color": "#ff0000"}}, `<span style="color:#ff0000">t</span>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := render(docNode{Type: "paragraph", Content: []docNode{txt("t", tc.mark)}})
			if !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
	// Every mark the schema allows has to be here for the same reason node
	// coverage is checked: a mark with no rendering silently loses formatting.
	for markType := range allowedDocMarks {
		found := false
		for _, tc := range cases {
			if tc.mark.Type == markType {
				found = true
			}
		}
		if !found {
			t.Errorf("mark %q is allowed by the schema but is not rendered or tested", markType)
		}
	}
}

func TestRenderDocHTMLEscapesAndRefusesUnsafeLinks(t *testing.T) {
	got := render(docNode{Type: "paragraph", Content: []docNode{txt(`<script>alert("x")</script>`)}})
	if strings.Contains(got, "<script>") {
		t.Fatalf("text was not escaped: %q", got)
	}

	// safeDocHref already refuses this at write time. The renderer checks again
	// because a row written before that guard existed must not become an
	// executable link the moment it is exported.
	got = render(docNode{Type: "paragraph", Content: []docNode{
		txt("t", docMark{Type: "link", Attrs: map[string]any{"href": "javascript:alert(1)"}}),
	}})
	if strings.Contains(got, "javascript:") {
		t.Fatalf("javascript: link survived rendering: %q", got)
	}
	if !strings.Contains(got, "t") {
		t.Fatalf("refusing the link also dropped the text: %q", got)
	}
}

func TestRenderDocHTMLBlockStyleIsDeterministic(t *testing.T) {
	n := docNode{Type: "paragraph", Attrs: map[string]any{
		"textAlign":  "center",
		"lineHeight": "1.5",
		"indent":     float64(2),
	}, Content: []docNode{txt("t")}}
	first := render(n)
	for i := 0; i < 20; i++ {
		if render(n) != first {
			t.Fatal("style declarations are emitted in map order — output is not reproducible")
		}
	}
	if !strings.Contains(first, "text-align:center") ||
		!strings.Contains(first, "line-height:1.5") ||
		!strings.Contains(first, "margin-left:48pt") {
		t.Fatalf("block style not rendered: %q", first)
	}
}

func TestRenderDocHTMLDeclaresCharset(t *testing.T) {
	// Without this LibreOffice guesses the encoding and a Cyrillic document
	// exports as mojibake — a failure that looks like data loss and passes every
	// check that only counts bytes.
	page := renderDocHTML("Протокол", docNode{Type: "doc"}, nil)
	if !strings.Contains(page, `<meta charset="utf-8">`) {
		t.Fatal("no charset declaration in the exported page")
	}
	if !strings.Contains(page, "Протокол") {
		t.Fatal("document title missing from the export")
	}
}

func TestInlineDocAssetEmbedsOwnFileAndLeavesOthersAlone(t *testing.T) {
	dir := t.TempDir()
	h := &API{uploadDir: dir, assetKey: []byte("test-key")}
	wsID, docID := uuid.New(), uuid.New()
	name := uuid.NewString() + ".png"
	if err := os.MkdirAll(filepath.Join(dir, "documents", docID.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	if err := os.WriteFile(h.docAssetPath(docID, name), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	resolve := h.inlineDocAsset(wsID)
	got := resolve(h.docAssetURL(wsID, docID, name))
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload)
	if got != want {
		t.Fatalf("own asset not inlined:\n got %q\nwant %q", got, want)
	}

	if got := resolve("https://example.org/a.png"); got != "https://example.org/a.png" {
		t.Fatalf("external image was rewritten: %q", got)
	}

	// A member can type any URL into their document. Without the signature check
	// the export would read a file belonging to another workspace on their
	// behalf — the exact leak the signed route exists to prevent.
	forged := "/api/documents/asset?doc=" + docID.String() + "&ws=" + wsID.String() + "&n=" + name + "&sig=deadbeef"
	if got := resolve(forged); got != forged {
		t.Fatalf("forged signature was honoured and the file inlined: %q", got)
	}
}
