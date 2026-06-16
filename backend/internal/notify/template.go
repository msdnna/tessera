package notify

import (
	"strings"
	"text/template"
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

// SampleData is representative data for previewing/testing a template before any
// real notification exists.
func SampleData() TemplateData {
	n := int64(42)
	return TemplateData{
		Kind:       "assigned",
		Title:      "Назначена задача",
		Text:       "Алиса назначил вам задачу #42 «Починить чайник»",
		TaskNumber: &n,
		TaskTitle:  "Починить чайник",
		Actor:      "Алиса",
		Workspace:  "Личное пространство",
		Link:       "https://tessera.example/",
	}
}
