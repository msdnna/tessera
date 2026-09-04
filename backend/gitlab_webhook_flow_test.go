// GitLab webhook receiver flow (#2594): the public delivery endpoint against a
// real database — secret handling, authentication, payload classification and the
// last_webhook_at stamp. The debounce/queue arithmetic is unit-tested next to the
// queue itself (handlers/gitlab_webhook_test.go); here we exercise the HTTP
// surface end to end, including that the secret is shown exactly once.
package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// postHook sends a raw delivery to the public webhook endpoint, the way GitLab
// does: no session, a shared secret in X-Gitlab-Token, a JSON body.
func postHook(t *testing.T, integID, token, body string) resp {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/gitlab/webhook/"+integID, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Gitlab-Token", token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post hook: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

func TestGitlabWebhookFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-hook-user", "grp-hook")
	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)

	issueBody := `{"object_kind":"issue","project":{"path_with_namespace":"grp-hook"},"object_attributes":{"iid":1,"action":"update"}}`

	// Before the hook is configured the endpoint must not even admit the binding
	// exists — 404, not 401/403, so it can't be used to enumerate integration ids.
	if r := postHook(t, integID, "whatever", issueBody); r.Status != http.StatusNotFound {
		t.Fatalf("delivery to a hook-less binding: %d, want 404\n%s", r.Status, r.Body)
	}

	// Enabling returns the URL and the secret — this is the only time the secret is
	// ever readable.
	enabled := c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/webhook", nil), http.StatusOK)
	secret, _ := enabled["secret"].(string)
	if len(secret) != 64 { // 32 random bytes, hex
		t.Fatalf("secret = %q, want 64 hex chars", secret)
	}
	view, _ := enabled["integration"].(map[string]any)
	if view == nil || view["webhook_enabled"] != true {
		t.Fatalf("integration view after enable: %v", view)
	}
	if url, _ := view["webhook_url"].(string); url == "" || !bytes.Contains([]byte(url), []byte(integID)) {
		t.Fatalf("webhook_url = %q, want the delivery URL for %s", view["webhook_url"], integID)
	}

	// …and it is not readable afterwards, from any authenticated view.
	listed := c.get("/workspaces/" + s.WS + "/gitlab/integrations")
	if bytes.Contains(listed.Body, []byte(secret)) {
		t.Fatal("the webhook secret leaked into the integrations list")
	}

	// Wrong token → 401. The endpoint is public, so this is the only thing standing
	// between GitLab and an attacker.
	if r := postHook(t, integID, secret+"x", issueBody); r.Status != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d, want 401\n%s", r.Status, r.Body)
	}
	// An empty token must not match an empty comparison.
	if r := postHook(t, integID, "", issueBody); r.Status != http.StatusUnauthorized {
		t.Fatalf("missing token: %d, want 401\n%s", r.Status, r.Body)
	}

	// An event for another project, delivered with our token (hook URL pasted into
	// the wrong project) → 400, never a sync of somebody else's issues.
	foreign := `{"object_kind":"issue","project":{"path_with_namespace":"someone/else"}}`
	if r := postHook(t, integID, secret, foreign); r.Status != http.StatusBadRequest {
		t.Fatalf("foreign project: %d, want 400\n%s", r.Status, r.Body)
	}

	// An event kind we don't mirror → 204. Not an error: GitLab disables a hook that
	// keeps failing, and an over-broad event selection is the user's to narrow.
	push := `{"object_kind":"push","project":{"path_with_namespace":"grp-hook"}}`
	if r := postHook(t, integID, secret, push); r.Status != http.StatusNoContent {
		t.Fatalf("push event: %d, want 204\n%s", r.Status, r.Body)
	}

	// The real thing: accepted immediately (GitLab times out after 10 s) and
	// stamped, so the UI can tell a live hook from a silent one.
	if r := postHook(t, integID, secret, issueBody); r.Status != http.StatusAccepted {
		t.Fatalf("issue event: %d, want 202\n%s", r.Status, r.Body)
	}
	after := c.expect(t, c.get("/workspaces/"+s.WS+"/gitlab/integrations"), http.StatusOK)
	got := integrationFromList(t, after, integID)
	if got["last_webhook_at"] == nil {
		t.Fatalf("last_webhook_at not stamped after an accepted delivery: %v", got)
	}

	// Disabling wipes the secret, so a delivery still carrying it is refused rather
	// than silently accepted.
	c.expect(t, c.del("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/webhook"), http.StatusOK)
	if r := postHook(t, integID, secret, issueBody); r.Status != http.StatusNotFound {
		t.Fatalf("delivery after disable: %d, want 404\n%s", r.Status, r.Body)
	}

	// Re-enabling issues a NEW secret; the old one must not work.
	again := c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/webhook", nil), http.StatusOK)
	rotated, _ := again["secret"].(string)
	if rotated == secret {
		t.Fatal("rotation returned the same secret")
	}
	if r := postHook(t, integID, secret, issueBody); r.Status != http.StatusUnauthorized {
		t.Fatalf("old secret after rotation: %d, want 401\n%s", r.Status, r.Body)
	}
	if r := postHook(t, integID, rotated, issueBody); r.Status != http.StatusAccepted {
		t.Fatalf("rotated secret: %d, want 202\n%s", r.Status, r.Body)
	}
}

// integrationFromList picks one binding out of the list envelope.
func integrationFromList(t *testing.T, list map[string]any, integID string) map[string]any {
	t.Helper()
	items, _ := list["integrations"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m != nil && m["id"] == integID {
			return m
		}
	}
	t.Fatalf("integration %s not in list: %v", integID, list)
	return nil
}
