package notify

import (
	"strings"
	"text/template"

	"tessera/internal/i18n"
)

// TemplateData is the data a channel's message template renders against. It's the
// stable, documented surface for template authors — keep field names stable, and
// when adding one, surface it in the editor's field hints too.
type TemplateData struct {
	Kind       string // assigned | comment | mention | due_soon | test
	Title      string // default subject line for the kind (no app-name prefix)
	Text       string // the pre-composed notification sentence
	TaskNumber *int64 // task #, when the notification is about a task
	TaskTitle  string
	Actor      string // display name of who triggered it
	Workspace  string // workspace name
	Link       string // app URL
}

// DefaultTemplate is used when a channel has no custom template: the composed
// sentence followed by the app link. Custom templates override the whole message.
const DefaultTemplate = "{{.Text}}{{with .Link}}\n{{.}}{{end}}"

// Render executes a Go text/template against the data. text (not html) — these
// are plain messages, and the author owns their own notifications. Used for both
// delivery and the live editor preview; an unknown field or parse error surfaces
// here so the preview can show it.
func Render(tmpl string, data TemplateData) (string, error) {
	t, err := template.New("msg").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// sampleKind / sampleTaskNumber are the stand-in notification every preview and
// test probe is built from: an "assigned" event on task #42.
const (
	sampleKind       = "assigned"
	sampleTaskNumber = int64(42)
)

// SampleData is representative data for previewing/testing a template before any
// real notification exists — the channel editor's live preview and the "send a
// test" probe both render against it.
//
// It is written in lang for the same reason a real delivery is (locale.go): both
// are rehearsals of a message that leaves the app, and a rehearsal in the wrong
// language teaches the wrong thing — an English reader would preview a Russian
// message, receive a Russian test, and only then start getting English ones.
// Every field is taken from the same catalog handlers.templateData draws on, so
// the rehearsal reads exactly like the delivery.
func SampleData(lang string) TemplateData {
	b := bundleFor(lang)
	n := sampleTaskNumber
	return TemplateData{
		Kind:  sampleKind,
		Title: Title(sampleKind, lang),
		Text: Sentence(map[string]any{
			"event":       "task_assigned",
			"actor":       b.sampleActor,
			"task_number": n,
			"title":       b.sampleTaskTitle,
		}, lang, ""),
		TaskNumber: &n,
		TaskTitle:  b.sampleTaskTitle,
		Actor:      b.sampleActor,
		// The personal workspace is a server-seeded name carrying a key (#2800),
		// captioned for the reader exactly as a real delivery captions it.
		Workspace: DefaultName("personal", bundles[i18n.Default].defaultNames["personal"], lang),
		Link:      "https://tessera.example/",
	}
}
