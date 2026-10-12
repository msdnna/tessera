package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The connect flow's state (#3083) names the user who started it — the callback
// is a public route with no session — so it must not be forgeable, reusable after
// it expires, or point the return anywhere off this origin.
func TestConnectState(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	user := uuid.New()
	st, err := signConnectState("secret", connectClaims{User: user, Exp: now.Add(time.Minute).Unix(), ReturnTo: "/board/x?y=1"})
	if err != nil || !strings.HasPrefix(st, oauthConnectState) {
		t.Fatalf("sign: %q %v", st, err)
	}
	cl, ok := verifyConnectState("secret", st, now)
	if !ok || cl.User != user || cl.ReturnTo != "/board/x?y=1" {
		t.Fatalf("verify = %+v %v", cl, ok)
	}
	if _, ok := verifyConnectState("other", st, now); ok {
		t.Fatal("a state signed with another secret passed")
	}
	if _, ok := verifyConnectState("secret", st, now.Add(2*time.Minute)); ok {
		t.Fatal("an expired state passed")
	}
	tampered := st[:len(oauthConnectState)+2] + "x" + st[len(oauthConnectState)+3:]
	if _, ok := verifyConnectState("secret", tampered, now); ok {
		t.Fatal("a tampered state passed")
	}
	for in, want := range map[string]string{
		"/board/1": "/board/1", "": "/", "//evil.example": "/", "https://evil": "/", "/\\evil": "/",
	} {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
	if got := withQuery("/board/1?a=2", "gitlab_oauth", "connected"); got != "/board/1?a=2&gitlab_oauth=connected" {
		t.Errorf("withQuery = %q", got)
	}
}
