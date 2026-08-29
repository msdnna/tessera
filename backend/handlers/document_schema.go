package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"tessera/internal/office"
)

// The document body is a ProseMirror tree (documents.content, jsonb). On the
// client the editor schema *is* the allow-list — a node type it does not
// declare cannot survive parsing. That only protects clients that go through
// our frontend, though: this endpoint takes JSON, so the same list is enforced
// here. The names must stay in step with frontend/src/utils/docSchema.js;
// TestDocumentSchemaMatchesFrontend reads that file and compares.
var allowedDocNodes = map[string]bool{
	"doc":            true,
	"paragraph":      true,
	"text":           true,
	"heading":        true,
	"bulletList":     true,
	"orderedList":    true,
	"listItem":       true,
	"taskList":       true,
	"taskItem":       true,
	"blockquote":     true,
	"codeBlock":      true,
	"horizontalRule": true,
	"hardBreak":      true,
	"image":          true,
	"table":          true,
	"tableRow":       true,
	"tableHeader":    true,
	"tableCell":      true,
	// A PDF is the one format of item 1 of #2718 that is read rather than
	// converted: LibreOffice turns a PDF into a page of positioned text boxes,
	// which is worse than useless as a block tree. So it stays a file and the
	// document holds a reference to it — one atom block rendered by pdf.js.
	"pdfEmbed": true,
	// Boundary between two page geometries inside one document (#2827). It is
	// the second node allowed to carry "page", and the reason checkDocPage is
	// reached from two places below rather than only from the doc node.
	"sectionBreak": true,
}

var allowedDocMarks = map[string]bool{
	"bold":      true,
	"italic":    true,
	"strike":    true,
	"underline": true,
	"code":      true,
	"link":      true,
	"textStyle": true,
}

// Attribute names the extensions above can produce. Kept as one set rather than
// per-type: the precision gained by splitting it does not buy a security
// property (the type is already checked), and a per-type table drifts silently
// every time an extension gains an option.
//
// This list was written by reading the extensions, and that is exactly how it
// went wrong: "align" and "type" are added by TipTap itself, so no editor code
// mentions them, and every document containing a table was rejected with a 400
// the moment a user inserted one (#2728). It is now derived from the schema on
// the frontend (docSchema.js ALLOWED_ATTRS) and compared here by
// TestDocumentSchemaMatchesFrontend.
var allowedDocAttrs = map[string]bool{
	"id":         true, // BlockId — the anchor D4 locks and D5 annotates
	"level":      true,
	"align":      true, // TipTap puts this on table cells, not us — see below
	"type":       true, // ...and this on ordered lists
	"textAlign":  true,
	"lineHeight": true,
	"indent":     true,
	"start":      true,
	"checked":    true,
	"language":   true,
	"src":        true,
	"alt":        true,
	"title":      true,
	"width":      true,
	"height":     true,
	"href":       true,
	"target":     true,
	"rel":        true,
	"class":      true,
	"colspan":    true,
	"rowspan":    true,
	"colwidth":   true,
	"color":      true,
	"fontFamily": true,
	"fontSize":   true,
	// Fill and grid of a table cell, kept from an imported document (#2756).
	"backgroundColor": true,
	"borderColor":     true,
	"name":            true, // pdfEmbed — the original file name, shown in its header
	"size":            true, // pdfEmbed — byte size, shown before the file is fetched
	// Page geometry, on the doc node only (#2821). Unlike every other entry here
	// it carries an object rather than a scalar, and it is the one attribute that
	// leaves the document: the export writes it into an @page rule LibreOffice
	// obeys. checkDocPage below is therefore stricter than the bare membership
	// test the rest of this list gets.
	"page": true,
}

// docPageKeys are the six numbers a page geometry consists of, in millimetres:
// the sheet and its four margins. Exactly these — no more (an unknown key is a
// client writing something we would then export blind) and no fewer (a partial
// geometry has no sensible completion, since "the rest of A4" is a guess).
var docPageKeys = []string{"w", "h", "ml", "mr", "mt", "mb"}

const (
	maxDocDepth       = 32
	maxDocNodes       = 20000
	maxDocPreviewLen  = 200
	maxDocContentSize = 4 << 20 // 4 MiB of JSON
)

// docNode is the shape of one ProseMirror node. Unknown keys are tolerated at
// the parse step and rejected below, so a payload can never smuggle fields past
// the validator by hiding them in an unmodelled position.
type docNode struct {
	Type    string          `json:"type"`
	Text    *string         `json:"text,omitempty"`
	Attrs   map[string]any  `json:"attrs,omitempty"`
	Content []docNode       `json:"content,omitempty"`
	Marks   []docMark       `json:"marks,omitempty"`
	Extra   map[string]bool `json:"-"`
}

type docMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// validateDocContent parses and checks a document body, returning the
// re-serialised tree (so anything the model dropped never reaches the database)
// and a plain-text preview for the tile grid.
func validateDocContent(raw json.RawMessage) (json.RawMessage, string, error) {
	if len(raw) == 0 {
		return nil, "", errors.New("content is required")
	}
	if len(raw) > maxDocContentSize {
		return nil, "", fmt.Errorf("content exceeds %d bytes", maxDocContentSize)
	}
	var root docNode
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&root); err != nil {
		return nil, "", errors.New("content is not valid document JSON")
	}
	if root.Type != "doc" {
		return nil, "", errors.New("content root must be a doc node")
	}
	count := 0
	if err := checkDocNode(root, 0, &count); err != nil {
		return nil, "", err
	}
	clean, err := json.Marshal(root)
	if err != nil {
		return nil, "", err
	}
	return clean, docPreview(root), nil
}

func checkDocNode(n docNode, depth int, count *int) error {
	if depth > maxDocDepth {
		return fmt.Errorf("content nested deeper than %d levels", maxDocDepth)
	}
	*count++
	if *count > maxDocNodes {
		return fmt.Errorf("content has more than %d nodes", maxDocNodes)
	}
	if !allowedDocNodes[n.Type] {
		return fmt.Errorf("node type %q is not allowed", n.Type)
	}
	if n.Type == "text" && (n.Text == nil || *n.Text == "") {
		return errors.New("text node without text")
	}
	for k := range n.Attrs {
		if !allowedDocAttrs[k] {
			return fmt.Errorf("attribute %q is not allowed on %s", k, n.Type)
		}
	}
	if raw, ok := n.Attrs["page"]; ok {
		// Two carriers, and only two: the doc node holds the geometry of
		// everything up to the first section break, each break holds the
		// geometry of what follows it (#2827). A "page" anywhere else is a
		// client writing into a place nothing reads, which would then be
		// exported blind the day something did.
		if n.Type != "doc" && n.Type != "sectionBreak" {
			return errors.New(`attribute "page" is only allowed on the doc and sectionBreak nodes`)
		}
		if err := checkDocPage(raw); err != nil {
			return err
		}
	}
	for _, m := range n.Marks {
		if !allowedDocMarks[m.Type] {
			return fmt.Errorf("mark type %q is not allowed", m.Type)
		}
		for k, v := range m.Attrs {
			if !allowedDocAttrs[k] {
				return fmt.Errorf("attribute %q is not allowed on mark %s", k, m.Type)
			}
			if k == "href" && !safeDocHref(v) {
				return errors.New("link scheme is not allowed")
			}
		}
	}
	if src, ok := n.Attrs["src"]; ok && !safeDocHref(src) {
		return errors.New("image source is not allowed")
	}
	for _, child := range n.Content {
		if err := checkDocNode(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

// checkDocPage validates the page geometry carried on the doc node.
//
// It is checked here rather than only in the editor because this value is the
// one piece of document content the export turns into CSS the sidecar acts on:
// `@page { size: <w>mm <h>mm }` built from an unchecked number is how a document
// body ends up steering LibreOffice. A null is fine and common — ProseMirror
// serialises an unset attribute rather than omitting it, so every document saved
// by the editor carries `page: null` until the user opens the page dialog.
func checkDocPage(raw any) error {
	if raw == nil {
		return nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return errors.New("page must be an object")
	}
	if len(obj) != len(docPageKeys) {
		return fmt.Errorf("page must have exactly %d keys", len(docPageKeys))
	}
	vals := make(map[string]float64, len(docPageKeys))
	for _, k := range docPageKeys {
		v, ok := obj[k]
		if !ok {
			return fmt.Errorf("page is missing %q", k)
		}
		// Only float64: JSON numbers decode as float64, so anything else is a
		// string or a bool dressed up as a measurement.
		f, ok := v.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("page.%s must be a number", k)
		}
		vals[k] = f
	}
	for _, k := range []string{"w", "h"} {
		if vals[k] < office.MinSide || vals[k] > office.MaxSide {
			return fmt.Errorf("page.%s must be between %g and %g mm", k, office.MinSide, office.MaxSide)
		}
	}
	for _, k := range []string{"ml", "mr", "mt", "mb"} {
		if vals[k] < 0 {
			return fmt.Errorf("page.%s must not be negative", k)
		}
	}
	// Margins that meet leave no column to print in — a geometry the sheet cannot
	// render and the export would turn into an empty page.
	if vals["ml"]+vals["mr"] >= vals["w"] || vals["mt"]+vals["mb"] >= vals["h"] {
		return errors.New("page margins leave no printable area")
	}
	return nil
}

// safeDocHref keeps javascript:/data: URLs out of stored content. The renderer
// sanitises too — this is the layer that stops them being persisted at all.
func safeDocHref(v any) bool {
	s, ok := v.(string)
	if !ok {
		return v == nil
	}
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return true
	}
	if strings.HasPrefix(low, "/") || strings.HasPrefix(low, "#") {
		return true
	}
	for _, p := range []string{"http://", "https://", "mailto:", "tel:"} {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	// Anything without a scheme is a relative link; anything with one we did not
	// list (javascript:, data:, vbscript:) is refused.
	return !strings.Contains(strings.SplitN(low, "/", 2)[0], ":")
}

// docPreview flattens the tree into the short plain-text line the document
// tiles show. Truncation is by rune, not byte, so a Cyrillic preview is not cut
// mid-character.
func docPreview(root docNode) string {
	var b strings.Builder
	var walk func(docNode)
	walk = func(n docNode) {
		if b.Len() > maxDocPreviewLen*4 {
			return
		}
		if n.Type == "text" && n.Text != nil {
			b.WriteString(*n.Text)
			return
		}
		for _, c := range n.Content {
			walk(c)
		}
		if len(n.Content) > 0 {
			b.WriteString(" ")
		}
	}
	walk(root)
	text := strings.Join(strings.Fields(b.String()), " ")
	runes := []rune(text)
	if len(runes) > maxDocPreviewLen {
		return strings.TrimSpace(string(runes[:maxDocPreviewLen]))
	}
	return text
}

// isEmptyDocContent reports whether stored content holds nothing worth keeping
// in the version journal. It answers one question only: whether a document's
// first tracked save deserves a "before" baseline (#2731). A document created a
// minute ago and typed into for the first time has no earlier state anyone would
// want back, and a baseline for it would be an empty entry at the bottom of
// every journal.
//
// Unparseable content counts as non-empty: if we cannot read it, the one thing
// not to do is decide on the user's behalf that it can be dropped.
func isEmptyDocContent(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var root docNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return false
	}
	if docPreview(root) != "" {
		return false
	}
	// Text is not the only content: a document holding a single image or table
	// is not empty, even though it flattens to an empty preview.
	empty := true
	var walk func(docNode)
	walk = func(n docNode) {
		if !empty {
			return
		}
		if n.Type == "image" || n.Type == "table" || n.Type == "horizontalRule" {
			empty = false
			return
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(root)
	return empty
}
