package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// fcmData pulls the data map out of a built request, which is the thing FCM
// weighs against its ceiling.
func fcmData(t *testing.T, msg Message) map[string]string {
	t.Helper()
	req := fcmRequest("tok-abc", msg)
	m, ok := req["message"].(map[string]any)
	if !ok {
		t.Fatalf("request has no message object: %#v", req)
	}
	data, ok := m["data"].(map[string]string)
	if !ok {
		t.Fatalf("message has no data map: %#v", m)
	}
	return data
}

// TestFCMTrimsAnOversizedBody: a push whose body outgrows FCM's 4096-byte data
// ceiling is cut down to fit instead of being sent whole.
//
// Sent whole it comes back 400 INVALID_ARGUMENT, which fcmError classifies as
// permanent — so the delivery worker marks it failed on its first attempt and
// the phone never rings, while the same notification sits in the app as usual
// and nothing the reader can see says why. The body is the only field that can
// get there: a channel template is allowed 8192 characters, a reminder carries
// the user's own text verbatim (Sentence), and a digest glues a whole claimed
// batch into one message. Cyrillic costs two bytes a character, so none of those
// has to look long to pass 4096.
func TestFCMTrimsAnOversizedBody(t *testing.T) {
	// A reminder the user wrote in Russian: 3000 characters, two bytes each.
	long := strings.Repeat("я", 3000)
	data := fcmData(t, Message{
		Kind: "reminder", Title: "Напоминание", Body: long,
		Link: "https://app/x", ID: "n-1", TaskID: "t-1",
	})

	if n := dataBytes(data); n > maxFCMDataBytes {
		t.Fatalf("data payload is %d bytes, over FCM's %d — the push would come back 400 and be dropped for good", n, maxFCMDataBytes)
	}
	body := data["body"]
	if !strings.HasPrefix(body, "яяя") {
		t.Fatalf("body = %q, want what the user actually wrote, shortened", body)
	}
	if !strings.HasSuffix(body, fcmEllipsis) {
		t.Fatalf("body ends %q, want the cut marked with %q", body[len(body)-6:], fcmEllipsis)
	}
	if !utf8.ValidString(body) {
		t.Fatal("body was cut mid-rune — it would reach the device as replacement characters")
	}
	// The fields the deep link and the WS dedup run on must survive whole.
	for k, want := range map[string]string{
		"notification_id": "n-1", "task_id": "t-1", "kind": "reminder", "link": "https://app/x",
	} {
		if data[k] != want {
			t.Fatalf("data[%q] = %q, want %q — the trim must only touch the body", k, data[k], want)
		}
	}
}

// TestFCMLeavesAnOrdinaryBodyAlone: the ceiling is a backstop, not a formatter —
// a normal notification reaches the device character for character.
func TestFCMLeavesAnOrdinaryBodyAlone(t *testing.T) {
	body := "Иван назначил вам задачу #42 «Починить логин»"
	data := fcmData(t, Message{Kind: "assigned", Title: "Назначена задача", Body: body, Link: "https://app/x"})
	if data["body"] != body {
		t.Fatalf("body = %q, want it untouched: %q", data["body"], body)
	}
}

// TestTruncateBytesBoundaries pins the cut itself: never over budget, never
// mid-rune, and empty when there is no room even for the marker.
func TestTruncateBytesBoundaries(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 10, "abc"},   // fits
		{"abc", 3, "abc"},    // fits exactly
		{"abcdef", 5, "ab…"}, // 2 bytes + a 3-byte marker
		{"abcdef", 3, "…"},
		{"abcdef", 2, ""},  // no room for the marker
		{"abcdef", -1, ""}, // nothing left at all
		{"яяя", 6, "яяя"},  // fits exactly
		{"яяяя", 7, "яя…"},
		{"яяяя", 6, "я…"}, // a second я would split across the budget
	}
	for _, tc := range cases {
		got := truncateBytes(tc.in, tc.max)
		if got != tc.want {
			t.Errorf("truncateBytes(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
		// A negative budget is "nothing fits", so the empty result is the floor.
		if got != tc.in && len(got) > tc.max && len(got) > 0 {
			t.Errorf("truncateBytes(%q, %d) = %q — %d bytes, over budget", tc.in, tc.max, got, len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("truncateBytes(%q, %d) = %q — cut mid-rune", tc.in, tc.max, got)
		}
	}
}
