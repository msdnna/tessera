// Running CI from a command (#3099): «>deploy» starts a pipeline of the board's
// GitLab project on a ref with variables, «>ci» retries the last failed one.
package main

import (
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestRunCI(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	stranger := signup(t)
	st := mkStack(t, owner)
	f := newFakeGitlab(t, "ci-user", "team/web")
	var mu sync.Mutex
	var created, retried string
	f.restHook = func(w http.ResponseWriter, r *http.Request) bool {
		p := r.URL.EscapedPath()
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/projects/team%2Fweb/pipeline"):
			_ = r.ParseForm()
			mu.Lock()
			created = r.Form.Get("ref") + "|" + strings.Join(r.Form["variables[][key]"], ",") + "=" + strings.Join(r.Form["variables[][value]"], ",")
			mu.Unlock()
			writeJSON(w, http.StatusCreated, map[string]any{"id": 77, "status": "created", "ref": r.Form.Get("ref"), "web_url": f.url() + "/team/web/-/pipelines/77"})
			return true
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/projects/team%2Fweb/pipelines"):
			if r.URL.Query().Get("status") != "failed" {
				return false
			}
			writeJSON(w, http.StatusOK, []map[string]any{{"id": 5, "status": "failed", "ref": "main"}})
			return true
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/projects/team%2Fweb/pipelines/5/retry"):
			mu.Lock()
			retried = "5"
			mu.Unlock()
			writeJSON(w, http.StatusCreated, map[string]any{"id": 5, "status": "pending", "ref": "main"})
			return true
		}
		return false
	}
	makeAdmin(t, owner) // binding a project to a board is instance-admin work
	connectGitlab(t, owner, f)
	createIntegration(t, owner, st.WS, st.Board, f, nil)

	run := owner.expect(t, owner.post("/boards/"+st.Board+"/ci/run", map[string]any{
		"mode": "run", "ref": "main", "variables": map[string]string{"ENV": "staging"},
	}), http.StatusOK)
	if run["id"].(float64) != 77 || run["provider"] != "gitlab" || run["project"] != "team/web" {
		t.Fatalf("run: %v", run)
	}
	mu.Lock()
	if created != "main|ENV=staging" {
		t.Fatalf("GitLab got %q", created)
	}
	mu.Unlock()

	again := owner.expect(t, owner.post("/boards/"+st.Board+"/ci/run", map[string]any{"mode": "retry_failed"}), http.StatusOK)
	if again["status"] != "pending" || retried != "5" {
		t.Fatalf("retry: %v (retried %q)", again, retried)
	}

	if r := owner.post("/boards/"+st.Board+"/ci/run", map[string]any{"mode": "deploy"}); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown mode: %d", r.Status)
	}
	if r := stranger.post("/boards/"+st.Board+"/ci/run", map[string]any{"mode": "run"}); r.Status != http.StatusForbidden && r.Status != http.StatusNotFound {
		t.Fatalf("stranger: %d", r.Status)
	}
}
