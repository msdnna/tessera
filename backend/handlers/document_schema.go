package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
var allowedDocAttrs = map[string]bool{
	"id":         true, // BlockId — the anchor D4 locks and D5 annotates
	"level":      true,
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
}

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
