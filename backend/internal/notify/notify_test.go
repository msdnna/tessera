package notify

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestMatcherMatches(t *testing.T) {
	ws1 := uuid.New()
	ws2 := uuid.New()

	cases := []struct {
		name    string
		matcher Matcher
		event   Event
		want    bool
	}{
		{"empty matches anything", Matcher{}, Event{Kind: "comment", WorkspaceID: ws1}, true},
		{"kind hit", Matcher{Kinds: []string{"mention", "assigned"}}, Event{Kind: "assigned", WorkspaceID: ws1}, true},
		{"kind miss", Matcher{Kinds: []string{"mention"}}, Event{Kind: "comment", WorkspaceID: ws1}, false},
		{"workspace hit", Matcher{WorkspaceID: &ws1}, Event{Kind: "comment", WorkspaceID: ws1}, true},
		{"workspace miss", Matcher{WorkspaceID: &ws1}, Event{Kind: "comment", WorkspaceID: ws2}, false},
		{
			"kind and workspace both required",
			Matcher{Kinds: []string{"comment"}, WorkspaceID: &ws1},
			Event{Kind: "comment", WorkspaceID: ws2},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.matcher.Matches(tc.event); got != tc.want {
				t.Fatalf("Matches() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRedact(t *testing.T) {
	token := "123456:AA-realbottoken"
	// Mimics the net/http timeout error that embeds the request URL (token and all).
	msg := `Post "https://api.telegram.org/bot` + token + `/sendMessage": context deadline exceeded`
	got := redact(msg, token)
	if got == msg {
		t.Fatal("redact did not change the message")
	}
	if contains := len(got) > 0 && (indexOf(got, token) >= 0); contains {
		t.Fatalf("token leaked through redact: %q", got)
	}
	// Empty secrets are a no-op (and must not blank the whole string).
	if redact("hello", "", "  ") != "hello" {
		t.Fatal("empty secrets should be ignored")
	}
}

// indexOf is a tiny strings.Index without importing strings into the test.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestPermanentError(t *testing.T) {
	base := errors.New("boom")
	if IsPermanent(base) {
		t.Fatal("plain error should not be permanent")
	}
	p := Permanent(base)
	if !IsPermanent(p) {
		t.Fatal("Permanent(err) should be permanent")
	}
	if !errors.Is(p, base) {
		t.Fatal("Permanent should unwrap to the original error")
	}
}
