package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Notification content lives here: every notification carries both the legacy
// Russian sentence (`notifications.text`) and the structured facts the client
// renders from (`notifications.payload`, added in migration 0065, task #2801).
//
// The rule this file exists to enforce: the server states *what happened*, the
// client decides *how to say it*. A sentence assembled here can only ever be
// Russian — it is written to `text` for old clients (Android until stage 7 of
// #2796) and for rows that predate the payload, and for nothing else.
//
// Adding a notification kind means adding a builder here and the matching
// `notifications.event.<event>` key to both locale bundles; the frontend spec
// tests/cx-notifications.spec.js fails on an event without a key.

// Payload event names. These are render keys on the client
// (`notifications.event.<event>`), so they are stable identifiers — renaming one
// silently degrades old rows to the `text` fallback.
const (
	evAssigned      = "task_assigned"
	evUpdated       = "task_updated"
	evMoved         = "task_moved"
	evArchived      = "task_archived"
	evComment       = "task_comment"
	evThreadReply   = "task_thread_reply"
	evMention       = "task_mention"
	evDueSoon       = "task_due_soon"
	evReminder      = "reminder"
	evSyncOK      = "integration_sync_ok"
	evSyncPartial = "integration_sync_partial"
	evSyncFailed  = "integration_sync_failed"
)

// notifyMsg is one notification's content: the legacy pre-rendered sentence and
// the structured payload. Build one with the msg* helpers below — never write a
// payload inline at a call site, or the event names drift out of this file.
type notifyMsg struct {
	text    string
	payload map[string]any
}

// json renders the payload for storage. An empty payload becomes `{}` — the
// column is NOT NULL, and the client reads "no keys" as "use the text".
func (m notifyMsg) json() json.RawMessage {
	if len(m.payload) == 0 {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(m.payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// event builds the common payload skeleton: the render key plus the task the
// notification points at (absent for workspace-level events like a sync report).
func event(name string, taskNumber *int64) map[string]any {
	p := map[string]any{"event": name}
	if taskNumber != nil {
		p["task_number"] = *taskNumber
	}
	return p
}

// withCtx adds a short piece of content (a task title, a comment excerpt) under
// key, but only when it is short enough to inline — mirroring shortCtx, so the
// rendered sentence carries the same context in both languages.
func withCtx(p map[string]any, key, s string) map[string]any {
	if v := shortValue(s); v != "" {
		p[key] = v
	}
	return p
}

// shortValue is shortCtx without the « » decoration: the inlined content itself,
// or "" when it is too long to inline.
func shortValue(s string) string {
	if shortCtx(s) == "" {
		return ""
	}
	return strings.TrimSpace(s)
}

func msgAssigned(actor string, number *int64, title string) notifyMsg {
	p := withCtx(event(evAssigned, number), "title", title)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s назначил вам задачу #%s%s", actor, taskRef(number), shortCtx(title)),
		payload: p,
	}
}

// msgUpdated summarises a task edit. fields are the machine names returned by
// journalUpdate ("title", "due", …); the Russian text maps them through
// fieldWords, the client through its own `notifications.field.*` keys.
func msgUpdated(actor string, number *int64, fields []string) notifyMsg {
	p := event(evUpdated, number)
	p["actor"] = actor
	p["fields"] = fields
	words := make([]string, 0, len(fields))
	for _, f := range fields {
		if w, ok := fieldWords[f]; ok {
			words = append(words, w)
		}
	}
	return notifyMsg{
		text: fmt.Sprintf("%s изменил(а) задачу #%s: %s",
			actor, taskRef(number), strings.Join(words, ", ")),
		payload: p,
	}
}

// fieldWords renders a changed field name in Russian for the legacy text. Keep
// in sync with journalUpdate's field names and the client's notifications.field.*.
var fieldWords = map[string]string{
	"title":       "название",
	"description": "описание",
	"priority":    "приоритет",
	"due":         "срок",
	"start":       "начало",
	"estimate":    "оценка",
	"completed":   "выполнена",
	"reopened":    "возвращена в работу",
}

func msgMoved(actor string, number *int64, column string) notifyMsg {
	p := event(evMoved, number)
	p["actor"] = actor
	// Column names are user data (and the GitLab mapping keys off them), so this
	// one is carried verbatim, not translated.
	p["column"] = column
	return notifyMsg{
		text:    fmt.Sprintf("%s переместил(а) задачу #%s → «%s»", actor, taskRef(number), column),
		payload: p,
	}
}

func msgArchived(actor string, number *int64, title string) notifyMsg {
	p := withCtx(event(evArchived, number), "title", title)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s архивировал(а) задачу #%s%s", actor, taskRef(number), shortCtx(title)),
		payload: p,
	}
}

// msgComment / msgThreadReply / msgMention share the "comment" kind but differ in
// wording, so each gets its own event: `kind` drives routing rules, `event`
// drives the sentence.
func msgComment(actor string, number *int64, excerpt string) notifyMsg {
	p := withCtx(event(evComment, number), "excerpt", excerpt)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s прокомментировал #%s%s", actor, taskRef(number), shortCtx(excerpt)),
		payload: p,
	}
}

func msgThreadReply(actor string, number *int64, excerpt string) notifyMsg {
	p := withCtx(event(evThreadReply, number), "excerpt", excerpt)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s ответил(а) в обсуждении #%s%s", actor, taskRef(number), shortCtx(excerpt)),
		payload: p,
	}
}

func msgMention(actor string, number *int64, excerpt string) notifyMsg {
	p := withCtx(event(evMention, number), "excerpt", excerpt)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s упомянул(а) вас в #%s%s", actor, taskRef(number), shortCtx(excerpt)),
		payload: p,
	}
}

// msgDueSoon has no actor (the scanner raises it) and always inlines the title:
// without it the notification says nothing but a number.
func msgDueSoon(number *int64, title string) notifyMsg {
	p := event(evDueSoon, number)
	p["title"] = title
	return notifyMsg{
		text:    fmt.Sprintf("Приближается срок задачи #%s «%s»", taskRef(number), title),
		payload: p,
	}
}

// msgReminder carries the user's own reminder text, which is content and stays
// untranslated; only the empty-message default is a UI string.
func msgReminder(message string) notifyMsg {
	message = strings.TrimSpace(message)
	p := event(evReminder, nil)
	if message != "" {
		p["message"] = message
	}
	text := message
	if text == "" {
		text = "Напоминание"
	}
	return notifyMsg{text: text, payload: p}
}

// syncCounts are the facts of a finished integration sync, shared by the three
// outcome events.
type syncCounts struct {
	label   string
	created int
	updated int
	took    string // duration already rendered in Russian, for the legacy text
	seconds int    // the same duration as a number, for the client to render
	reason  string // failure reason; only set for the failed outcome
}

func msgSyncFailed(s syncCounts) notifyMsg {
	reason := s.reason
	if reason == "" {
		reason = "неизвестная ошибка"
	}
	p := event(evSyncFailed, nil)
	p["label"] = s.label
	p["seconds"] = s.seconds
	p["reason"] = reason
	return notifyMsg{
		text:    fmt.Sprintf("%s: синхронизация не удалась — %s (за %s)", s.label, reason, s.took),
		payload: p,
	}
}

func msgSyncDone(s syncCounts, partial bool) notifyMsg {
	name := evSyncOK
	text := fmt.Sprintf("%s: +%d новых, ~%d обновлено, за %s", s.label, s.created, s.updated, s.took)
	if partial {
		name = evSyncPartial
		text = fmt.Sprintf("%s: +%d новых, ~%d обновлено, за %s (часть действий с ошибками)",
			s.label, s.created, s.updated, s.took)
	}
	p := event(name, nil)
	p["label"] = s.label
	p["created"] = s.created
	p["updated"] = s.updated
	p["seconds"] = s.seconds
	return notifyMsg{text: text, payload: p}
}
