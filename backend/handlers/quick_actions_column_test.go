package handlers

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/i18n"
)

func seededCols(t *testing.T) []db.BoardColumn {
	t.Helper()
	cols := make([]db.BoardColumn, 0, len(defaultColumns))
	for _, dc := range defaultColumns {
		key := dc.nameKey
		cols = append(cols, db.BoardColumn{ID: uuid.New(), Name: dc.name, NameKey: &key})
	}
	return cols
}

// A seeded column is stored under its Russian name but captioned by each client
// from its name_key (#2800), so an English reader's board says "Done" where the
// row says «Готово». `/move Done` has to reach that column: the user is echoing
// the caption their own screen shows them.
func TestMatchColumnBySeededCaption(t *testing.T) {
	cols := seededCols(t)
	byKey := map[string]uuid.UUID{}
	for _, c := range cols {
		byKey[*c.NameKey] = c.ID
	}

	for _, c := range []struct {
		name, typed, wantKey string
	}{
		{"stored russian name", "Готово", "done"},
		{"stored name, other case", "готово", "done"},
		{"stored name, padded", "  В процессе  ", "in_progress"},
		{"english caption", "Done", "done"},
		{"english caption, other case", "done", "done"},
		{"english caption, two words", "In progress", "in_progress"},
		{"english caption, padded", "  To do  ", "todo"},
		{"english review caption", "In review", "review"},
		{"not a column at all", "Backlog", ""},
		{"empty", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := matchColumn(cols, c.typed)
			if c.wantKey == "" {
				if got != nil {
					t.Fatalf("matchColumn(%q) = %q, want no match", c.typed, got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("matchColumn(%q) = nil, want the %q column", c.typed, c.wantKey)
			}
			if got.ID != byKey[c.wantKey] {
				t.Fatalf("matchColumn(%q) = %q, want the %q column", c.typed, got.Name, c.wantKey)
			}
		})
	}
}

// The caption table must never outrank a column somebody actually named. A board
// whose own column is called "Done" has that name in the row with no name_key
// (UpdateColumn clears it), and `/move Done` must land there rather than on the
// seeded «Готово» the same table also spells "Done".
func TestMatchColumnPrefersStoredName(t *testing.T) {
	cols := seededCols(t)
	own := db.BoardColumn{ID: uuid.New(), Name: "Done"}
	cols = append(cols, own)

	got := matchColumn(cols, "Done")
	if got == nil || got.ID != own.ID {
		t.Fatalf("matchColumn(\"Done\") = %v, want the user's own column", got)
	}
}

// A column the user renamed has no name_key, so the caption table must not reach
// it: "Done" on this board means nothing, even though a seeded done column would
// have answered to it.
func TestMatchColumnIgnoresKeylessColumns(t *testing.T) {
	cols := []db.BoardColumn{{ID: uuid.New(), Name: "Готово"}}
	if got := matchColumn(cols, "Done"); got != nil {
		t.Fatalf("matchColumn(\"Done\") = %q, want no match on a keyless column", got.Name)
	}
}

// The Russian arm of the caption table is the same data as defaultColumns, and
// they are edited in different places — pin them together so a rename on one
// side cannot silently break `/move` for the other.
func TestSeededColumnCaptionsMatchDefaults(t *testing.T) {
	for _, dc := range defaultColumns {
		if got := seededColumnKey(dc.name); got != dc.nameKey {
			t.Fatalf("seededColumnKey(%q) = %q, want %q", dc.name, got, dc.nameKey)
		}
	}
}

// Adding a language to i18n.Supported without an arm here would leave its
// readers typing captions `/move` has never heard of — the same silent gap this
// test's subject was written to close.
func TestSeededColumnCaptionsCoverSupportedLanguages(t *testing.T) {
	for _, lang := range i18n.Supported {
		arm, ok := seededColumnCaptions[lang]
		if !ok {
			t.Fatalf("no seeded column captions for %q", lang)
		}
		for _, dc := range defaultColumns {
			caption, ok := arm[dc.nameKey]
			if !ok || strings.TrimSpace(caption) == "" {
				t.Fatalf("%s: no caption for the %q column", lang, dc.nameKey)
			}
			if got := seededColumnKey(caption); got != dc.nameKey {
				t.Fatalf("seededColumnKey(%q) = %q, want %q (%s)", caption, got, dc.nameKey, lang)
			}
		}
	}
}
