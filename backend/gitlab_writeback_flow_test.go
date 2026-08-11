// GitLab write-back flow tests: user-side changes (column move, priority,
// comment, completion) are enqueued into the gitlab_writebacks outbox and a
// worker drain pushes them to the fake GitLab as REST mutations. Also covers
// the retry path (500 → retried → journal fail → retry endpoint → sent) and
// the OAuth surface (public providers/authorize/callback + admin config).
//
// The fake here wraps the fakeGitlab from gitlab_flow_test.go with a recording
// proxy: every REST v4 mutation is captured (method/path/form) and can be made
// to fail with a 500, while reads (GraphQL, members) delegate to the inner fake.
package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── recording write-back fake ────────────────────────────────────────────────

type restCall struct {
	Method string
	Path   string
	Form   url.Values
	Sudo   string // the Sudo header (impersonation), "" when absent
}

// wbFake fronts a fakeGitlab with mutation capture + failure injection. Its own
// server URL is what the test connects Tessera to; reads pass through.
type wbFake struct {
	*fakeGitlab
	outer *httptest.Server

	wmu   sync.Mutex
	calls []restCall
	fail  int // fail the next N REST mutations with a 500
}

func newWBFake(t *testing.T, username, projectPath string) *wbFake {
	t.Helper()
	w := &wbFake{fakeGitlab: newFakeGitlab(t, username, projectPath)}
	w.outer = httptest.NewServer(w)
	t.Cleanup(w.outer.Close)
	return w
}

func (w *wbFake) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/") {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		w.wmu.Lock()
		w.calls = append(w.calls, restCall{Method: r.Method, Path: r.URL.Path, Form: form, Sudo: r.Header.Get("Sudo")})
		failNow := w.fail > 0
		if failNow {
			w.fail--
		}
		w.wmu.Unlock()
		if failNow {
			writeJSON(rw, http.StatusInternalServerError, map[string]any{"message": "boom"})
			return
		}
		// Notes get a real id so CreateIssueNote can tag the source comment.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes") {
			writeJSON(rw, http.StatusCreated, map[string]any{"id": 4242})
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}
	w.fakeGitlab.ServeHTTP(rw, r)
}

func (w *wbFake) setFail(n int) {
	w.wmu.Lock()
	w.fail = n
	w.wmu.Unlock()
}

// callsMatching returns the captured mutations whose path ends with suffix.
func (w *wbFake) callsMatching(method, suffix string) []restCall {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	var out []restCall
	for _, c := range w.calls {
		if c.Method == method && strings.HasSuffix(c.Path, suffix) {
			out = append(out, c)
		}
	}
	return out
}

// ── stand + drain helpers ────────────────────────────────────────────────────

// wbStand is the assembled write-back fixture: a synced task linked to fake
// issue !1 on an integration owned by an admin user.
type wbStand struct {
	c       *client
	s       stack
	w       *wbFake
	integID string
	taskID  string
}

// newWritebackStand builds workspace+board, connects the fake via the recording
// proxy URL, creates the integration and syncs one bare issue (!1) into a task.
func newWritebackStand(t *testing.T, username, projectPath string) wbStand {
	t.Helper()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	w := newWBFake(t, username, projectPath)
	w.addIssue(glIssue{IID: 1, Title: "WB target"}) // no labels/due/milestone/notes

	m := c.expect(t, c.post("/gitlab/connection", map[string]any{
		"base_url": w.outer.URL, "token": w.token, // proxy URL, inner token
	}), http.StatusOK)
	if m["connected"] != true {
		t.Fatalf("connect: %v", m)
	}
	integ := createIntegration(t, c, s.WS, s.Board, w.fakeGitlab, nil)
	integID := integ["id"].(string)

	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	task := taskByIid(t, tasks, 1)
	return wbStand{c: c, s: s, w: w, integID: integID, taskID: task["id"].(string)}
}

// putBindings replaces the integration's write-back config with an explicit
// binding table (explicit bindings take over completely from the legacy flags).
func (st wbStand) putBindings(t *testing.T, bindings []map[string]any) {
	t.Helper()
	st.c.expect(t, st.c.put("/workspaces/"+st.s.WS+"/gitlab/integrations/"+st.integID, map[string]any{
		"name": "GL", "project_path": st.w.projectPath, "board_id": st.s.Board,
		"enabled": true, "scope": "all",
		"writeback": map[string]any{"enabled": true, "bindings": bindings},
	}), http.StatusOK)
}

// wbRow is one outbox row of the task under test.
type wbRow struct {
	Kind, Status, LastError string
	Attempts                int32
}

func writebackRows(t *testing.T, taskID string) map[string]wbRow {
	t.Helper()
	rows, err := testPool.Query(context.Background(),
		`SELECT change_kind, status, last_error, attempts FROM gitlab_writebacks WHERE task_id = $1::uuid`, taskID)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	out := map[string]wbRow{}
	for rows.Next() {
		var r wbRow
		if err := rows.Scan(&r.Kind, &r.Status, &r.LastError, &r.Attempts); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		// Keyed by kind; a retried kind may have several rows — prefer a sent one.
		if prev, ok := out[r.Kind]; !ok || prev.Status != "sent" {
			out[r.Kind] = r
		}
	}
	return out
}

// drainOutboxUntil spawns the write-back worker (which drains once at startup)
// and polls cond; respawns a few times so a row that appeared between drains is
// still picked up. Fails the test when cond never holds.
func drainOutboxUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			testAPI.RunGitlabWriteBackWorker(ctx)
			close(done)
		}()
		ok := false
		for i := 0; i < 20 && !ok; i++ {
			if cond() {
				ok = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
		<-done
		if ok {
			return
		}
	}
	t.Fatalf("write-back drain condition never met")
}

// ── tests ────────────────────────────────────────────────────────────────────

// Happy path: column move → set_label, priority → set_label (P: swap), comment
// → post_comment, completion → set_state; all rows end sent, the fake receives
// the REST mutations, and the push journal records four ok actions.
func TestGitlabWritebackPushFlow(t *testing.T) {
	t.Parallel()
	st := newWritebackStand(t, "gl-wbpush-user", "grp-wbpush")
	c, s, w := st.c, st.s, st.w

	// The priority binding is level-qualified (priority: 3) on purpose — the
	// enqueue gate (triggerFromKind) must read the int32 the handler puts into
	// the in-memory payload, not just JSON float64.
	st.putBindings(t, []map[string]any{
		{"enabled": true,
			"trigger": map[string]any{"type": "column", "column_id": s.col(t, 1), "column_name": "В процессе"},
			"action":  map[string]any{"type": "set_label", "label": "S: In progress", "clear_prefix": true}},
		{"enabled": true,
			"trigger": map[string]any{"type": "priority", "priority": 3},
			"action":  map[string]any{"type": "set_label", "label": "P: High", "clear_prefix": true}},
		{"enabled": true,
			"trigger": map[string]any{"type": "comment"},
			"action":  map[string]any{"type": "post_comment"}},
		{"enabled": true,
			"trigger": map[string]any{"type": "completion"},
			"action":  map[string]any{"type": "set_state"}},
	})

	// Four user-side changes, one per trigger kind.
	c.expect(t, c.patch("/tasks/"+st.taskID+"/move", map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+st.taskID, map[string]any{
		"title": "WB target", "priority": 3,
	}), http.StatusOK)
	r := c.post("/tasks/"+st.taskID+"/comments", map[string]any{"body": "Привет из Tessera"})
	if r.Status != http.StatusCreated {
		t.Fatalf("comment: status %d\n%s", r.Status, r.Body)
	}
	c.expect(t, c.patch("/tasks/"+st.taskID, map[string]any{
		"title": "WB target", "priority": 3, "completed": true,
	}), http.StatusOK)

	// All four kinds are queued (a parallel worker may already be draining them,
	// so only the presence of the rows is asserted here, not "pending").
	if rows := writebackRows(t, st.taskID); len(rows) != 4 {
		t.Fatalf("outbox rows = %v, want 4 kinds", rows)
	}

	kinds := []string{"column", "priority", "comment", "completion"}
	drainOutboxUntil(t, func() bool {
		rows := writebackRows(t, st.taskID)
		for _, k := range kinds {
			if rows[k].Status != "sent" {
				return false
			}
		}
		return true
	})

	// The fake received the expected REST mutations.
	puts := w.callsMatching(http.MethodPut, "/issues/1")
	var sawStatusLabel, sawPrioLabel, sawClose bool
	for _, call := range puts {
		switch {
		case call.Form.Get("add_labels") == "S: In progress":
			sawStatusLabel = true
		case call.Form.Get("add_labels") == "P: High":
			sawPrioLabel = true
			// clear_prefix swaps out every other P: label from the rule map.
			if rm := call.Form.Get("remove_labels"); !strings.Contains(rm, "P: Critical") {
				t.Fatalf("priority push remove_labels = %q, want P: siblings", rm)
			}
		case call.Form.Get("state_event") == "close":
			sawClose = true
		}
	}
	if !sawStatusLabel || !sawPrioLabel || !sawClose {
		t.Fatalf("missing pushes: status=%v prio=%v close=%v\ncalls: %+v", sawStatusLabel, sawPrioLabel, sawClose, puts)
	}
	notes := w.callsMatching(http.MethodPost, "/issues/1/notes")
	if len(notes) != 1 || notes[0].Form.Get("body") != "Привет из Tessera" {
		t.Fatalf("note pushes: %+v", notes)
	}

	// Journal: push run(s) with four ok push-actions in total.
	pushActions := 0
	for _, run := range c.get("/workspaces/"+s.WS+"/gitlab/sync-runs").listBody(t) {
		if run["kind"] != "push" {
			continue
		}
		for _, a := range c.get("/workspaces/" + s.WS + "/gitlab/sync-runs/" + run["id"].(string) + "/actions").itemsBody(t) {
			if a["direction"] != "push" || a["op"] != "push" || a["status"] != "ok" {
				t.Fatalf("push action: %v", a)
			}
			pushActions++
		}
	}
	if pushActions != 4 {
		t.Fatalf("push actions = %d, want 4", pushActions)
	}
}

// Sudo impersonation (task #2690): when the admin enables sudo write-back and the
// acting user has no personal PAT but a known GitLab identity, the async push runs
// under the service (admin) token with a `Sudo: <username>` header so GitLab
// attributes the write to the real user. Also asserts the pull never carries Sudo.
func TestGitlabWritebackSudoImpersonation(t *testing.T) {
	// Not parallel: it flips the instance-wide OAuth provider (service token + sudo),
	// which is shared state other GitLab tests read.
	st := newWritebackStand(t, "gl-sudo-user", "grp-sudo")
	c, s, w := st.c, st.s, st.w

	// oauth_providers is a global singleton; clear the service token afterwards so it
	// doesn't leak into the parallel tests (which run after this sequential one).
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`UPDATE oauth_providers SET service_token_enc = '', sudo_writeback = false, gl_base_url = '', enabled = false WHERE provider = 'gitlab'`)
	})

	// Give the acting user a GitLab identity via OAuth (no personal PAT), so their
	// username resolves for impersonation without short-circuiting to a personal token.
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO oauth_identities (user_id, provider, provider_user_id, provider_username, gl_base_url)
		 VALUES ($1::uuid, 'gitlab', '90210', 'gl-sudo-user', $2)`, c.UserID, w.outer.URL); err != nil {
		t.Fatalf("insert oauth identity: %v", err)
	}
	// Drop the personal PAT so writeGitlabConn falls through to service-token + sudo.
	if r := c.del("/gitlab/connection"); r.Status != http.StatusNoContent {
		t.Fatalf("disconnect PAT: status %d\n%s", r.Status, r.Body)
	}
	// Admin: configure the instance service token (the fake) and turn sudo on.
	c.expect(t, c.put("/admin/oauth/gitlab", map[string]any{
		"gl_base_url": w.outer.URL, "service_token": w.token,
		"enabled": true, "sudo_writeback": true,
	}), http.StatusOK)

	st.putBindings(t, []map[string]any{
		{"enabled": true,
			"trigger": map[string]any{"type": "column", "column_id": s.col(t, 1), "column_name": "В процессе"},
			"action":  map[string]any{"type": "set_label", "label": "S: In progress", "clear_prefix": true}},
	})

	c.expect(t, c.patch("/tasks/"+st.taskID+"/move", map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)

	drainOutboxUntil(t, func() bool { return writebackRows(t, st.taskID)["column"].Status == "sent" })

	puts := w.callsMatching(http.MethodPut, "/issues/1")
	if len(puts) == 0 {
		t.Fatalf("no label push captured")
	}
	for _, call := range puts {
		if call.Sudo != "gl-sudo-user" {
			t.Fatalf("write Sudo header = %q, want gl-sudo-user\ncall: %+v", call.Sudo, call)
		}
	}
	// The pull side (issue fetch, GET/GraphQL) must never impersonate.
	w.wmu.Lock()
	for _, call := range w.calls {
		if call.Method == http.MethodGet && call.Sudo != "" {
			w.wmu.Unlock()
			t.Fatalf("read call carried Sudo=%q (pull must not impersonate): %+v", call.Sudo, call)
		}
	}
	w.wmu.Unlock()
}

// Retry path: the first push gets a 500 → the row is backed off (pending again,
// attempts=1, error kept) and the journal records a failed push action; the
// retry endpoint re-enqueues it and a healthy drain delivers it.
func TestGitlabWritebackRetry(t *testing.T) {
	t.Parallel()
	st := newWritebackStand(t, "gl-wbretry-user", "grp-wbretry")
	c, s, w := st.c, st.s, st.w

	st.putBindings(t, []map[string]any{
		{"enabled": true,
			"trigger": map[string]any{"type": "column", "column_id": s.col(t, 1), "column_name": "В процессе"},
			"action":  map[string]any{"type": "set_label", "label": "S: In progress", "clear_prefix": true}},
	})

	w.setFail(1) // next mutation → 500
	c.expect(t, c.patch("/tasks/"+st.taskID+"/move", map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)

	// Drain: the 500 is transient → back to pending with backoff + error.
	drainOutboxUntil(t, func() bool {
		row := writebackRows(t, st.taskID)["column"]
		return row.Attempts >= 1 && row.Status == "pending" && row.LastError != ""
	})

	// Find the failed push action in the journal (flushed just after settle).
	var runID, actionID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && actionID == "" {
		for _, run := range c.get("/workspaces/"+s.WS+"/gitlab/sync-runs").listBody(t) {
			if run["kind"] != "push" {
				continue
			}
			for _, a := range c.get("/workspaces/" + s.WS + "/gitlab/sync-runs/" + run["id"].(string) + "/actions").itemsBody(t) {
				if a["status"] == "fail" && a["entity_type"] == "column" {
					runID, actionID = run["id"].(string), a["id"].(string)
				}
			}
		}
		if actionID == "" {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if actionID == "" {
		t.Fatalf("no failed push action in journal")
	}

	// Re-enqueue from the journal; the fake is healthy now.
	m := c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/sync-runs/"+runID+"/actions/"+actionID+"/retry", nil), http.StatusOK)
	if m["status"] != "queued" {
		t.Fatalf("retry: %v", m)
	}
	drainOutboxUntil(t, func() bool {
		return writebackRows(t, st.taskID)["column"].Status == "sent"
	})

	// Both attempts hit the fake; the successful one carried the label.
	puts := w.callsMatching(http.MethodPut, "/issues/1")
	if len(puts) < 2 {
		t.Fatalf("issue PUTs = %d, want ≥2 (failed + retried)", len(puts))
	}
	if got := puts[len(puts)-1].Form.Get("add_labels"); got != "S: In progress" {
		t.Fatalf("retried push add_labels = %q", got)
	}
}

// ── OAuth surface ────────────────────────────────────────────────────────────

// noRedirect performs a GET without following redirects (the OAuth handlers
// answer with 302s whose Location carries the outcome).
func noRedirect(t *testing.T, path string) (int, string) {
	t.Helper()
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := cl.Get(testServer.URL + "/api" + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode, res.Header.Get("Location")
}

// OAuth endpoints without config, then the admin config round-trip (secret is
// write-only) and the configured authorize redirect.
func TestOAuthProvidersAndAdminConfig(t *testing.T) {
	t.Parallel()
	c := signup(t)

	// Unconfigured: providers reports gitlab=false; authorize/callback bounce to
	// the SPA login with an oauth_error instead of failing hard.
	m := c.expect(t, doReq(t, "", http.MethodGet, "/auth/providers", nil), http.StatusOK)
	if m["gitlab"] != false {
		t.Fatalf("providers unconfigured: %v", m)
	}
	status, loc := noRedirect(t, "/auth/gitlab/authorize")
	if status != http.StatusFound || !strings.Contains(loc, "oauth_error=not_configured") {
		t.Fatalf("authorize unconfigured: %d %s", status, loc)
	}
	status, loc = noRedirect(t, "/auth/gitlab/callback?state=garbage&code=x")
	if status != http.StatusFound || !strings.Contains(loc, "oauth_error=state_mismatch") {
		t.Fatalf("callback garbage state: %d %s", status, loc)
	}

	// Admin config is admin-gated. Demote explicitly first — under a filtered
	// run this signup may be the instance's very first user (auto-admin).
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = FALSE WHERE id = $1::uuid`, c.UserID); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if r := c.get("/admin/oauth/gitlab"); r.Status != http.StatusForbidden {
		t.Fatalf("oauth config as non-admin: status %d", r.Status)
	}
	makeAdmin(t, c)
	m = c.expect(t, c.get("/admin/oauth/gitlab"), http.StatusOK)
	if m["provider"] != "gitlab" || m["has_secret"] != false {
		t.Fatalf("empty oauth config: %v", m)
	}

	// Set the config; the response must mask the secret (has_secret only).
	r := c.put("/admin/oauth/gitlab", map[string]any{
		"client_id": "test-client", "client_secret": "s3cret-value",
		"gl_base_url": "https://gl.example.test/", "enabled": true, "org_map": map[string]any{},
	})
	m = c.expect(t, r, http.StatusOK)
	if m["client_id"] != "test-client" || m["gl_base_url"] != "https://gl.example.test" ||
		m["enabled"] != true || m["has_secret"] != true {
		t.Fatalf("set oauth config: %v", m)
	}
	if strings.Contains(string(r.Body), "s3cret-value") {
		t.Fatalf("secret leaked in response: %s", r.Body)
	}

	// Providers flips on; authorize now redirects to the GitLab authorize URL.
	m = c.expect(t, doReq(t, "", http.MethodGet, "/auth/providers", nil), http.StatusOK)
	if m["gitlab"] != true {
		t.Fatalf("providers configured: %v", m)
	}
	status, loc = noRedirect(t, "/auth/gitlab/authorize")
	if status != http.StatusFound || !strings.HasPrefix(loc, "https://gl.example.test/oauth/authorize?") ||
		!strings.Contains(loc, "client_id=test-client") {
		t.Fatalf("authorize configured: %d %s", status, loc)
	}

	// Re-save without a secret: the stored one is kept; disable to restore the
	// unconfigured providers surface for other tests.
	m = c.expect(t, c.put("/admin/oauth/gitlab", map[string]any{
		"client_id": "test-client", "gl_base_url": "https://gl.example.test", "enabled": false,
	}), http.StatusOK)
	if m["has_secret"] != true || m["enabled"] != false {
		t.Fatalf("re-save oauth config: %v", m)
	}
	m = c.expect(t, doReq(t, "", http.MethodGet, "/auth/providers", nil), http.StatusOK)
	if m["gitlab"] != false {
		t.Fatalf("providers after disable: %v", m)
	}
}
