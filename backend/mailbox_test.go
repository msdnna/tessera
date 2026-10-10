package main

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"tessera/internal/mail"
)

// testMailbox is the harness's mailer: it behaves like the no-op one (Enabled
// false — nothing here pretends SMTP is configured) and keeps every letter, so
// a test can read the code or link the server would have mailed (#3035).
type testMailbox struct {
	mu      sync.Mutex
	letters []testLetter
}

type testLetter struct{ to, subject, body string }

var testMail = &testMailbox{}

var _ mail.Mailer = testMail

func (m *testMailbox) Enabled() bool { return false }

func (m *testMailbox) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.letters = append(m.letters, testLetter{to: strings.ToLower(to), subject: subject, body: body})
	return nil
}

// count is how many letters went to an address so far.
func (m *testMailbox) count(to string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, l := range m.letters {
		if l.to == strings.ToLower(to) {
			n++
		}
	}
	return n
}

// waitLetter waits for the (after+1)-th letter to an address — letters go out
// on a goroutine (mail.SendAsync) — and returns it.
func (m *testMailbox) waitLetter(t *testing.T, to string, after int) testLetter {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := 0
		for _, l := range m.letters {
			if l.to != strings.ToLower(to) {
				continue
			}
			if n == after {
				m.mu.Unlock()
				return l
			}
			n++
		}
		m.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no letter #%d to %s", after+1, to)
	return testLetter{}
}

// noLetter asserts no (after+1)-th letter reaches an address within a moment.
func (m *testMailbox) noLetter(t *testing.T, to string, after int) {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
	if n := m.count(to); n > after {
		t.Fatalf("unexpected letter to %s (%d sent, expected %d)", to, n, after)
	}
}

// waitCode waits for a letter to an address that carries a code and returns the
// newest such code — the address may also hold invitations or notices.
func (m *testMailbox) waitCode(t *testing.T, to string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		code := ""
		for _, l := range m.letters {
			if l.to == strings.ToLower(to) {
				if c := sixDigits.FindString(l.body); c != "" {
					code = c
				}
			}
		}
		m.mu.Unlock()
		if code != "" {
			return code
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no code mailed to %s", to)
	return ""
}

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

// codeIn pulls the six-digit code out of a letter.
func codeIn(t *testing.T, l testLetter) string {
	t.Helper()
	code := sixDigits.FindString(l.body)
	if code == "" {
		t.Fatalf("no code in letter %q:\n%s", l.subject, l.body)
	}
	return code
}
