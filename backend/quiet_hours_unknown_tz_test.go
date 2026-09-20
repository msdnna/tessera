// Quiet hours are evaluated in the zone the user stored, and an unrecognised zone
// used to be accepted in silence.
package main

import (
	"net/http"
	"testing"
)

// TestQuietHoursRejectsUnknownTimezone pins that the prefs endpoint refuses a
// zone it cannot resolve.
//
// quiet_tz is the zone the scanner reads the window in: quietWindow calls
// time.LoadLocation on it and, when that fails, falls back to UTC without a
// trace. Stored unvalidated, a zone the server's tzdata does not know turns the
// whole feature into a lie both clients corroborate — the PUT answers 200 and
// echoes the string back, so the web and Android screens print "22:00–08:00,
// <zone>" while the server is silencing 22:00–08:00 UTC. For a Moscow user that
// is 01:00–11:00 local: the phone buzzes through the night and goes quiet over
// the working morning.
//
// It is not a hypothetical string either. Android sends
// TimeZone.getDefault().id, which on a device with a manually picked offset is
// "GMT+03:00" — a valid Java id and not an IANA name; and a browser with newer
// tzdata than the server image knows zones (Europe/Kyiv, America/Ciudad_Juarez)
// the container does not. The same silent-UTC fallback already bit us once from
// the other side, when an empty zone meant "browser" on the client and "UTC" on
// the server (#2932).
func TestQuietHoursRejectsUnknownTimezone(t *testing.T) {
	t.Parallel()
	c := signup(t)

	prefs := func(tz string) map[string]any {
		return map[string]any{
			"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
			"reminder_enabled": true, "quiet_enabled": true,
			"quiet_start_minutes": 1320, "quiet_end_minutes": 480,
			"quiet_tz": tz, "digest_minutes": 0,
		}
	}

	// Control: a real IANA zone is stored and echoed back.
	p := c.expect(t, c.put("/notification-prefs", prefs("Europe/Moscow")), http.StatusOK)
	if p["quiet_tz"] != "Europe/Moscow" {
		t.Fatalf("a known zone must round-trip, got: %v", p)
	}

	// The finding: zones the server cannot resolve must not be accepted.
	// A typo is the shape a hand-written API call has; "GMT+03:00" is what Android
	// hands over for a manually picked offset (a valid Java id, not an IANA name);
	// "Moscow Standard Time" is the Windows id a naive integration sends.
	for _, tz := range []string{"Europe/Moskva", "GMT+03:00", "Moscow Standard Time", "не зона"} {
		if r := c.put("/notification-prefs", prefs(tz)); r.Status != http.StatusBadRequest {
			t.Fatalf("quiet_tz %q was accepted with status %d: quiet hours will run in UTC "+
				"while both clients show %q\n%s", tz, r.Status, tz, r.Body)
		}
	}

	// A rejected PUT must not have moved anything: the zone that still guards the
	// user's night is the last good one.
	p = c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["quiet_tz"] != "Europe/Moscow" {
		t.Fatalf("a rejected PUT changed the stored zone: %v", p)
	}
	if p["quiet_start_minutes"] != float64(1320) || p["quiet_end_minutes"] != float64(480) {
		t.Fatalf("a rejected PUT changed the stored window: %v", p)
	}

	// Control: an empty zone stays legal. It is the documented "evaluate in UTC"
	// value and the default for a user who never customised, so validation must
	// not turn the default into an error.
	p = c.expect(t, c.put("/notification-prefs", prefs("")), http.StatusOK)
	if p["quiet_tz"] != "" {
		t.Fatalf("an empty zone must stay accepted, got: %v", p)
	}

	// Control: quiet hours turned off do not need a zone at all — the window is
	// not read, and a stale unresolvable string must not block saving other prefs.
	off := prefs("")
	off["quiet_enabled"] = false
	c.expect(t, c.put("/notification-prefs", off), http.StatusOK)
}
