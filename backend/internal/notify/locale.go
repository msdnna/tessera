package notify

import (
	"encoding/json"
	"strconv"
	"strings"

	"tessera/internal/i18n"
)

// Localized delivery text (stage 6 of #2796).
//
// In-app the client renders a notification from its structured payload (#2801)
// and this file is never touched. It exists for the deliveries that leave the
// app — email, FCM push, telegram/webhook channels — where there is no client to
// do the rendering, so the server has to say the sentence itself, in the
// *recipient's* language (their stored preference, not the language of whoever
// triggered the event).
//
// This catalog is the twin of frontend/src/locales/<lang>/notifications.json:
// same event keys, same placeholders, same wording. When one side changes an
// event's phrasing or adds a key, change the other in the same commit.

// bundle is one language's wording. Every events/fields/kinds map must carry the
// same key set across languages — locale_test.go walks the ru bundle and demands
// each key exists in every other one.
type bundle struct {
	ctx          string // wrapper for an inlined title/excerpt, holds {text}
	events       map[string]string
	fields       map[string]string
	kinds        map[string]string
	defaultNames map[string]string // captions for server-seeded names, by name_key (#2800)
	fallbackKind string            // subject for a kind this bundle doesn't know
	listSep      string            // how a list of changed fields is joined
	listLast     string            // the conjunction before its last item — see joinList
	digestTitle  string
	digestHeader string // holds {count} and {noun}
	// digestNoun is the noun {noun} stands for, in the three CLDR forms
	// i18n.Plural chooses between (one · few · many). The header carries a count,
	// so the noun has to agree with it: a digest combines two rows or more, which
	// in Russian is the "few" form far more often than not.
	digestNoun [3]string
	instant    string // duration under a second
	durationHM string // {h} {m}
	durationMS string // {m} {s}
	durationS  string // {s}
	// deviceLabel names a client's auto-registered device channel when the client
	// didn't send a name of its own.
	deviceLabel string
	// unknownReason stands in for a sync failure that names no cause. The reason
	// itself is the integration's own error text — content, carried verbatim — so
	// the producer leaves the key out rather than defaulting it in the sender's
	// language (handlers.msgSyncFailed, the same rule as an empty reminder). That
	// makes the default this side's business, and without it the sentence breaks
	// off at the dash.
	unknownReason string
	// sampleActor/sampleTaskTitle furnish the stand-in notification the channel
	// editor previews and the "send a test" probe delivers (SampleData). They live
	// here, not at the call site, for the same reason everything else in this file
	// does: those two are rehearsals of a message that leaves the app, so they have
	// to be written in the reader's language or they rehearse the wrong thing.
	sampleActor     string
	sampleTaskTitle string
}

var bundles = map[string]bundle{
	"ru": {
		ctx: " «{text}»",
		events: map[string]string{
			"task_assigned":            "{actor} назначил вам задачу #{ref}{ctx}",
			"task_updated":             "{actor} изменил(а) задачу #{ref}: {fields}",
			"task_moved":               "{actor} переместил(а) задачу #{ref} → «{column}»",
			"task_archived":            "{actor} архивировал(а) задачу #{ref}{ctx}",
			"task_deleted":             "{actor} удалил(а) задачу #{ref}{ctx}",
			"task_unassigned":          "{actor} снял(а) вас с задачи #{ref}{ctx}",
			"task_completed":           "{actor} выполнил(а) задачу #{ref}{ctx}",
			"task_reopened":            "{actor} вернул(а) в работу задачу #{ref}{ctx}",
			"task_comment":             "{actor} прокомментировал #{ref}{ctx}",
			"task_thread_reply":        "{actor} ответил(а) в обсуждении #{ref}{ctx}",
			"task_mention":             "{actor} упомянул(а) вас в #{ref}{ctx}",
			"task_due_soon":            "Приближается срок задачи #{ref} «{title}»",
			"reminder":                 "Напоминание",
			"integration_sync_ok":      "{label}: +{created} новых, ~{updated} обновлено, за {took}",
			"integration_sync_partial": "{label}: +{created} новых, ~{updated} обновлено, за {took} (часть действий с ошибками)",
			"integration_sync_failed":  "{label}: синхронизация не удалась — {reason} (за {took})",
			"conference_invited":       "{actor} приглашает вас в конференцию{ctx}",
			"note_mention":             "{actor} упомянул(а) вас в заметке{ctx}",
			// v2 notifications (#3048).
			"milestone_closed":     "{actor} закрыл(а) этап «{title}» — выполнено {done} из {total}",
			"conference_starting":  "Через {minutes} мин начнётся встреча «{title}»",
			"integration_conflict": "{label} · конфликт синхронизации в #{ref} ({fields}): изменено в обоих местах",
			"document_mention":     "{actor} упомянул(а) вас в документе «{name}»{ctx}",
			"conference_mention":   "{actor} упомянул(а) вас в чате встречи «{name}»{ctx}",
		},
		fields: map[string]string{
			"title":       "название",
			"description": "описание",
			"priority":    "приоритет",
			"due":         "срок",
			"start":       "начало",
			"estimate":    "оценка",
			"completed":   "выполнена",
			"reopened":    "возвращена в работу",
			"state":       "статус",
		},
		defaultNames: map[string]string{
			"personal":    "Личное пространство",
			"todo":        "К работе",
			"in_progress": "В процессе",
			"review":      "На рассмотрении",
			"done":        "Готово",
		},
		kinds: map[string]string{
			"assigned":          "Назначена задача",
			"comment":           "Новый комментарий",
			"mention":           "Вас упомянули",
			"updated":           "Задача изменена",
			"moved":             "Задача перемещена",
			"archived":          "Задача архивирована",
			"deleted":           "Задача удалена",
			"unassigned":        "Вас сняли с задачи",
			"completed":         "Задача выполнена",
			"reopened":          "Задача возвращена в работу",
			"due_soon":          "Скоро дедлайн",
			"reminder":          "Напоминание",
			"integration_sync":  "Синхронизация завершена",
			"conference_invite": "Приглашение в конференцию",
			// v2 notifications (#3048).
			"milestone_closed":     "Этап закрыт",
			"conference_soon":      "Скоро встреча",
			"integration_conflict": "Конфликт синхронизации",
			// Not a stored notification: the subject of the probe a user sends to
			// check a channel they just set up.
			"test": "Проверка канала",
		},
		fallbackKind:    "Уведомление",
		listSep:         ", ",
		listLast:        " и ",
		digestTitle:     "Сводка уведомлений",
		digestHeader:    "Сводка — {count} {noun}:",
		digestNoun:      [3]string{"уведомление", "уведомления", "уведомлений"},
		instant:         "меньше секунды",
		durationHM:      "{h} ч {m} м",
		durationMS:      "{m} м {s} с",
		durationS:       "{s} с",
		deviceLabel:     "Устройство",
		unknownReason:   "неизвестная ошибка",
		sampleActor:     "Алиса",
		sampleTaskTitle: "Починить чайник",
	},
	"en": {
		ctx: " “{text}”",
		events: map[string]string{
			"task_assigned":            "{actor} assigned task #{ref} to you{ctx}",
			"task_updated":             "{actor} updated task #{ref}: {fields}",
			"task_moved":               "{actor} moved task #{ref} → “{column}”",
			"task_archived":            "{actor} archived task #{ref}{ctx}",
			"task_deleted":             "{actor} deleted task #{ref}{ctx}",
			"task_unassigned":          "{actor} unassigned you from task #{ref}{ctx}",
			"task_completed":           "{actor} completed task #{ref}{ctx}",
			"task_reopened":            "{actor} reopened task #{ref}{ctx}",
			"task_comment":             "{actor} commented on #{ref}{ctx}",
			"task_thread_reply":        "{actor} replied in the thread on #{ref}{ctx}",
			"task_mention":             "{actor} mentioned you in #{ref}{ctx}",
			"task_due_soon":            "Task #{ref} “{title}” is due soon",
			"reminder":                 "Reminder",
			"integration_sync_ok":      "{label}: +{created} new, ~{updated} updated, in {took}",
			"integration_sync_partial": "{label}: +{created} new, ~{updated} updated, in {took} (some actions failed)",
			"integration_sync_failed":  "{label}: sync failed — {reason} (after {took})",
			"conference_invited":       "{actor} invited you to a conference{ctx}",
			"note_mention":             "{actor} mentioned you in a note{ctx}",
			// v2 notifications (#3048).
			"milestone_closed":     "{actor} closed stage “{title}” — {done} of {total} done",
			"conference_starting":  "Meeting “{title}” starts in {minutes} min",
			"integration_conflict": "{label} · sync conflict on #{ref} ({fields}): changed on both sides",
			"document_mention":     "{actor} mentioned you in document “{name}”{ctx}",
			"conference_mention":   "{actor} mentioned you in the chat of “{name}”{ctx}",
		},
		fields: map[string]string{
			"title":       "title",
			"description": "description",
			"priority":    "priority",
			"due":         "due date",
			"start":       "start date",
			"estimate":    "estimate",
			"completed":   "completed",
			"reopened":    "reopened",
			"state":       "status",
		},
		defaultNames: map[string]string{
			"personal":    "Personal space",
			"todo":        "To do",
			"in_progress": "In progress",
			"review":      "In review",
			"done":        "Done",
		},
		kinds: map[string]string{
			"assigned":          "Task assigned",
			"comment":           "New comment",
			"mention":           "You were mentioned",
			"updated":           "Task updated",
			"moved":             "Task moved",
			"archived":          "Task archived",
			"deleted":           "Task deleted",
			"unassigned":        "Unassigned from a task",
			"completed":         "Task completed",
			"reopened":          "Task reopened",
			"due_soon":          "Deadline approaching",
			"reminder":          "Reminder",
			"integration_sync":  "Sync finished",
			"conference_invite": "Conference invitation",
			// v2 notifications (#3048).
			"milestone_closed":     "Stage closed",
			"conference_soon":      "Meeting starting soon",
			"integration_conflict": "Sync conflict",
			"test":                 "Channel test",
		},
		fallbackKind: "Notification",
		listSep:      ", ",
		// No Oxford comma: the web client's locale for English is en-GB (see
		// utils/format.js localeTag), and that is what Intl.ListFormat words there.
		listLast:        " and ",
		digestTitle:     "Notification digest",
		digestHeader:    "Digest — {count} {noun}:",
		digestNoun:      [3]string{"notification", "notifications", "notifications"},
		instant:         "less than a second",
		durationHM:      "{h}h {m}m",
		durationMS:      "{m}m {s}s",
		durationS:       "{s}s",
		deviceLabel:     "Device",
		unknownReason:   "unknown error",
		sampleActor:     "Alice",
		sampleTaskTitle: "Fix the kettle",
	},
}

// fieldOrder is the order changed fields are listed in, so a task_updated
// sentence reads the same on every delivery (payload arrays keep their order,
// but an unknown name is dropped rather than shown raw).
var fieldOrder = []string{"title", "description", "priority", "due", "start", "estimate", "completed", "reopened", "state"}

func bundleFor(lang string) bundle { return bundles[i18n.Normalize(lang)] }

// Title is the subject line for a notification kind, in the reader's language.
func Title(kind, lang string) string {
	b := bundleFor(lang)
	if s, ok := b.kinds[kind]; ok {
		return s
	}
	return b.fallbackKind
}

// DefaultName captions a name the server seeded and stored in Russian — the
// personal workspace and the four columns every board is seeded with (#2800). In
// a client the caption is drawn from name_key by the client itself; a delivery
// that leaves the app has no client, so it is drawn here, in the recipient's
// language.
//
// key is the row's name_key: empty for a name the user chose, and that name is
// carried verbatim in every language. A key this build doesn't know falls back to
// the stored string for the same reason Sentence does — a phrase beats a raw key.
func DefaultName(key, stored, lang string) string {
	if key == "" {
		return stored
	}
	if s, ok := bundleFor(lang).defaultNames[key]; ok {
		return s
	}
	return stored
}

// DeviceLabel names a device channel a client registered without a name of its
// own. It is stored, and the user may rename it afterwards — so this is the name
// they first see, written in their language rather than the server's.
func DeviceLabel(lang string) string { return bundleFor(lang).deviceLabel }

// DigestTitle is the subject of a combined digest delivery.
func DigestTitle(lang string) string { return bundleFor(lang).digestTitle }

// DigestHeader is the line that opens a digest, above the bulleted items. The
// noun agrees with count in the reader's language — a digest is by definition a
// plural delivery, and the count it opens with is the first thing read.
func DigestHeader(count int, lang string) string {
	b := bundleFor(lang)
	noun := i18n.Plural(lang, count, b.digestNoun[0], b.digestNoun[1], b.digestNoun[2])
	return strings.NewReplacer("{count}", strconv.Itoa(count), "{noun}", noun).Replace(b.digestHeader)
}

// Sentence renders a notification's payload into one line in lang. fallback is
// the server-composed Russian sentence stored on the row: it is returned as-is
// for a row written before the payload existed (empty payload) and for an event
// this build doesn't know — exactly the two cases the web client falls back on.
func Sentence(payload map[string]any, lang, fallback string) string {
	b := bundleFor(lang)
	event, _ := payload["event"].(string)
	tmpl, ok := b.events[event]
	if event == "" || !ok {
		return fallback
	}
	// A reminder's body is the user's own text — content, not UI — so it is
	// carried verbatim; only the empty-message default is translated.
	if event == "reminder" {
		if msg := str(payload["message"]); msg != "" {
			return msg
		}
		return tmpl
	}
	// A failure that names no cause carries no `reason` key at all, so the stand-in
	// is worded here rather than travelling in the payload — see bundle.unknownReason.
	reason := strings.TrimSpace(str(payload["reason"]))
	if reason == "" {
		reason = b.unknownReason
	}
	inline := str(payload["title"])
	if inline == "" {
		inline = str(payload["excerpt"])
	}
	ctx := ""
	if inline != "" {
		ctx = strings.ReplaceAll(b.ctx, "{text}", inline)
	}
	return strings.NewReplacer(
		"{actor}", str(payload["actor"]),
		"{ref}", taskRef(payload["task_number"]),
		"{ctx}", ctx,
		// A column seeded with the board carries its name_key, so it reads in the
		// recipient's language like every other server-born name; a column the
		// user named has no key and is carried verbatim.
		"{column}", DefaultName(str(payload["column_key"]), str(payload["column"]), lang),
		"{title}", str(payload["title"]),
		"{label}", str(payload["label"]),
		"{reason}", reason,
		"{created}", strconv.Itoa(num(payload["created"])),
		"{updated}", strconv.Itoa(num(payload["updated"])),
		"{took}", duration(num(payload["seconds"]), b),
		"{fields}", fieldList(payload["fields"], b),
		// v2 notifications (#3048): the place a mention was made, a stage's
		// tally, the minutes left before a meeting.
		"{name}", str(payload["name"]),
		"{done}", strconv.Itoa(num(payload["done"])),
		"{total}", strconv.Itoa(num(payload["total"])),
		"{minutes}", strconv.Itoa(num(payload["minutes"])),
	).Replace(tmpl)
}

// fieldList words the changed fields of a task_updated payload. Unknown names
// (a server newer than this catalog) are dropped, not shown raw.
func fieldList(v any, b bundle) string {
	list, ok := v.([]any)
	if !ok {
		return ""
	}
	seen := map[string]bool{}
	for _, item := range list {
		seen[str(item)] = true
	}
	words := make([]string, 0, len(list))
	for _, f := range fieldOrder {
		if seen[f] {
			words = append(words, b.fields[f])
		}
	}
	return joinList(words, b)
}

// joinList words a list the way the reader's language does — commas between all
// but the last pair, a conjunction before the last item: «название, срок и
// приоритет», "title, due date and start date".
//
// A worded list, not a punctuated one, because the same notification reaches one
// reader through three renderers — the web bell feed, the phone's, and this
// catalogue for everything that leaves the app — and the web one has always joined
// it with Intl.ListFormat (utils/format.js formatList, conjunction/long). The two
// that used a bare comma made a single edit read two different ways depending on
// where it was delivered. English gets no Oxford comma: the web client's locale
// for it is en-GB (utils/format.js localeTag), which is what Intl words there.
func joinList(words []string, b bundle) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	default:
		return strings.Join(words[:len(words)-1], b.listSep) + b.listLast + words[len(words)-1]
	}
}

// duration words a sync duration from the number of seconds the server measured.
func duration(seconds int, b bundle) string {
	if seconds <= 0 {
		return b.instant
	}
	switch {
	case seconds >= 3600:
		return strings.NewReplacer(
			"{h}", strconv.Itoa(seconds/3600),
			"{m}", strconv.Itoa(seconds%3600/60),
		).Replace(b.durationHM)
	case seconds >= 60:
		return strings.NewReplacer(
			"{m}", strconv.Itoa(seconds/60),
			"{s}", strconv.Itoa(seconds%60),
		).Replace(b.durationMS)
	default:
		return strings.ReplaceAll(b.durationS, "{s}", strconv.Itoa(seconds))
	}
}

// taskRef renders the task number a notification points at; "?" when absent, as
// on the web (a sentence with a blank number reads as a bug).
func taskRef(v any) string {
	if v == nil {
		return "?"
	}
	if s := strconv.Itoa(num(v)); s != "0" {
		return s
	}
	return "?"
}

// str/num read a JSON-decoded payload value. Numbers arrive as float64 after a
// round-trip through the column and as int64 when built in-process, so both are
// accepted.
func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
