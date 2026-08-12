package handlers

import (
	"testing"
	"time"

	"tessera/internal/db"
)

func TestShortCtx(t *testing.T) {
	cases := map[string]string{
		"Починить чайник":   " «Починить чайник»",  // 15 runes → inlined
		"0123456789012345":  " «0123456789012345»", // exactly 16 → inlined
		"01234567890123456": "",                    // 17 runes → omitted
		"":                  "",
		"   ":               "",
		"  Полить цветы  ":  " «Полить цветы»", // trimmed, then short
	}
	for in, want := range cases {
		if got := shortCtx(in); got != want {
			t.Fatalf("shortCtx(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeviceIDOf(t *testing.T) {
	if got := deviceIDOf(db.NotificationChannel{Config: []byte(`{"device_id":"abc","platform":"web"}`)}); got != "abc" {
		t.Fatalf("device_id = %q, want abc", got)
	}
	if got := deviceIDOf(db.NotificationChannel{Config: []byte(`{}`)}); got != "" {
		t.Fatalf("no device_id should be empty, got %q", got)
	}
	if got := deviceIDOf(db.NotificationChannel{Config: nil}); got != "" {
		t.Fatalf("nil config should be empty, got %q", got)
	}
}

// channelCfgString is what gates background push (an absent fcm_token means the
// device stays on the live-socket path), so a non-string or missing value must
// read as empty rather than panicking or coercing.
func TestChannelCfgString(t *testing.T) {
	cases := []struct {
		cfg  string
		want string
	}{
		{`{"device_id":"abc","fcm_token":"tok-1"}`, "tok-1"},
		{`{"device_id":"abc"}`, ""},
		{`{"fcm_token":null}`, ""},
		{`{"fcm_token":42}`, ""}, // wrong type — not a usable token
		{`{}`, ""},
		{`broken`, ""},
	}
	for _, tc := range cases {
		if got := channelCfgString(db.NotificationChannel{Config: []byte(tc.cfg)}, "fcm_token"); got != tc.want {
			t.Fatalf("channelCfgString(%s) = %q, want %q", tc.cfg, got, tc.want)
		}
	}
}

func TestQuietWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 6, 16, h, m, 0, 0, time.UTC) }

	// disabled / empty window
	if _, q := quietWindow(false, 1320, 480, "", at(23, 0)); q {
		t.Fatal("disabled should never be quiet")
	}
	if _, q := quietWindow(true, 600, 600, "", at(10, 0)); q {
		t.Fatal("start==end is no window")
	}

	// non-wrapping window 09:00–17:00
	if end, q := quietWindow(true, 540, 1020, "", at(12, 0)); !q || end != at(17, 0) {
		t.Fatalf("inside non-wrap: q=%v end=%v", q, end)
	}
	if _, q := quietWindow(true, 540, 1020, "", at(8, 0)); q {
		t.Fatal("08:00 is outside 09:00–17:00")
	}

	// wrapping window 22:00–08:00
	if end, q := quietWindow(true, 1320, 480, "", at(2, 0)); !q || end != at(8, 0) {
		t.Fatalf("02:00 inside wrap → ends 08:00 today: q=%v end=%v", q, end)
	}
	if end, q := quietWindow(true, 1320, 480, "", at(23, 0)); !q || !end.Equal(at(8, 0).Add(24*time.Hour)) {
		t.Fatalf("23:00 inside wrap → ends 08:00 tomorrow: q=%v end=%v", q, end)
	}
	if _, q := quietWindow(true, 1320, 480, "", at(12, 0)); q {
		t.Fatal("12:00 is outside 22:00–08:00")
	}
}

func TestDueShouldFire(t *testing.T) {
	due := time.Date(2026, 6, 16, 18, 0, 0, 0, time.UTC)
	state := func(firedDue time.Time, last time.Time) *db.DueNotificationState {
		return &db.DueNotificationState{FiredDue: firedDue, LastFiredAt: last}
	}

	cases := []struct {
		name   string
		now    time.Time
		lead   int32
		repeat int32
		prior  *db.DueNotificationState
		want   bool
	}{
		{"before lead window", due.Add(-90 * time.Minute), 60, 0, nil, false},
		{"inside lead window, never fired", due.Add(-30 * time.Minute), 60, 0, nil, true},
		{"one-shot already fired", due.Add(-30 * time.Minute), 60, 0, state(due, due.Add(-31*time.Minute)), false},
		{"due date moved → re-arm", due.Add(-30 * time.Minute), 60, 0, state(due.Add(-time.Hour), due.Add(-90*time.Minute)), true},
		{"repeat not yet elapsed", due.Add(-30 * time.Minute), 60, 15, state(due, due.Add(-25*time.Minute)), false},
		{"repeat elapsed", due.Add(-5 * time.Minute), 60, 15, state(due, due.Add(-25*time.Minute)), true},
		{"overdue still repeats", due.Add(40 * time.Minute), 60, 15, state(due, due.Add(20*time.Minute)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dueShouldFire(tc.now, due, tc.lead, tc.repeat, tc.prior); got != tc.want {
				t.Fatalf("dueShouldFire = %v, want %v", got, tc.want)
			}
		})
	}
}
