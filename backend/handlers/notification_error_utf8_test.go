package handlers

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// truncErr's output is stored in the last_error text column, so it has to stay
// valid UTF-8: Postgres rejects a broken byte sequence outright, and the caller
// (settleDelivery) swallows that rejection through soft() — leaving the row in
// 'sending' for the stale-requeue to send all over again. The package already
// states the rule twice, in truncateRunes ("not bytes — the quote is Russian
// text as often as not, and a byte cut would split a character") and in
// truncForJournal; this is the third caller of it.
func TestTruncErrKeepsValidUTF8(t *testing.T) {
	cases := map[string]string{
		// The cut lands between the two bytes of a Cyrillic letter: the prefix is
		// padded to 499 bytes so byte 500 is the tail half of the first "я".
		"cut inside a letter": strings.Repeat("a", 499) + strings.Repeat("я", 60),
		// And one byte later, where the cut is already rune-aligned — the case
		// that makes the byte version look fine half the time.
		"cut on a boundary": strings.Repeat("a", 500) + strings.Repeat("я", 60),
		// A realistic transport error: the webhook URL travels inside it, and a
		// raw query string is not escaped on the way into url.Error.
		"webhook error": `Post "http://127.0.0.1:65535/?q=` + strings.Repeat("значение-", 60) + `": dial tcp: connect: connection refused`,
	}
	for name, msg := range cases {
		got := truncErr(errors.New(msg))
		if !utf8.ValidString(got) {
			t.Errorf("%s: truncErr produced invalid UTF-8 (%d bytes, tail %q)", name, len(got), got[max(0, len(got)-8):])
		}
		if strings.ContainsRune(got, utf8.RuneError) && !strings.ContainsRune(msg, utf8.RuneError) {
			t.Errorf("%s: truncErr introduced a replacement character", name)
		}
	}
}

// A short error must survive untouched — capping is for verbose upstream bodies.
func TestTruncErrLeavesShortErrorAlone(t *testing.T) {
	msg := "канал недоступен: соединение отклонено"
	if got := truncErr(errors.New(msg)); got != msg {
		t.Fatalf("truncErr rewrote a short error: %q", got)
	}
}
