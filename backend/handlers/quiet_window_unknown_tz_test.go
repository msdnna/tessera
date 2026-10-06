package handlers

import (
	"testing"
	"time"
)

// TestQuietWindowUnknownZoneSilentlyMeansUTC records the behaviour that makes
// validating quiet_tz worth a 400 (see TestQuietHoursRejectsUnknownTimezone).
//
// quietWindow resolves the stored zone with time.LoadLocation and keeps UTC when
// that fails. There is no error to surface and no second chance to notice: the
// scanner just reads the user's night off the wrong clock. This test is the
// measurement, not a wish — it asserts the fallback exists, so that the day it
// stops being silent the reason for the endpoint's validation is re-read rather
// than quietly deleted.
func TestQuietWindowUnknownZoneSilentlyMeansUTC(t *testing.T) {
	t.Parallel()

	// 23:30 Moscow = 20:30 UTC. A 22:00–08:00 window covers the first and not the
	// second, so the two clocks disagree about this instant.
	at := time.Date(2026, 9, 20, 20, 30, 0, 0, time.UTC)

	if _, in := quietWindow(true, 1320, 480, "Europe/Moscow", at); !in {
		t.Fatal("23:30 Moscow must fall inside a 22:00–08:00 Moscow window")
	}
	if _, in := quietWindow(true, 1320, 480, "UTC", at); in {
		t.Fatal("20:30 UTC must fall outside a 22:00–08:00 UTC window")
	}
	// The whole point: a zone that does not resolve behaves exactly like UTC, and
	// says nothing about having done so.
	if _, in := quietWindow(true, 1320, 480, "Europe/Moskva", at); in {
		t.Fatal("an unresolvable zone no longer falls back to UTC — re-read why " +
			"UpdateMyNotificationPrefs rejects unknown zones before changing it")
	}
}
