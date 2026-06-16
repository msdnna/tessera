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

func TestShoutrrrURL(t *testing.T) {
	// generic shoutrrr channel passes its secret URL through verbatim
	if u, err := shoutrrrURL(Channel{Type: "shoutrrr", Secret: map[string]string{"url": "discord://tok@id"}}); err != nil || u != "discord://tok@id" {
		t.Fatalf("shoutrrr passthrough = %q, %v", u, err)
	}
	// telegram builds a telegram:// URL from token + chat id
	tg := Channel{Type: "telegram", Secret: map[string]string{"bot_token": "123:ABC"}, Config: map[string]any{"chat_id": "42"}}
	if u, err := shoutrrrURL(tg); err != nil || u != "telegram://123:ABC@telegram/?chats=42" {
		t.Fatalf("telegram url = %q, %v", u, err)
	}
	// missing required fields error out
	if _, err := shoutrrrURL(Channel{Type: "telegram"}); err == nil {
		t.Fatal("telegram without token/chat should error")
	}
	if _, err := shoutrrrURL(Channel{Type: "shoutrrr"}); err == nil {
		t.Fatal("shoutrrr without url should error")
	}
}

func TestRender(t *testing.T) {
	data := SampleData()
	// default template = text + link
	out, err := Render(DefaultTemplate, data)
	if err != nil || !indexOfOK(out, data.Text) || !indexOfOK(out, data.Link) {
		t.Fatalf("default render = %q, %v", out, err)
	}
	// custom template using the documented fields
	out, err = Render("#{{.TaskNumber}} {{.TaskTitle}} — {{.Actor}}", data)
	if err != nil || out != "#42 Починить чайник — Алиса" {
		t.Fatalf("custom render = %q, %v", out, err)
	}
	// unknown field errors (so the editor preview can surface it)
	if _, err := Render("{{.Nope}}", data); err == nil {
		t.Fatal("unknown field should error")
	}
	// parse error surfaces too
	if _, err := Render("{{.Text", data); err == nil {
		t.Fatal("bad syntax should error")
	}
}

func indexOfOK(s, sub string) bool { return indexOf(s, sub) >= 0 }

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
