//go:build e2e

// cmd/token — the CLI that bootstraps headless clients (the MCP server). It
// ships inside the image and is covered by nothing: it has no HTTP surface, so
// the in-process harness cannot reach it, and a token it mints is only proven
// good by a server accepting it.
package e2e

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// patRE matches the printed personal access token (auth.PATPrefix + 24 hex bytes).
var patRE = regexp.MustCompile(`tsra_[0-9a-f]{48}`)

func runToken(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binToken, args...)
	cmd.Dir = subprocDir
	cmd.Env = append(baseEnv(), "DATABASE_URL="+serverDBURL)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestTokenCLIMintsAcceptedToken(t *testing.T) {
	s := startServer(t, nil)
	a := s.register(t, "pat")

	out, err := runToken(t, "-email", a.Email, "-name", "e2e")
	if err != nil {
		t.Fatalf("token CLI: %v\n%s", err, out)
	}
	tok := patRE.FindString(out)
	if tok == "" {
		t.Fatalf("no token in the CLI output:\n%s", out)
	}

	me := expect(t, s.get(t, "/auth/me", tok), 200)
	user, _ := me["user"].(map[string]any)
	if user == nil || user["email"] != a.Email {
		t.Fatalf("minted token authenticated as %v, want %s", me["user"], a.Email)
	}
}

func TestTokenCLIRejectsUnknownUser(t *testing.T) {
	out, err := runToken(t, "-email", "nobody-"+runID+"@test.local")
	if err == nil {
		t.Fatalf("CLI succeeded for a user that does not exist:\n%s", out)
	}
	if !strings.Contains(out, "no user with email") {
		t.Errorf("failure message does not name the cause:\n%s", out)
	}
}
