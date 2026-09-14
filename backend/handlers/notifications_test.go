package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

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

// The end of the window is a reading on the user's wall clock, so the day the
// clocks change must not move it. Computing it as a duration from local midnight
// did: the autumn Sunday lasts 25 hours, so midnight+7h is 06:00 and the phone
// starts buzzing an hour before the user said it may; the spring one lasts 23, so
// the same sum is 08:00 and holds delivery an hour too long. Every other case in
// TestQuietWindow runs in UTC, where no day is ever anything but 24 hours.
func TestQuietWindowAcrossClockChange(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("tzdata for Europe/Berlin unavailable: %v", err)
	}
	// Quiet 22:00–07:00 (wrapping), read in the user's own zone.
	const startMin, endMin = 1320, 420
	local := func(y int, mo time.Month, d, h, m int) time.Time {
		return time.Date(y, mo, d, h, m, 0, 0, berlin)
	}
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		// 2026-10-25 is the 25-hour Sunday (03:00 CEST → 02:00 CET).
		{"autumn, inside the long night", local(2026, 10, 25, 1, 0), local(2026, 10, 25, 7, 0)},
		// Same night entered before midnight — the "resumes tomorrow" branch, which
		// used to add a flat 24 hours to a day that has 25.
		{"autumn, entered the evening before", local(2026, 10, 24, 23, 0), local(2026, 10, 25, 7, 0)},
		// 2026-03-29 is the 23-hour Sunday (02:00 CET → 03:00 CEST).
		{"spring, inside the short night", local(2026, 3, 29, 1, 0), local(2026, 3, 29, 7, 0)},
		{"spring, entered the evening before", local(2026, 3, 28, 23, 0), local(2026, 3, 29, 7, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			end, quiet := quietWindow(true, startMin, endMin, "Europe/Berlin", tc.now)
			if !quiet {
				t.Fatalf("%s should be inside 22:00–07:00", tc.now.Format(time.RFC3339))
			}
			if !end.Equal(tc.want) {
				t.Errorf("window ends %s, want %s (07:00 on the user's clock)",
					end.In(berlin).Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
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

// A digest is only ever combined out of rows the drain claims in one tick
// (deliverGroup groups whatever ClaimPendingDeliveries returned), so two
// notifications inside the same window have to come due at the *same* instant
// and under the same group key — otherwise each is delivered on its own and the
// window is nothing but a delay. The user-facing promise is per-window, not
// per-notification: "уведомления за окно объединяются в одно сообщение на канал".
func TestDigestSlotCombinesWholeWindow(t *testing.T) {
	ch := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	window := int32(30)
	first := time.Date(2026, 6, 16, 10, 2, 0, 0, time.UTC)
	later := time.Date(2026, 6, 16, 10, 21, 30, 0, time.UTC) // same 10:00–10:30 window

	at1, g1 := digestSlot(first, time.Time{}, window, ch)
	at2, g2 := digestSlot(later, time.Time{}, window, ch)
	if !at1.Equal(at2) {
		t.Fatalf("two notifications in one window come due apart: %v vs %v — delivered separately", at1, at2)
	}
	if g1 != g2 {
		t.Fatalf("same window, different digest group: %q vs %q", g1, g2)
	}
	if !at1.After(first) || at1.Sub(first) > time.Duration(window)*time.Minute {
		t.Fatalf("due %v is not inside the window that follows %v", at1, first)
	}

	// The next window is a separate digest: its rows must not wait for, or merge
	// into, the one that already went out.
	next := time.Date(2026, 6, 16, 10, 31, 0, 0, time.UTC)
	at3, g3 := digestSlot(next, time.Time{}, window, ch)
	if g3 == g1 {
		t.Fatalf("next window reuses the spent group key %q", g3)
	}
	if !at3.After(at1) {
		t.Fatalf("next window due %v must follow the previous digest %v", at3, at1)
	}
}

// Quiet hours and the digest compose: everything released at the window's end is
// one burst, so it must be combined rather than arrive as a stream. And with no
// digest configured, delivery still resumes exactly when the quiet window ends.
func TestDigestSlotWithQuietHours(t *testing.T) {
	ch := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	qEnd := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)

	// No digest: quiet hours alone decide, and they only ever push delivery later.
	now := time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC)
	if at, g := digestSlot(now, qEnd, 0, ch); !at.Equal(qEnd) || g != "" {
		t.Fatalf("no digest, quiet → at=%v g=%q, want %v and no group", at, g, qEnd)
	}
	if at, _ := digestSlot(now, time.Time{}, 0, ch); !at.Equal(now) {
		t.Fatalf("no digest, no quiet → at=%v, want now", at)
	}

	// Digest shorter than the remaining quiet time: the window may not release a
	// delivery before quiet hours are over.
	at1, g1 := digestSlot(now, qEnd, 30, ch)
	at2, g2 := digestSlot(now.Add(2*time.Hour), qEnd, 30, ch)
	if at1.Before(qEnd) || at2.Before(qEnd) {
		t.Fatalf("delivery escapes quiet hours: %v / %v before %v", at1, at2, qEnd)
	}
	if !at1.Equal(at2) || g1 != g2 {
		t.Fatalf("quiet-release burst not combined: %v/%q vs %v/%q", at1, g1, at2, g2)
	}
}
