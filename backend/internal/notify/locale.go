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
	fallbackKind string // subject for a kind this bundle doesn't know
	listSep      string // how a list of changed fields is joined
	digestTitle  string
	digestHeader string // holds {count}
	instant      string // duration under a second
	durationHM   string // {h} {m}
	durationMS   string // {m} {s}
	durationS    string // {s}
}

var bundles = map[string]bundle{
	"ru": {
		ctx: " «{text}»",
		events: map[string]string{
			"task_assigned":            "{actor} назначил вам задачу #{ref}{ctx}",
			"task_updated":             "{actor} изменил(а) задачу #{ref}: {fields}",
			"task_moved":               "{actor} переместил(а) задачу #{ref} → «{column}»",
			"task_archived":            "{actor} архивировал(а) задачу #{ref}{ctx}",
			"task_comment":             "{actor} прокомментировал #{ref}{ctx}",
			"task_thread_reply":        "{actor} ответил(а) в обсуждении #{ref}{ctx}",
			"task_mention":             "{actor} упомянул(а) вас в #{ref}{ctx}",
			"task_due_soon":            "Приближается срок задачи #{ref} «{title}»",
			"reminder":                 "Напоминание",
			"integration_sync_ok":      "{label}: +{created} новых, ~{updated} обновлено, за {took}",
			"integration_sync_partial": "{label}: +{created} новых, ~{updated} обновлено, за {took} (часть действий с ошибками)",
			"integration_sync_failed":  "{label}: синхронизация не удалась — {reason} (за {took})",
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
		},
		defaultNames: map[string]string{
			"personal": "Личное пространство",
		},
		kinds: map[string]string{
			"assigned":         "Назначена задача",
			"comment":          "Новый комментарий",
			"mention":          "Вас упомянули",
			"updated":          "Задача изменена",
			"moved":            "Задача перемещена",
			"archived":         "Задача архивирована",
			"due_soon":         "Скоро дедлайн",
			"reminder":         "Напоминание",
			"integration_sync": "Синхронизация завершена",
		},
		fallbackKind: "Уведомление",
		listSep:      ", ",
		digestTitle:  "Сводка уведомлений",
		digestHeader: "Сводка — {count} уведомлений:",
		instant:      "меньше секунды",
		durationHM:   "{h} ч {m} м",
		durationMS:   "{m} м {s} с",
		durationS:    "{s} с",
	},
	"en": {
		ctx: " “{text}”",
		events: map[string]string{
			"task_assigned":            "{actor} assigned task #{ref} to you{ctx}",
			"task_updated":             "{actor} updated task #{ref}: {fields}",
			"task_moved":               "{actor} moved task #{ref} → “{column}”",
			"task_archived":            "{actor} archived task #{ref}{ctx}",
			"task_comment":             "{actor} commented on #{ref}{ctx}",
			"task_thread_reply":        "{actor} replied in the thread on #{ref}{ctx}",
			"task_mention":             "{actor} mentioned you in #{ref}{ctx}",
			"task_due_soon":            "Task #{ref} “{title}” is due soon",
			"reminder":                 "Reminder",
			"integration_sync_ok":      "{label}: +{created} new, ~{updated} updated, in {took}",
			"integration_sync_partial": "{label}: +{created} new, ~{updated} updated, in {took} (some actions failed)",
			"integration_sync_failed":  "{label}: sync failed — {reason} (after {took})",
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
		},
		defaultNames: map[string]string{
			"personal": "Personal space",
		},
		kinds: map[string]string{
			"assigned":         "Task assigned",
			"comment":          "New comment",
			"mention":          "You were mentioned",
			"updated":          "Task updated",
			"moved":            "Task moved",
			"archived":         "Task archived",
			"due_soon":         "Deadline approaching",
			"reminder":         "Reminder",
			"integration_sync": "Sync finished",
		},
		fallbackKind: "Notification",
		listSep:      ", ",
		digestTitle:  "Notification digest",
		digestHeader: "Digest — {count} notifications:",
		instant:      "less than a second",
		durationHM:   "{h}h {m}m",
		durationMS:   "{m}m {s}s",
		durationS:    "{s}s",
	},
}

// fieldOrder is the order changed fields are listed in, so a task_updated
// sentence reads the same on every delivery (payload arrays keep their order,
// but an unknown name is dropped rather than shown raw).
var fieldOrder = []string{"title", "description", "priority", "due", "start", "estimate", "completed", "reopened"}

func bundleFor(lang string) bundle { return bundles[i18n.Normalize(lang)] }

// Title is the subject line for a notification kind, in the reader's language.
func Title(kind, lang string) string {
	b := bundleFor(lang)
	if s, ok := b.kinds[kind]; ok {
		return s
	}
	return b.fallbackKind
}

// DefaultName captions a name the server seeded and stored in Russian — today
// the personal workspace (#2800). In a client the caption is drawn from name_key
// by the client itself; a delivery that leaves the app has no client, so it is
// drawn here, in the recipient's language.
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

// DigestTitle is the subject of a combined digest delivery.
func DigestTitle(lang string) string { return bundleFor(lang).digestTitle }

// DigestHeader is the line that opens a digest, above the bulleted items.
func DigestHeader(count int, lang string) string {
	return strings.ReplaceAll(bundleFor(lang).digestHeader, "{count}", strconv.Itoa(count))
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
		"{column}", str(payload["column"]),
		"{title}", str(payload["title"]),
		"{label}", str(payload["label"]),
		"{reason}", str(payload["reason"]),
		"{created}", strconv.Itoa(num(payload["created"])),
		"{updated}", strconv.Itoa(num(payload["updated"])),
		"{took}", duration(num(payload["seconds"]), b),
		"{fields}", fieldList(payload["fields"], b),
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
	return strings.Join(words, b.listSep)
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
