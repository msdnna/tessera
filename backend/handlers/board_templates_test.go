package handlers

import (
	"strings"
	"testing"

	"tessera/internal/i18n"
)

// A template column is keyed like the default four, so `/move` has to know its
// caption in every language the clients show it in, and the Russian arm has to
// be the very name stored on the row (#3047).
func TestBoardTemplateColumnsHaveCaptions(t *testing.T) {
	for key, tpl := range boardTemplates {
		if len(tpl.columns) < 2 {
			t.Fatalf("%s: a template needs a working column and a done one", key)
		}
		if _, ok := boardLayout(tpl.layout); !ok {
			t.Fatalf("%s: unknown layout %q", key, tpl.layout)
		}
		for _, col := range tpl.columns {
			for _, lang := range i18n.Supported {
				caption := seededColumnCaptions[lang][col.nameKey]
				if strings.TrimSpace(caption) == "" {
					t.Fatalf("%s/%s: no %s caption", key, col.nameKey, lang)
				}
				if got := seededColumnKey(caption); got != col.nameKey {
					t.Fatalf("%s: seededColumnKey(%q) = %q, want %q", key, caption, got, col.nameKey)
				}
			}
			if seededColumnCaptions["ru"][col.nameKey] != col.name {
				t.Fatalf("%s/%s: stored name %q ≠ ru caption %q", key, col.nameKey, col.name,
					seededColumnCaptions["ru"][col.nameKey])
			}
		}
		for _, tag := range tpl.tags {
			for _, lang := range i18n.Supported {
				if tag.names[lang] == "" {
					t.Fatalf("%s: a tag has no %s name: %v", key, lang, tag.names)
				}
			}
		}
	}
}
