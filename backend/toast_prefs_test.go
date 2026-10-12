// Всплывающие в приложении и «Не беспокоить» (#3117, миграция 0082): настройки
// тостов лежат в notification_prefs рядом с тихими часами.
//
// Главная ловушка — PUT /notification-prefs исторически full-replace, а Android
// шлёт только те поля, о которых знает. Новые поля поэтому «нет в теле» —
// «оставить как было», иначе одно сохранение настроек с телефона молча
// выключало бы тосты в браузере.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// The scheduling fields Android sends — everything but the toast ones.
func androidPrefs() map[string]any {
	return map[string]any{
		"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": true, "quiet_enabled": false,
		"quiet_start_minutes": 1320, "quiet_end_minutes": 480, "quiet_tz": "", "digest_minutes": 0,
	}
}

func TestToastPrefsDefaultsAndRoundTrip(t *testing.T) {
	t.Parallel()
	c := signup(t)

	p := c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["toasts_enabled"] != true || p["quiet_toasts"] != false || p["dnd_until"] != nil {
		t.Fatalf("defaults: %v", p)
	}
	if cfg, ok := p["toasts"].(map[string]any); !ok || len(cfg) != 0 {
		t.Fatalf("default toasts config should be an empty object, got %#v", p["toasts"])
	}

	body := androidPrefs()
	body["toasts_enabled"] = false
	body["quiet_toasts"] = true
	body["toasts"] = map[string]any{"activity": "all", "batchMin": 4, "me": map[string]any{"comments": false}}
	p = c.expect(t, c.put("/notification-prefs", body), http.StatusOK)
	if p["toasts_enabled"] != false || p["quiet_toasts"] != true {
		t.Fatalf("round-trip: %v", p)
	}
	cfg := p["toasts"].(map[string]any)
	if cfg["activity"] != "all" || cfg["batchMin"] != float64(4) {
		t.Fatalf("toasts config round-trip: %v", cfg)
	}

	// An Android save knows nothing of the toast fields — they stay.
	p = c.expect(t, c.put("/notification-prefs", androidPrefs()), http.StatusOK)
	if p["toasts_enabled"] != false || p["quiet_toasts"] != true {
		t.Fatalf("a PUT without the toast fields reset them: %v", p)
	}
	if cfg := p["toasts"].(map[string]any); cfg["activity"] != "all" {
		t.Fatalf("a PUT without the toast config reset it: %v", cfg)
	}
}

func TestToastPrefsRejectsJunkConfig(t *testing.T) {
	t.Parallel()
	c := signup(t)
	for name, v := range map[string]any{
		"array":  []any{1, 2},
		"string": "on",
		"huge":   map[string]any{"x": string(make([]byte, 5000))},
	} {
		body := androidPrefs()
		body["toasts"] = v
		if r := c.put("/notification-prefs", body); r.Status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400\n%s", name, r.Status, r.Body)
		}
	}
}

func TestNotificationDnd(t *testing.T) {
	t.Parallel()
	c := signup(t)

	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	p := c.expect(t, c.put("/notification-prefs/dnd", map[string]any{"until": until.Format(time.RFC3339)}), http.StatusOK)
	got, err := time.Parse(time.RFC3339, p["dnd_until"].(string))
	if err != nil || !got.Equal(until) {
		t.Fatalf("dnd_until = %v, want %v", p["dnd_until"], until)
	}
	// It is part of the prefs everybody reads, and survives a scheduling save.
	c.expect(t, c.put("/notification-prefs", androidPrefs()), http.StatusOK)
	p = c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["dnd_until"] == nil {
		t.Fatalf("a scheduling save switched «Не беспокоить» off: %v", p)
	}

	// Off.
	p = c.expect(t, c.put("/notification-prefs/dnd", map[string]any{"until": nil}), http.StatusOK)
	if p["dnd_until"] != nil {
		t.Fatalf("dnd off: %v", p)
	}

	// A moment already gone is "off", not an error.
	past := time.Now().Add(-time.Minute).Format(time.RFC3339)
	p = c.expect(t, c.put("/notification-prefs/dnd", map[string]any{"until": past}), http.StatusOK)
	if p["dnd_until"] != nil {
		t.Fatalf("dnd in the past: %v", p)
	}

	// Past a week is a typo, not a preset.
	far := time.Now().Add(9 * 24 * time.Hour).Format(time.RFC3339)
	if r := c.put("/notification-prefs/dnd", map[string]any{"until": far}); r.Status != http.StatusBadRequest {
		t.Fatalf("dnd for 9 days: status %d, want 400", r.Status)
	}
}

// The reminder toast offers «Отложить» / «Готово», which act on the reminder:
// the notification has to say which one.
func TestReminderNotificationCarriesReminderID(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	mkStack(t, c)
	fireReminder(ctx, t, c, "созвон по лимитам")

	var found bool
	for _, n := range c.get("/notifications").listBody(t) {
		if n["kind"] != "reminder" {
			continue
		}
		p, _ := n["payload"].(map[string]any)
		id, _ := p["reminder_id"].(string)
		if id == "" {
			t.Fatalf("reminder notification without reminder_id: %v", n)
		}
		found = true
	}
	if !found {
		t.Fatal("no reminder notification")
	}
}
