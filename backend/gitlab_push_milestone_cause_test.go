// Pushing a native milestone to GitLab has exactly one failure path after the
// remote milestone already exists: the link row refuses to write. That is the
// worst moment in the handler to lose the reason — the GitLab milestone is out
// there and Tessera has no record of it, so the next press of the button makes a
// second one — and it was the one place that did (#2932).
//
// The invariant is the one TestFailDoesNotLeakError states for `fail` itself:
// the cause never reaches the client, and it always reaches the log.
package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// captureDefaultSlog redirects the default logger into a buffer for the duration
// of the test. Deliberately NOT parallel: slog.SetDefault is process-wide, and
// the harness's background workers log through it too.
func captureDefaultSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// failedRequestLine returns the `fail` log line for the given request path.
// Matching on the path keeps the assertion immune to the outbox workers logging
// into the same buffer.
func failedRequestLine(buf *bytes.Buffer, path string) string {
	for _, ln := range strings.Split(buf.String(), "\n") {
		if strings.Contains(ln, `msg="request failed"`) && strings.Contains(ln, path) {
			return ln
		}
	}
	return ""
}

func TestGitlabPushMilestoneLogsLinkFailureCause(t *testing.T) {
	buf := captureDefaultSlog(t)
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-mscause-user", "grp-mscause")

	connectGitlab(t, c, f)
	createIntegration(t, c, s.WS, s.Board, f, nil)

	ms := c.expect(t, c.post("/projects/"+s.Project+"/milestones", map[string]any{
		"title": "Doomed sprint",
	}), http.StatusCreated)
	msID := ms["id"].(string)

	// Delete the milestone while the push is out at GitLab: a second tab, or a
	// slow instance, is all this takes. The remote milestone gets created, then
	// the link insert hits the foreign key to a row that is gone.
	f.mu.Lock()
	f.onCreateMilestone = func() {
		if r := c.del("/milestones/" + msID); r.Status != http.StatusNoContent {
			t.Errorf("delete milestone mid-push: status %d\n%s", r.Status, r.Body)
		}
	}
	f.mu.Unlock()

	path := "/milestones/" + msID + "/gitlab"
	r := c.post(path, nil)
	if r.Status != http.StatusInternalServerError {
		t.Fatalf("push after the milestone vanished: status %d, want 500\n%s", r.Status, r.Body)
	}
	if strings.Contains(string(r.Body), "gitlab_milestone_links") {
		t.Fatalf("database detail leaked to the client: %s", r.Body)
	}

	line := failedRequestLine(buf, path)
	if line == "" {
		t.Fatalf("no `request failed` log line for %s:\n%s", path, buf.String())
	}
	// The bug: the handler passed the (already nil-checked) GitLab error instead
	// of the link insert's own, so this line named no cause at all.
	if strings.Contains(line, "err=<nil>") {
		t.Fatalf("link failure logged without a cause:\n%s", line)
	}
	if !strings.Contains(line, "gitlab_milestone_links") {
		t.Fatalf("log line does not name the failing insert:\n%s", line)
	}
}
