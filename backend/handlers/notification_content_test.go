package handlers

import (
	"encoding/json"
	"testing"

	"tessera/internal/db"
)

// The legacy `text` is what old clients (and every row written before migration
// 0065) show, so each builder is pinned to the exact sentence the handlers used
// to format inline — a wording drift here is a silent regression for Android.
func TestNotifyMsgLegacyText(t *testing.T) {
	num := int64(42)
	cases := []struct {
		name string
		msg  notifyMsg
		want string
	}{
		{"assigned", msgAssigned("Иван", &num, "Полить цветы"), "Иван назначил вам задачу #42 «Полить цветы»"},
		{"assigned/long title", msgAssigned("Иван", &num, "Очень длинное название задачи"), "Иван назначил вам задачу #42"},
		{"assigned/no number", msgAssigned("Иван", nil, ""), "Иван назначил вам задачу #?"},
		{"updated", msgUpdated("Иван", &num, []string{"title", "due"}), "Иван изменил(а) задачу #42: название, срок"},
		{"moved", msgMoved("Иван", &num, "Готово"), "Иван переместил(а) задачу #42 → «Готово»"},
		{"archived", msgArchived("Иван", &num, "Полить цветы"), "Иван архивировал(а) задачу #42 «Полить цветы»"},
		{"comment", msgComment("Иван", &num, "Готово"), "Иван прокомментировал #42 «Готово»"},
		{"thread reply", msgThreadReply("Иван", &num, "Готово"), "Иван ответил(а) в обсуждении #42 «Готово»"},
		{"mention", msgMention("Иван", &num, "Готово"), "Иван упомянул(а) вас в #42 «Готово»"},
		{"due soon", msgDueSoon(&num, "Полить цветы"), "Приближается срок задачи #42 «Полить цветы»"},
		{"reminder", msgReminder("  Позвонить  "), "Позвонить"},
		{"reminder/empty", msgReminder("   "), "Напоминание"},
		{
			"sync ok",
			msgSyncDone(syncCounts{label: "GitLab · a/b", created: 2, updated: 3, took: "5 с", seconds: 5}, false),
			"GitLab · a/b: +2 новых, ~3 обновлено, за 5 с",
		},
		{
			"sync partial",
			msgSyncDone(syncCounts{label: "GitLab · a/b", created: 2, updated: 3, took: "5 с", seconds: 5}, true),
			"GitLab · a/b: +2 новых, ~3 обновлено, за 5 с (часть действий с ошибками)",
		},
		{
			"sync failed",
			msgSyncFailed(syncCounts{label: "GitLab · a/b", took: "5 с", seconds: 5, reason: "401"}),
			"GitLab · a/b: синхронизация не удалась — 401 (за 5 с)",
		},
		{
			"sync failed/no reason",
			msgSyncFailed(syncCounts{label: "GitLab · a/b", took: "5 с", seconds: 5}),
			"GitLab · a/b: синхронизация не удалась — неизвестная ошибка (за 5 с)",
		},
	}
	for _, tc := range cases {
		if tc.msg.text != tc.want {
			t.Errorf("%s: text = %q, want %q", tc.name, tc.msg.text, tc.want)
		}
	}
}

// The payload is the half a client renders from, so the facts have to be there
// in machine form — no Russian words, no pre-formatted duration.
func TestNotifyMsgPayload(t *testing.T) {
	num := int64(42)

	p := decode(t, msgUpdated("Иван", &num, []string{"title", "completed"}))
	if p["event"] != "task_updated" || p["actor"] != "Иван" || p["task_number"] != float64(42) {
		t.Fatalf("updated payload = %v", p)
	}
	fields, _ := p["fields"].([]any)
	if len(fields) != 2 || fields[0] != "title" || fields[1] != "completed" {
		t.Fatalf("updated fields = %v, want the machine names", p["fields"])
	}

	// A title too long to inline is left out of both halves: the notification
	// says "#42" and nothing more, in either language.
	p = decode(t, msgAssigned("Иван", &num, "Очень длинное название задачи"))
	if _, ok := p["title"]; ok {
		t.Fatalf("long title should not be inlined, got %v", p["title"])
	}

	// No task means no number — the client renders "#?" itself.
	p = decode(t, msgSyncDone(syncCounts{label: "GitLab · a/b", created: 1, updated: 0, seconds: 80}, false))
	if _, ok := p["task_number"]; ok {
		t.Fatalf("sync notification should carry no task number, got %v", p["task_number"])
	}
	if p["seconds"] != float64(80) {
		t.Fatalf("seconds = %v, want the raw number (the client words the duration)", p["seconds"])
	}

	// An empty reminder carries no message: the client shows its own default.
	p = decode(t, msgReminder(""))
	if _, ok := p["message"]; ok {
		t.Fatalf("empty reminder should carry no message, got %v", p["message"])
	}
}

// A payload-less message must still store valid JSON — the column is NOT NULL.
func TestNotifyMsgEmptyJSON(t *testing.T) {
	if got := string(notifyMsg{text: "плоский текст"}.json()); got != "{}" {
		t.Fatalf("empty payload = %s, want {}", got)
	}
}

// A conference invitation (#2875) is the one event that points at something
// other than a task, so the id to open travels in the payload — and every
// destination (the bell feed, the desktop deep link, the push) reads it there.
func TestMsgConferenceInvited(t *testing.T) {
	m := msgConferenceInvited("Алиса", "Летучка", "c-1")
	p := decode(t, m)
	if p["event"] != evConfInvited {
		t.Fatalf("event = %v, want %s", p["event"], evConfInvited)
	}
	if p["conference_id"] != "c-1" {
		t.Fatalf("conference_id = %v, want c-1", p["conference_id"])
	}
	if p["title"] != "Летучка" {
		t.Fatalf("title = %v, want the conference title", p["title"])
	}
	if _, ok := p["task_number"]; ok {
		t.Fatalf("an invitation is about no task, got task_number %v", p["task_number"])
	}
	if m.text != "Алиса приглашает вас в конференцию «Летучка»" {
		t.Fatalf("legacy text = %q", m.text)
	}
}

// notifyLink turns that payload into the URL an email or push opens. A task
// notification keeps landing on the app root; only the invitation deep-links,
// because it is the one that goes stale.
func TestNotifyLinkDeepLinksAConference(t *testing.T) {
	invite := db.Notification{Payload: msgConferenceInvited("Алиса", "Летучка", "c-1").json()}
	if got := notifyLink("https://app/", invite); got != "https://app/conferences/c-1" {
		t.Fatalf("conference link = %q", got)
	}
	// No public URL configured: there is no base to hang a path on, and half a
	// URL is worse than none.
	if got := notifyLink("", invite); got != "" {
		t.Fatalf("link without a public URL = %q, want empty", got)
	}
	task := db.Notification{Payload: msgAssigned("Алиса", nil, "Полить цветы").json()}
	if got := notifyLink("https://app/", task); got != "https://app/" {
		t.Fatalf("task link = %q, want the app root", got)
	}
	if got := notifyLink("https://app/", db.Notification{}); got != "https://app/" {
		t.Fatalf("payload-less link = %q, want the app root", got)
	}
}

// fieldWords must cover every name journalUpdate can return, or the legacy text
// silently drops a changed field from its list.
func TestFieldWordsCoverJournalFields(t *testing.T) {
	for _, f := range []string{"title", "description", "priority", "due", "start", "estimate", "completed", "reopened"} {
		if fieldWords[f] == "" {
			t.Errorf("fieldWords has no Russian word for %q", f)
		}
	}
}

func decode(t *testing.T, m notifyMsg) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(m.json(), &out); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	return out
}
