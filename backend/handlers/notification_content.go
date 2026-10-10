package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
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
	evAssigned    = "task_assigned"
	evUpdated     = "task_updated"
	evMoved       = "task_moved"
	evArchived    = "task_archived"
	evDeleted     = "task_deleted"
	evUnassigned  = "task_unassigned"
	evCompleted   = "task_completed"
	evReopened    = "task_reopened"
	evComment     = "task_comment"
	evThreadReply = "task_thread_reply"
	evMention     = "task_mention"
	evDueSoon     = "task_due_soon"
	evReminder    = "reminder"
	evSyncOK      = "integration_sync_ok"
	evSyncPartial = "integration_sync_partial"
	evSyncFailed  = "integration_sync_failed"
	evConfInvited = "conference_invited"
	evConfMention = "conference_mention"
)

// kindConferenceInvite is the routing kind of a conference invitation. Unlike
// the task kinds it points at no task at all — the thing to open is the call,
// and its id travels in the payload (see msgConferenceInvited).
const kindConferenceInvite = "conference_invite"

// The routing kinds of the two notifications the user has a switch for:
// "уведомления о сроке" (also a per-task override) and "напоминания". Stored on
// the notification row and read by timedKinds and deliveryStillWanted. Note these
// are the stored kinds, not the payload event names above — evDueSoon is
// "task_due_soon" while the row says "due_soon".
const (
	kindDueSoon  = "due_soon"
	kindReminder = "reminder"
)

// The routing kinds of what was done to a task the reader takes part in (#3114),
// beside the older assigned/updated/moved/archived. kindDeleted is the odd one:
// its task is gone by the time anyone reads the row, so the row carries no
// task_id (the foreign key would cascade it away) and the payload says which
// task it was — see msgDeleted.
const (
	kindDeleted    = "deleted"
	kindUnassigned = "unassigned"
	kindCompleted  = "completed"
	kindReopened   = "reopened"
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

func msgMoved(actor string, number *int64, column string, columnKey *string) notifyMsg {
	p := event(evMoved, number)
	p["actor"] = actor
	// A column the user named is user data (and the GitLab mapping keys off that
	// name), so it is carried verbatim. The four columns seeded with a board are
	// not: they carry a name_key (#2800) and every client captions them from it,
	// so the key rides along or the bell would say «Готово» about the column the
	// same screen labels "Done".
	p["column"] = column
	if columnKey != nil && *columnKey != "" {
		p["column_key"] = *columnKey
	}
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

// msgDeleted announces that a task the reader took part in was deleted. The task
// is gone by the time the row is read: notifications.task_id is a foreign key
// with ON DELETE CASCADE, so the row is written with no task at all, and
// everything a reader needs to recognise the task travels in the payload — the
// number, the whole title (always, like msgDueSoon: a bare number of a task that
// no longer exists says nothing) and the board it stood on (`board_id`, for a
// client that wants to say where; there is nothing left to open).
func msgDeleted(actor string, number *int64, title string, boardID uuid.UUID) notifyMsg {
	title = strings.TrimSpace(title)
	p := event(evDeleted, number)
	p["actor"] = actor
	p["title"] = title
	p["board_id"] = boardID.String()
	return notifyMsg{
		text:    fmt.Sprintf("%s удалил(а) задачу #%s «%s»", actor, taskRef(number), title),
		payload: p,
	}
}

// msgUnassigned is msgAssigned's mirror: the reader was taken off a task.
func msgUnassigned(actor string, number *int64, title string) notifyMsg {
	p := withCtx(event(evUnassigned, number), "title", title)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s снял(а) вас с задачи #%s%s", actor, taskRef(number), shortCtx(title)),
		payload: p,
	}
}

// msgCompleted / msgReopened report a task entering or leaving the completed
// state — by the checkbox or by a drag into or out of the board's done column.
// They stand in for task_updated / task_moved for that change: «X выполнил(а)
// задачу» is what the reader wants to hear, not which column it landed in.
func msgCompleted(actor string, number *int64, title string) notifyMsg {
	p := withCtx(event(evCompleted, number), "title", title)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s выполнил(а) задачу #%s%s", actor, taskRef(number), shortCtx(title)),
		payload: p,
	}
}

func msgReopened(actor string, number *int64, title string) notifyMsg {
	p := withCtx(event(evReopened, number), "title", title)
	p["actor"] = actor
	return notifyMsg{
		text:    fmt.Sprintf("%s вернул(а) в работу задачу #%s%s", actor, taskRef(number), shortCtx(title)),
		payload: p,
	}
}

// msgComment / msgThreadReply / msgMention share the "comment" kind but differ in
// wording, so each gets its own event: `kind` drives routing rules, `event`
// drives the sentence.
func msgComment(actor string, number *int64, excerpt string) notifyMsg {
	p := withCtx(event(evComment, number), "excerpt", excerpt)
	p["actor"] = actor
	withQuote(p, excerpt)
	return notifyMsg{
		text:    fmt.Sprintf("%s прокомментировал #%s%s", actor, taskRef(number), shortCtx(excerpt)),
		payload: p,
	}
}

func msgThreadReply(actor string, number *int64, excerpt string) notifyMsg {
	p := withCtx(event(evThreadReply, number), "excerpt", excerpt)
	p["actor"] = actor
	withQuote(p, excerpt)
	return notifyMsg{
		text:    fmt.Sprintf("%s ответил(а) в обсуждении #%s%s", actor, taskRef(number), shortCtx(excerpt)),
		payload: p,
	}
}

func msgMention(actor string, number *int64, excerpt string) notifyMsg {
	p := withCtx(event(evMention, number), "excerpt", excerpt)
	p["actor"] = actor
	withQuote(p, excerpt)
	return notifyMsg{
		text:    fmt.Sprintf("%s упомянул(а) вас в #%s%s", actor, taskRef(number), shortCtx(excerpt)),
		payload: p,
	}
}

// quoteLimit bounds the comment quote a notification carries for the in-app
// toast (#3117): long enough to recognise the comment, short enough for one line.
const quoteLimit = 120

// withQuote adds the comment as the toast quotes it — whitespace collapsed,
// cut at quoteLimit runes with an ellipsis. `excerpt` stays the short inline
// context the sentence carries; this is only drawn under it.
func withQuote(p map[string]any, s string) {
	q := strings.Join(strings.Fields(s), " ")
	if q == "" {
		return
	}
	if r := []rune(q); len(r) > quoteLimit {
		q = strings.TrimSpace(string(r[:quoteLimit])) + "…"
	}
	p["quote"] = q
}

// msgConferenceInvited announces an invitation to a call. It is the first event
// that points at something other than a task, so it carries `conference_id`
// instead of `task_number`: that id is what the bell feed, the desktop deep link
// and the push payload all navigate by. The title rides in the same `title` key
// as everywhere else, so the client's `{ctx}` inlining works unchanged.
func msgConferenceInvited(actor, title, confID string) notifyMsg {
	p := withCtx(event(evConfInvited, nil), "title", title)
	p["actor"] = actor
	p["conference_id"] = confID
	return notifyMsg{
		text:    fmt.Sprintf("%s приглашает вас в конференцию%s", actor, shortCtx(title)),
		payload: p,
	}
}

// msgConferenceMention is an @-mention in a call's chat (#3055). It rides the
// "mention" routing kind — a mention is a mention to the reader's switches — but
// points at the call the way an invitation does: `conference_id` instead of a
// task number, the call's title as `title`, the line itself as the quote.
func msgConferenceMention(actor, title, confID, body string) notifyMsg {
	p := withCtx(event(evConfMention, nil), "title", title)
	p["actor"] = actor
	p["conference_id"] = confID
	withQuote(p, body)
	return notifyMsg{
		text:    fmt.Sprintf("%s упомянул(а) вас в чате конференции%s", actor, shortCtx(title)),
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

// msgSyncFailed reports a failed sync. The reason is the integration's own error
// text, which is content and travels verbatim; the "we don't know why" default is
// a UI string, so — like msgReminder's empty-message default — it stays in the
// legacy text and is left out of the payload. The client splices `reason` into a
// sentence of its own language, and a Russian default in there would read
// "sync failed — неизвестная ошибка" on an English screen.
func msgSyncFailed(s syncCounts) notifyMsg {
	reason := strings.TrimSpace(s.reason)
	p := event(evSyncFailed, nil)
	p["label"] = s.label
	p["seconds"] = s.seconds
	if reason != "" {
		p["reason"] = reason
	}
	text := reason
	if text == "" {
		text = "неизвестная ошибка"
	}
	return notifyMsg{
		text:    fmt.Sprintf("%s: синхронизация не удалась — %s (за %s)", s.label, text, s.took),
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
