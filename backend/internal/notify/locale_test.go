package notify

import (
	"encoding/json"
	"strings"
	"testing"

	"tessera/internal/i18n"
)

// samplePayload has every key any event reads, so one payload can be rendered
// through the whole catalog and checked for leftovers.
func samplePayload(event string) map[string]any {
	return map[string]any{
		"event":       event,
		"actor":       "Алиса",
		"task_number": float64(42), // as it arrives from the jsonb column
		"title":       "Починить чайник",
		"excerpt":     "уже кипит",
		"column":      "В процессе",
		"label":       "GitLab",
		"reason":      "401",
		"created":     float64(3),
		"updated":     float64(2),
		"seconds":     float64(200),
		"fields":      []any{"title", "due"},
	}
}

func TestSentenceRendersEveryEventInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Supported {
		for event := range bundles[i18n.Default].events {
			got := Sentence(samplePayload(event), lang, "LEGACY")
			if got == "" || got == "LEGACY" {
				t.Errorf("%s/%s: fell back instead of rendering (%q)", lang, event, got)
				continue
			}
			if strings.Contains(got, "{") || strings.Contains(got, "}") {
				t.Errorf("%s/%s: unrendered placeholder in %q", lang, event, got)
			}
		}
	}
}

func TestBundlesCarryTheSameKeys(t *testing.T) {
	base := bundles[i18n.Default]
	for _, lang := range i18n.Supported {
		b := bundles[lang]
		if b.ctx == "" || b.fallbackKind == "" || b.digestTitle == "" {
			t.Fatalf("%s: bundle is incomplete", lang)
		}
		for k := range base.events {
			if _, ok := b.events[k]; !ok {
				t.Errorf("%s: missing event %q", lang, k)
			}
		}
		for k := range base.fields {
			if _, ok := b.fields[k]; !ok {
				t.Errorf("%s: missing field %q", lang, k)
			}
		}
		for k := range base.kinds {
			if _, ok := b.kinds[k]; !ok {
				t.Errorf("%s: missing kind %q", lang, k)
			}
		}
	}
}

func TestSentenceWording(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		lang    string
		want    string
	}{
		{
			name:    "assigned ru inlines the title",
			payload: map[string]any{"event": "task_assigned", "actor": "Алиса", "task_number": float64(42), "title": "Починить чайник"},
			lang:    "ru",
			want:    "Алиса назначил вам задачу #42 «Починить чайник»",
		},
		{
			name:    "assigned en inlines the title",
			payload: map[string]any{"event": "task_assigned", "actor": "Alice", "task_number": float64(42), "title": "Fix the kettle"},
			lang:    "en",
			want:    "Alice assigned task #42 to you “Fix the kettle”",
		},
		{
			name:    "comment without an excerpt has no context tail",
			payload: map[string]any{"event": "task_comment", "actor": "Alice", "task_number": float64(7)},
			lang:    "en",
			want:    "Alice commented on #7",
		},
		{
			name:    "updated lists changed fields in catalog order",
			payload: map[string]any{"event": "task_updated", "actor": "Alice", "task_number": float64(7), "fields": []any{"due", "title"}},
			lang:    "en",
			want:    "Alice updated task #7: title, due date",
		},
		{
			name:    "updated drops a field this build doesn't know",
			payload: map[string]any{"event": "task_updated", "actor": "Алиса", "task_number": float64(7), "fields": []any{"title", "warp_drive"}},
			lang:    "ru",
			want:    "Алиса изменил(а) задачу #7: название",
		},
		{
			name:    "sync duration is worded from seconds",
			payload: map[string]any{"event": "integration_sync_ok", "label": "GitLab", "created": float64(3), "updated": float64(2), "seconds": float64(200)},
			lang:    "en",
			want:    "GitLab: +3 new, ~2 updated, in 3m 20s",
		},
		{
			name:    "sync failure carries the reason",
			payload: map[string]any{"event": "integration_sync_failed", "label": "GitLab", "reason": "401", "seconds": float64(0)},
			lang:    "ru",
			want:    "GitLab: синхронизация не удалась — 401 (за меньше секунды)",
		},
		{
			name:    "reminder keeps the user's own words",
			payload: map[string]any{"event": "reminder", "message": "Позвонить в банк"},
			lang:    "en",
			want:    "Позвонить в банк",
		},
		{
			name:    "empty reminder falls to the translated default",
			payload: map[string]any{"event": "reminder"},
			lang:    "en",
			want:    "Reminder",
		},
		{
			name:    "missing task number reads as unknown, not zero",
			payload: map[string]any{"event": "task_mention", "actor": "Alice"},
			lang:    "en",
			want:    "Alice mentioned you in #?",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Sentence(c.payload, c.lang, "LEGACY"); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestSentenceFallsBackToStoredText(t *testing.T) {
	legacy := "Алиса назначил вам задачу #42"
	cases := []map[string]any{
		{},                           // pre-0065 row: no payload at all
		{"event": ""},                // payload written, event lost
		{"event": "task_teleported"}, // server newer than this catalog
	}
	for _, p := range cases {
		if got := Sentence(p, "en", legacy); got != legacy {
			t.Errorf("payload %v: got %q, want the stored text", p, got)
		}
	}
}

func TestSentenceAcceptsJSONNumbers(t *testing.T) {
	// A payload decoded with UseNumber (or built in-process with int64) must read
	// the same as one decoded into float64.
	dec := json.NewDecoder(strings.NewReader(`{"event":"task_comment","actor":"Alice","task_number":9}`))
	dec.UseNumber()
	var p map[string]any
	if err := dec.Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, want := Sentence(p, "en", ""), "Alice commented on #9"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	p["task_number"] = int64(9)
	if got, want := Sentence(p, "en", ""), "Alice commented on #9"; got != want {
		t.Errorf("int64 number: got %q, want %q", got, want)
	}
}

func TestTitleAndDigest(t *testing.T) {
	if got, want := Title("assigned", "en"), "Task assigned"; got != want {
		t.Errorf("Title(assigned, en) = %q, want %q", got, want)
	}
	if got, want := Title("assigned", "de"), "Назначена задача"; got != want {
		t.Errorf("unknown language must fall back to ru, got %q", got)
	}
	if got, want := Title("no_such_kind", "en"), "Notification"; got != want {
		t.Errorf("unknown kind: got %q, want %q", got, want)
	}
	if got, want := DigestTitle("en"), "Notification digest"; got != want {
		t.Errorf("DigestTitle(en) = %q, want %q", got, want)
	}
	if got, want := DigestHeader(3, "en"), "Digest — 3 notifications:"; got != want {
		t.Errorf("DigestHeader(3, en) = %q, want %q", got, want)
	}
	if got, want := DigestHeader(3, "ru"), "Сводка — 3 уведомлений:"; got != want {
		t.Errorf("DigestHeader(3, ru) = %q, want %q", got, want)
	}
}

func TestDurationWording(t *testing.T) {
	b := bundles["en"]
	cases := map[int]string{0: "less than a second", 5: "5s", 90: "1m 30s", 3700: "1h 1m"}
	for secs, want := range cases {
		if got := duration(secs, b); got != want {
			t.Errorf("duration(%d) = %q, want %q", secs, got, want)
		}
	}
}
