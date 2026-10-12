package notify

import (
	"strings"
	"testing"
	"unicode"

	"tessera/internal/i18n"
)

// SampleData feeds the two places a user meets a notification before any real one
// exists: the channel editor's live preview and the "send a test" probe. Both
// stand in for a real delivery, and a real delivery is written in the recipient's
// language (handlers.templateData → Title/Sentence/DefaultName). So the sample has
// to follow the same rule — otherwise an English reader previews a Russian
// message, receives a Russian test, and only then starts getting English ones.

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func TestSampleDataFollowsTheReader(t *testing.T) {
	en := SampleData("en")
	if want := Title(en.Kind, "en"); en.Title != want {
		t.Errorf("en Title = %q, want the catalog's %q", en.Title, want)
	}
	for name, got := range map[string]string{
		"Text": en.Text, "TaskTitle": en.TaskTitle, "Actor": en.Actor, "Workspace": en.Workspace,
		"Title": en.Title,
	} {
		if got == "" {
			t.Errorf("en %s is empty — a preview must show every documented field", name)
		}
		if hasCyrillic(got) {
			t.Errorf("en %s = %q — Russian leaked into an English preview", name, got)
		}
	}
	// The sample sentence is built by the same catalog a real delivery uses, so
	// the preview shows the exact phrasing the channel will carry.
	wantText := Sentence(map[string]any{
		"event": "task_assigned", "actor": en.Actor,
		"task_number": float64(42), "title": en.TaskTitle,
	}, "en", "")
	if en.Text != wantText {
		t.Errorf("en Text = %q\nwant the catalog sentence %q", en.Text, wantText)
	}
	// The personal workspace is a server-seeded name with a key (#2800) — captioned
	// for the reader here, exactly as templateData does it.
	if want := DefaultName("personal", "Личное пространство", "en"); en.Workspace != want {
		t.Errorf("en Workspace = %q, want %q", en.Workspace, want)
	}

	ru := SampleData("ru")
	if !hasCyrillic(ru.Text) || !hasCyrillic(ru.Workspace) {
		t.Errorf("ru sample is not Russian: %+v", ru)
	}
	// Unknown tag falls back to the default bundle, not to an empty sample.
	// (Compared field-wise: TaskNumber is a pointer, fresh on every call.)
	de := SampleData("de")
	if de.Title != ru.Title || de.Text != ru.Text || de.Actor != ru.Actor ||
		de.TaskTitle != ru.TaskTitle || de.Workspace != ru.Workspace {
		t.Errorf("SampleData(de) = %+v, want the %s sample %+v", de, i18n.Default, ru)
	}
}

// Every supported language must produce a sample that renders through the default
// template with nothing left unresolved — that is what the editor shows before the
// user types a template of their own.
func TestSampleDataRendersInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Supported {
		out, err := Render(DefaultTemplate, SampleData(lang))
		if err != nil {
			t.Errorf("%s: render: %v", lang, err)
			continue
		}
		if strings.TrimSpace(out) == "" || strings.Contains(out, "{") {
			t.Errorf("%s: default template rendered %q", lang, out)
		}
		// A language added to the catalog without its sample strings would leave
		// the editor's field hints demonstrating blanks.
		d := SampleData(lang)
		for name, got := range map[string]string{
			"Title": d.Title, "Text": d.Text, "TaskTitle": d.TaskTitle,
			"Actor": d.Actor, "Workspace": d.Workspace,
		} {
			if got == "" {
				t.Errorf("%s: sample %s is empty", lang, name)
			}
		}
	}
}

// The probe's own subject line is a notification kind like any other, so it comes
// from the catalog instead of a literal at the call site.
func TestChannelTestKindIsInTheCatalog(t *testing.T) {
	if got, want := Title("test", "en"), "Channel test"; got != want {
		t.Errorf("Title(test, en) = %q, want %q", got, want)
	}
	if got, want := Title("test", "ru"), "Проверка канала"; got != want {
		t.Errorf("Title(test, ru) = %q, want %q", got, want)
	}
	// Negative control: a kind the catalog doesn't know still falls back.
	if got, want := Title("no_such_kind", "en"), "Notification"; got != want {
		t.Errorf("unknown kind: got %q, want %q", got, want)
	}
}
