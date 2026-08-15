package handlers

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestValidateDocContent(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"empty doc", `{"type":"doc","content":[]}`, false},
		{"paragraph with marks", `{"type":"doc","content":[{"type":"paragraph","attrs":{"id":"b1","textAlign":"center"},"content":[{"type":"text","text":"привет","marks":[{"type":"bold"}]}]}]}`, false},
		{"table", `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph"}]}]}]}]}`, false},
		// Verbatim from the browser: this is what insertTable() actually sends,
		// down to the attributes TipTap adds on its own. The hand-written case
		// above passed while this one returned 400 (#2728).
		{"table as the editor emits it", `{"type":"doc","content":[{"type":"table","attrs":{"id":"da9b"},"content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":1,"rowspan":1,"colwidth":null,"align":null},"content":[{"type":"paragraph","attrs":{"textAlign":null,"id":"35a8","lineHeight":null,"indent":null}}]}]}]}]}`, false},
		{"ordered list as the editor emits it", `{"type":"doc","content":[{"type":"orderedList","attrs":{"start":1,"type":null,"id":"b1"},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"раз"}]}]}]}]}`, false},
		{"relative link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"/documents/x"}}]}]}]}`, false},
		{"not a doc", `{"type":"paragraph"}`, true},
		{"unknown node", `{"type":"doc","content":[{"type":"iframe"}]}`, true},
		{"unknown mark", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"blink"}]}]}]}`, true},
		{"unknown attribute", `{"type":"doc","content":[{"type":"paragraph","attrs":{"onclick":"x"}}]}`, true},
		{"javascript href", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`, true},
		{"data image", `{"type":"doc","content":[{"type":"image","attrs":{"src":"data:text/html,<script>"}}]}`, true},
		{"empty text node", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}`, true},
		{"garbage", `{"nope"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := validateDocContent(json.RawMessage(tc.body))
			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got none")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateDocContentDepthLimit(t *testing.T) {
	// A deeply nested tree is cheap to send and expensive to walk; the limit is
	// what stops one request from recursing through an arbitrary depth.
	body := strings.Repeat(`{"type":"blockquote","content":[`, maxDocDepth+2) +
		`{"type":"paragraph"}` + strings.Repeat(`]}`, maxDocDepth+2)
	if _, _, err := validateDocContent(json.RawMessage(`{"type":"doc","content":[` + body + `]}`)); err == nil {
		t.Fatal("expected the depth limit to reject this document")
	}
}

func TestDocPreview(t *testing.T) {
	body := `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"первый"}]},
		{"type":"paragraph","content":[{"type":"text","text":"второй"}]}]}`
	_, preview, err := validateDocContent(json.RawMessage(body))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if preview != "первый второй" {
		t.Fatalf("preview = %q", preview)
	}
}

func TestDocPreviewTruncatesByRune(t *testing.T) {
	long := strings.Repeat("я", maxDocPreviewLen*2)
	body := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + long + `"}]}]}`
	_, preview, err := validateDocContent(json.RawMessage(body))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	// Cut by rune, not byte: a Cyrillic preview sliced mid-character would come
	// back as replacement glyphs.
	if got := len([]rune(preview)); got != maxDocPreviewLen {
		t.Fatalf("preview is %d runes, want %d", got, maxDocPreviewLen)
	}
	if !json.Valid([]byte(`"` + preview + `"`)) {
		t.Fatal("preview is not valid UTF-8 after truncation")
	}
}

// TestDocumentSchemaMatchesFrontend keeps the two allow-lists in step. They are
// in different languages and cannot share a file, so the check reads the
// frontend's list rather than trusting a comment: a node added to the editor
// and forgotten here would be rejected by the server the moment a user typed
// it, which is exactly the kind of break that shows up in production only.
func TestDocumentSchemaMatchesFrontend(t *testing.T) {
	src, err := os.ReadFile("../../frontend/src/utils/docSchema.js")
	if err != nil {
		t.Skipf("frontend sources not available: %v", err)
	}
	check := func(constName string, allowed map[string]bool) {
		re := regexp.MustCompile(`(?s)export const ` + constName + ` = \[(.*?)\]`)
		m := re.FindSubmatch(src)
		if m == nil {
			t.Fatalf("could not find %s in docSchema.js", constName)
		}
		names := regexp.MustCompile(`'([a-zA-Z]+)'`).FindAllStringSubmatch(string(m[1]), -1)
		if len(names) != len(allowed) {
			t.Fatalf("%s has %d entries, Go side has %d", constName, len(names), len(allowed))
		}
		for _, n := range names {
			if !allowed[n[1]] {
				t.Fatalf("%s contains %q, which the Go validator rejects", constName, n[1])
			}
		}
	}
	check("ALLOWED_NODES", allowedDocNodes)
	check("ALLOWED_MARKS", allowedDocMarks)
	// Attributes are the half that broke (#2728): the frontend list is derived
	// from the loaded extensions by ut-docSchema.spec.js, so comparing against
	// it here is what keeps a TipTap-supplied attribute from reaching the server
	// as a 400 on the user's first table.
	check("ALLOWED_ATTRS", allowedDocAttrs)
}
