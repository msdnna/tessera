// GitLab integration flow tests: a fake GitLab server (GraphQL + REST v4)
// backs the connection, integration CRUD, sync (pull), issue templates,
// create-issue-from-task and milestone push endpoints.
//
// The real client (internal/gitlab) reads issues via GraphQL (/api/graphql,
// cursor pagination) and writes via REST v4 (form-encoded), so the fake
// implements both surfaces. Each test spins its own fake with a unique GitLab
// username + project path, so parallel tests never cross-resolve credentials.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── fake GitLab server ───────────────────────────────────────────────────────

type glLabel struct{ Title, Color string }

type glNote struct {
	ID          int64
	Body        string
	System      bool
	AuthorLogin string
	CreatedAt   time.Time
}

type glMilestone struct {
	ID        int64
	IID       int64
	Title     string
	State     string // active | closed
	StartDate string // YYYY-MM-DD or ""
	DueDate   string
}

type glIssue struct {
	ID             int64
	IID            int64
	Title          string
	Description    string
	State          string // opened | closed
	DueDate        string // YYYY-MM-DD or ""
	Labels         []glLabel
	AssigneeLogins []string
	Milestone      *glMilestone
	Notes          []glNote
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type glMember struct {
	ID       int64
	Username string
	Name     string
}

// graphqlLinkType translates a REST link_type (how fixtures are written) into the
// WorkItemRelatedLinkType enum the GraphQL widget actually emits. Unknown values
// pass through unchanged so a test can still assert they're ignored.
func graphqlLinkType(rest string) string {
	switch rest {
	case "relates_to":
		return "RELATED"
	case "blocks":
		return "BLOCKS"
	case "is_blocked_by":
		return "BLOCKED_BY"
	default:
		return rest
	}
}

// glLink is one issue-link edge as GitLab reports it from the source issue's side.
// ProjectPath empty means "same project as the fake".
type glLink struct {
	SrcIID      int64
	DstIID      int64
	LinkType    string // relates_to | blocks | is_blocked_by
	ProjectPath string
	LinkID      int64
}

// fakeGitlab is an in-memory GitLab instance serving the exact API subset the
// backend's client uses. pageSize < len(issues) exercises GraphQL pagination.
type fakeGitlab struct {
	t           *testing.T
	srv         *httptest.Server
	username    string // the identity the PAT resolves to
	token       string
	projectPath string
	pageSize    int

	mu sync.Mutex
	// Artificial per-request latency, so a test can catch a sync mid-flight (the
	// manual sync is detached — its "running" state is what the UI shows).
	delay     time.Duration
	issues    []*glIssue
	members   []glMember
	templates map[string]string // name → content
	nextIID   int64
	nextMsID  int64

	// Issue links, plus the switch that decides which of the two client paths serves
	// them: with linkedItemsWidget the GraphQL widget answers, without it the widget
	// is absent from the response (an older self-hosted GitLab) and the client must
	// fall back to REST. linkCalls counts either kind of request, so a test can prove
	// relations_sync=off costs no round-trips at all.
	links             []glLink
	linkedItemsWidget bool
	linkCalls         int
}

func newFakeGitlab(t *testing.T, username, projectPath string) *fakeGitlab {
	t.Helper()
	f := &fakeGitlab{
		t: t, username: username, token: "glpat-test-" + username,
		projectPath: projectPath, pageSize: 2,
		templates: map[string]string{}, nextIID: 100, nextMsID: 9000,
		linkedItemsWidget: true,
	}
	f.srv = httptest.NewServer(f)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitlab) url() string { return f.srv.URL }

func (f *fakeGitlab) issueURL(iid int64) string {
	return f.srv.URL + "/" + f.projectPath + "/-/issues/" + strconv.FormatInt(iid, 10)
}

// addIssue registers an issue with an auto id derived from its iid.
func (f *fakeGitlab) addIssue(is glIssue) *glIssue {
	f.mu.Lock()
	defer f.mu.Unlock()
	if is.ID == 0 {
		is.ID = 1000 + is.IID
	}
	if is.State == "" {
		is.State = "opened"
	}
	if is.CreatedAt.IsZero() {
		is.CreatedAt = time.Now().Add(-24 * time.Hour).UTC()
	}
	if is.UpdatedAt.IsZero() {
		is.UpdatedAt = time.Now().UTC()
	}
	cp := is
	f.issues = append(f.issues, &cp)
	return &cp
}

func (f *fakeGitlab) findIssue(iid int64) *glIssue {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, is := range f.issues {
		if is.IID == iid {
			return is
		}
	}
	return nil
}

// setDelay makes every fake response take d, stretching a sync into a window a
// test can observe.
func (f *fakeGitlab) setDelay(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delay = d
}

func (f *fakeGitlab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	d := f.delay
	f.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
	switch {
	case r.URL.Path == "/api/graphql":
		f.graphql(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v4/"):
		f.rest(w, r)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// graphql dispatches on the query text (the client uses three fixed queries).
func (f *fakeGitlab) graphql(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "401 Unauthorized"})
		return
	}
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "bad body"})
		return
	}
	switch {
	case strings.Contains(req.Query, "currentUser"):
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"currentUser": map[string]any{"id": "gid://gitlab/User/99", "username": f.username},
		}})
	case strings.Contains(req.Query, "linkedItems"):
		f.linkedItemsQuery(w, req.Variables)
	case strings.Contains(req.Query, "workItems"):
		// Child-item hierarchy: none of the fixture issues group subtasks.
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"project": map[string]any{"workItems": map[string]any{"nodes": []any{}}},
		}})
	default:
		f.issuesQuery(w, req.Variables)
	}
}

// linkedItemsQuery serves the WorkItemWidgetLinkedItems batch. When the fake is
// configured without the widget, the work items come back carrying only an unrelated
// widget — exactly what an older GitLab returns, and the client's cue to use REST.
func (f *fakeGitlab) linkedItemsQuery(w http.ResponseWriter, vars map[string]any) {
	f.mu.Lock()
	f.linkCalls++
	widget := f.linkedItemsWidget
	f.mu.Unlock()

	raw, _ := vars["iids"].([]any)
	nodes := make([]any, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			continue
		}
		iid, _ := strconv.ParseInt(s, 10, 64)
		if !widget {
			nodes = append(nodes, map[string]any{"iid": s, "widgets": []any{map[string]any{}}})
			continue
		}
		items := make([]any, 0)
		for _, l := range f.linksFor(iid) {
			path := l.ProjectPath
			if path == "" {
				path = f.projectPath
			}
			items = append(items, map[string]any{
				// The GraphQL widget reports the WorkItemRelatedLinkType enum
				// (RELATED/BLOCKS/BLOCKED_BY), NOT the REST link_type the fixture is
				// written in — mirroring real GitLab so this path exercises the enum.
				"linkType": graphqlLinkType(l.LinkType),
				"linkId":   fmt.Sprintf("gid://gitlab/IssueLink/%d", l.LinkID),
				"workItem": map[string]any{
					"iid":       strconv.FormatInt(l.DstIID, 10),
					"webUrl":    f.srv.URL + "/" + path + "/-/issues/" + strconv.FormatInt(l.DstIID, 10),
					"namespace": map[string]any{"fullPath": path},
				},
			})
		}
		nodes = append(nodes, map[string]any{
			"iid": s,
			"widgets": []any{
				map[string]any{},
				map[string]any{"linkedItems": map[string]any{"nodes": items}},
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"project": map[string]any{"workItems": map[string]any{"nodes": nodes}},
	}})
}

func (f *fakeGitlab) linksFor(srcIID int64) []glLink {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []glLink
	for _, l := range f.links {
		if l.SrcIID == srcIID {
			out = append(out, l)
		}
	}
	return out
}

// issuesQuery serves the paginated issues query with optional iids / assignee
// username filters (mirroring the real GraphQL filter semantics).
func (f *fakeGitlab) issuesQuery(w http.ResponseWriter, vars map[string]any) {
	f.mu.Lock()
	var filtered []*glIssue
	if raw, ok := vars["iids"].([]any); ok && raw != nil {
		want := map[int64]bool{}
		for _, v := range raw {
			if s, ok := v.(string); ok {
				n, _ := strconv.ParseInt(s, 10, 64)
				want[n] = true
			}
		}
		for _, is := range f.issues {
			if want[is.IID] {
				filtered = append(filtered, is)
			}
		}
	} else if uname, ok := vars["username"].(string); ok && uname != "" {
		for _, is := range f.issues {
			for _, a := range is.AssigneeLogins {
				if a == uname {
					filtered = append(filtered, is)
					break
				}
			}
		}
	} else {
		filtered = append(filtered, f.issues...)
	}
	f.mu.Unlock()

	start := 0
	if after, ok := vars["after"].(string); ok && after != "" {
		start, _ = strconv.Atoi(after)
	}
	end := start + f.pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	if start > end {
		start = end
	}
	nodes := make([]any, 0, end-start)
	for _, is := range filtered[start:end] {
		nodes = append(nodes, f.issueNode(is))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"project": map[string]any{"issues": map[string]any{
			"pageInfo": map[string]any{
				"hasNextPage": end < len(filtered),
				"endCursor":   strconv.Itoa(end),
			},
			"nodes": nodes,
		}},
	}})
}

// issueNode renders one issue in the GraphQL node shape the client decodes.
func (f *fakeGitlab) issueNode(is *glIssue) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	labels := make([]any, 0, len(is.Labels))
	for _, l := range is.Labels {
		labels = append(labels, map[string]any{"title": l.Title, "color": l.Color})
	}
	assignees := make([]any, 0, len(is.AssigneeLogins))
	for _, a := range is.AssigneeLogins {
		assignees = append(assignees, map[string]any{"username": a, "name": "User " + a, "avatarUrl": ""})
	}
	notes := make([]any, 0, len(is.Notes))
	for _, n := range is.Notes {
		notes = append(notes, map[string]any{
			"id": fmt.Sprintf("gid://gitlab/Note/%d", n.ID), "body": n.Body, "system": n.System,
			"createdAt": n.CreatedAt.UTC().Format(time.RFC3339),
			"author":    map[string]any{"username": n.AuthorLogin, "name": "User " + n.AuthorLogin, "avatarUrl": ""},
		})
	}
	var ms any
	if m := is.Milestone; m != nil {
		ms = map[string]any{
			"id": fmt.Sprintf("gid://gitlab/Milestone/%d", m.ID), "iid": strconv.FormatInt(m.IID, 10),
			"title": m.Title, "state": m.State, "startDate": m.StartDate, "dueDate": m.DueDate,
			"webPath": "/" + f.projectPath + "/-/milestones/" + strconv.FormatInt(m.IID, 10),
		}
	}
	return map[string]any{
		"id":  fmt.Sprintf("gid://gitlab/Issue/%d", is.ID),
		"iid": strconv.FormatInt(is.IID, 10), "title": is.Title, "description": is.Description,
		"webUrl": f.issueURL(is.IID), "state": is.State,
		"updatedAt": is.UpdatedAt.UTC().Format(time.RFC3339),
		"createdAt": is.CreatedAt.UTC().Format(time.RFC3339),
		"dueDate":   is.DueDate, "timeEstimate": 0, "milestone": ms,
		"author":    map[string]any{"username": f.username, "name": "GL Bot", "avatarUrl": ""},
		"assignees": map[string]any{"nodes": assignees},
		"labels":    map[string]any{"nodes": labels},
		"notes":     map[string]any{"nodes": notes},
	}
}

// rest serves the REST v4 subset: members, issue templates, issue + milestone
// creation. Everything else answers 200 {} so incidental write-backs don't 404.
func (f *fakeGitlab) rest(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != f.token {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "401 Unauthorized"})
		return
	}
	prefix := "/api/v4/projects/" + f.projectPath + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	switch {
	case strings.HasPrefix(rest, "issues/") && strings.HasSuffix(rest, "/links"):
		f.mu.Lock()
		f.linkCalls++
		f.mu.Unlock()
		iid, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(rest, "issues/"), "/links"), 10, 64)
		out := make([]any, 0)
		for _, l := range f.linksFor(iid) {
			path := l.ProjectPath
			if path == "" {
				path = f.projectPath
			}
			out = append(out, map[string]any{
				"iid": l.DstIID, "issue_link_id": l.LinkID, "link_type": l.LinkType,
				"web_url":    f.srv.URL + "/" + path + "/-/issues/" + strconv.FormatInt(l.DstIID, 10),
				"references": map[string]any{"full": fmt.Sprintf("%s#%d", path, l.DstIID)},
			})
		}
		writeJSON(w, http.StatusOK, out)
	case rest == "members/all":
		if r.URL.Query().Get("page") != "1" {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		f.mu.Lock()
		out := make([]any, 0, len(f.members))
		for _, m := range f.members {
			out = append(out, map[string]any{
				"id": m.ID, "username": m.Username, "name": m.Name, "avatar_url": "", "access_level": 30,
			})
		}
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case rest == "templates/issues":
		f.mu.Lock()
		out := make([]any, 0, len(f.templates))
		for name := range f.templates {
			out = append(out, map[string]any{"key": name, "name": name})
		}
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case strings.HasPrefix(rest, "templates/issues/"):
		name := strings.TrimPrefix(rest, "templates/issues/")
		f.mu.Lock()
		content, ok := f.templates[name]
		f.mu.Unlock()
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "404"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"name": name, "content": content})
	case rest == "issues" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		f.mu.Lock()
		f.nextIID++
		iid := f.nextIID
		f.mu.Unlock()
		var labels []glLabel
		if lv := r.PostForm.Get("labels"); lv != "" {
			for _, l := range strings.Split(lv, ",") {
				labels = append(labels, glLabel{Title: strings.TrimSpace(l)})
			}
		}
		created := f.addIssue(glIssue{
			IID: iid, Title: r.PostForm.Get("title"), Description: r.PostForm.Get("description"),
			DueDate: r.PostForm.Get("due_date"), Labels: labels,
		})
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": created.ID, "iid": created.IID, "web_url": f.issueURL(created.IID), "state": "opened",
		})
	case rest == "milestones" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		f.mu.Lock()
		f.nextMsID++
		id := f.nextMsID
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": id, "iid": id - 9000, "state": "active",
			"web_url": f.srv.URL + "/" + f.projectPath + "/-/milestones/" + strconv.FormatInt(id-9000, 10),
		})
	default:
		// Issue mutations (labels/state/notes/…) from incidental write-backs.
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

// ── shared helpers ───────────────────────────────────────────────────────────

// makeAdmin promotes the client's user to instance admin (integration CRUD is
// admin-gated; with parallel signups only the very first user is admin).
func makeAdmin(t *testing.T, c *client) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = TRUE WHERE id = $1::uuid`, c.UserID); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
}

// connectGitlab links the client's user to the fake instance via PAT.
func connectGitlab(t *testing.T, c *client, f *fakeGitlab) {
	t.Helper()
	m := c.expect(t, c.post("/gitlab/connection", map[string]any{
		"base_url": f.url(), "token": f.token,
	}), http.StatusOK)
	if m["connected"] != true || m["gl_username"] != f.username {
		t.Fatalf("connect: %v", m)
	}
}

// createIntegration binds the fake project to the board (scope=all). extra is
// merged over the base body (e.g. a writeback config).
func createIntegration(t *testing.T, c *client, wsID, boardID string, f *fakeGitlab, extra map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{
		"name": "GL", "project_path": f.projectPath, "board_id": boardID,
		"enabled": true, "scope": "all",
	}
	for k, v := range extra {
		body[k] = v
	}
	return c.expect(t, c.post("/workspaces/"+wsID+"/gitlab/integrations", body), http.StatusOK)
}

// waitSyncRuns polls the workspace sync journal until at least want finished
// runs exist, returning them (newest first as served).
func waitSyncRuns(t *testing.T, c *client, wsID string, want int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		runs := c.get("/workspaces/" + wsID + "/gitlab/sync-runs").listBody(t)
		finished := 0
		for _, r := range runs {
			if r["finished_at"] != nil {
				finished++
			}
		}
		if finished >= want {
			return runs
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("sync run(s) did not finish: want %d", want)
	return nil
}

// triggerSync starts a manual sync and asserts the 202 accepted contract: the
// request returns immediately (the pull runs detached) naming the journal run it
// opened, which the client watches instead of blocking.
func triggerSync(t *testing.T, c *client, wsID, integID string) string {
	t.Helper()
	r := c.post("/workspaces/"+wsID+"/gitlab/integrations/"+integID+"/sync", nil)
	if r.Status != http.StatusAccepted {
		t.Fatalf("sync: status %d\n%s", r.Status, r.Body)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		t.Fatalf("sync: bad body %q: %v", r.Body, err)
	}
	if body["started"] != true {
		t.Fatalf("sync: not started: %v", body)
	}
	runID, _ := body["run_id"].(string)
	if runID == "" {
		t.Fatalf("sync: no run_id in %v", body)
	}
	return runID
}

// taskByIid finds the board task mirroring a GitLab issue iid.
func taskByIid(t *testing.T, tasks []map[string]any, iid float64) map[string]any {
	t.Helper()
	for _, task := range tasks {
		if task["gitlab_iid"] == iid {
			return task
		}
	}
	t.Fatalf("no task with gitlab_iid=%v among %d tasks", iid, len(tasks))
	return nil
}

// ── tests ────────────────────────────────────────────────────────────────────

// Connection lifecycle: bad token rejected, connect, read back, disconnect.
func TestGitlabConnectionLifecycle(t *testing.T) {
	t.Parallel()
	c := signup(t)
	f := newFakeGitlab(t, "gl-conn-user", "grp-conn")

	m := c.expect(t, c.get("/gitlab/connection"), http.StatusOK)
	if m["connected"] != false {
		t.Fatalf("fresh user connected: %v", m)
	}

	// A wrong token fails CurrentUser → 502 from the handler.
	r := c.post("/gitlab/connection", map[string]any{"base_url": f.url(), "token": "wrong"})
	if r.Status != http.StatusBadGateway {
		t.Fatalf("bad token: status %d\n%s", r.Status, r.Body)
	}

	connectGitlab(t, c, f)
	m = c.expect(t, c.get("/gitlab/connection"), http.StatusOK)
	if m["connected"] != true || m["base_url"] != f.url() {
		t.Fatalf("connection readback: %v", m)
	}

	if r := c.del("/gitlab/connection"); r.Status != http.StatusNoContent {
		t.Fatalf("disconnect: status %d", r.Status)
	}
	m = c.expect(t, c.get("/gitlab/connection"), http.StatusOK)
	if m["connected"] != false {
		t.Fatalf("still connected after delete: %v", m)
	}
}

// Integration CRUD: create with defaults, list, dup guard, update, delete.
func TestGitlabIntegrationCRUD(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-crud-user", "grp-crud")

	created := createIntegration(t, c, s.WS, s.Board, f, nil)
	if created["configured"] != true || created["project_path"] != f.projectPath ||
		created["board_id"] != s.Board || created["scope"] != "all" {
		t.Fatalf("create integration: %v", created)
	}
	// Unset enum-ish fields default.
	if created["due_source"] != "issue_milestone" || created["start_source"] != "created" {
		t.Fatalf("defaults: due_source=%v start_source=%v", created["due_source"], created["start_source"])
	}
	integID := created["id"].(string)

	list := c.expect(t, c.get("/workspaces/"+s.WS+"/gitlab/integrations"), http.StatusOK)
	integs := list["integrations"].([]any)
	if len(integs) != 1 || list["default_rules"] == nil || list["is_admin"] != true {
		t.Fatalf("list integrations: %v", list)
	}

	// The same board can't be bound twice.
	r := c.post("/workspaces/"+s.WS+"/gitlab/integrations", map[string]any{
		"name": "dup", "project_path": f.projectPath, "board_id": s.Board, "enabled": true,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("duplicate binding: status %d\n%s", r.Status, r.Body)
	}

	upd := c.expect(t, c.put("/workspaces/"+s.WS+"/gitlab/integrations/"+integID, map[string]any{
		"name": "Renamed", "project_path": f.projectPath, "board_id": s.Board,
		"enabled": true, "scope": "assigned",
	}), http.StatusOK)
	if upd["name"] != "Renamed" || upd["scope"] != "assigned" {
		t.Fatalf("update integration: %v", upd)
	}

	if r := c.del("/workspaces/" + s.WS + "/gitlab/integrations/" + integID); r.Status != http.StatusNoContent {
		t.Fatalf("delete integration: status %d", r.Status)
	}
	list = c.expect(t, c.get("/workspaces/"+s.WS+"/gitlab/integrations"), http.StatusOK)
	if len(list["integrations"].([]any)) != 0 {
		t.Fatalf("integrations after delete: %v", list["integrations"])
	}
}

// Sync happy-path: pull creates tasks with column/priority/tags/assignees/
// comments/milestone, records a journal run + actions, syncs members, and a
// second sync after a remote change updates the mirrored task.
func TestGitlabSyncFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-sync-user", "grp-sync")

	// Fixtures: labelled issue with milestone/assignee/note, an unlabelled one,
	// and a closed one (no milestone → not archived by the default closed policy).
	f.addIssue(glIssue{
		IID: 1, Title: "Fix login bug", Description: "Steps to reproduce",
		DueDate: "2026-08-15",
		Labels: []glLabel{
			{Title: "S: In progress", Color: "#2f80ed"},
			{Title: "P: High", Color: "#d0021b"},
			{Title: "T: bug", Color: "#7c5cff"},
		},
		AssigneeLogins: []string{f.username},
		Milestone:      &glMilestone{ID: 41, IID: 4, Title: "Sprint 1", State: "active", StartDate: "2026-08-01", DueDate: "2026-08-31"},
		Notes: []glNote{
			{ID: 71, Body: "Looks good", AuthorLogin: "reviewer", CreatedAt: time.Now().Add(-time.Hour)},
			{ID: 72, Body: "changed status", System: true, AuthorLogin: "reviewer", CreatedAt: time.Now()},
		},
	})
	f.addIssue(glIssue{IID: 2, Title: "Untriaged idea"})
	f.addIssue(glIssue{IID: 3, Title: "Old fixed thing", State: "closed"})
	f.members = []glMember{
		{ID: 99, Username: f.username, Name: "GL Bot"},
		{ID: 12, Username: "colleague", Name: "Colleague"},
	}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)

	runID := triggerSync(t, c, s.WS, integID)
	runs := waitSyncRuns(t, c, s.WS, 1)
	run := runs[0]
	if run["kind"] != "pull" || run["trigger"] != "manual" || run["status"] != "ok" {
		t.Fatalf("run: %v", run)
	}
	// The row named in the 202 is the one the sync filled in — the client watches
	// that id rather than guessing which run is "its" one.
	if run["id"] != runID {
		t.Fatalf("journal run id = %v, want the id returned by the sync call %q", run["id"], runID)
	}
	if run["created_count"] != float64(3) {
		t.Fatalf("created_count = %v, want 3", run["created_count"])
	}

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if len(tasks) != 3 {
		t.Fatalf("board has %d tasks after sync, want 3\n%v", len(tasks), tasks)
	}

	// Issue 1: S:→column "В процессе", P: High→3, tag kept with prefix, assignee
	// resolved to the connected Tessera user, due date applied, comment imported.
	bug := taskByIid(t, tasks, 1)
	if bug["title"] != "Fix login bug" || bug["column_id"] != s.col(t, 1) {
		t.Fatalf("bug task column: %v (want col %s)", bug, s.col(t, 1))
	}
	if bug["priority"] != float64(3) {
		t.Fatalf("bug priority = %v, want 3", bug["priority"])
	}
	if bug["due_date"] == nil || bug["start_date"] == nil {
		t.Fatalf("bug dates: due=%v start=%v", bug["due_date"], bug["start_date"])
	}
	if tags, _ := bug["tag_ids"].([]any); len(tags) != 1 {
		t.Fatalf("bug tag_ids = %v, want 1", bug["tag_ids"])
	}
	assignees, _ := bug["assignee_ids"].([]any)
	if len(assignees) != 1 || assignees[0] != c.UserID {
		t.Fatalf("bug assignees = %v, want [%s]", assignees, c.UserID)
	}
	if bug["milestone_id"] == nil {
		t.Fatalf("bug milestone not linked: %v", bug["milestone_id"])
	}
	comments := c.get("/tasks/" + bug["id"].(string) + "/comments").listBody(t)
	if len(comments) != 1 || comments[0]["body"] != "Looks good" {
		t.Fatalf("bug comments: %v", comments)
	}

	// Issue 2: no status label → the default column ("К работе").
	idea := taskByIid(t, tasks, 2)
	if idea["column_id"] != s.col(t, 0) {
		t.Fatalf("idea column = %v, want default %s", idea["column_id"], s.col(t, 0))
	}

	// Issue 3: closed → done column + completed.
	old := taskByIid(t, tasks, 3)
	if old["column_id"] != s.col(t, 3) || old["completed_at"] == nil {
		t.Fatalf("closed issue task: col=%v completed=%v", old["column_id"], old["completed_at"])
	}

	// The label became a project-scoped tag with its prefix kept (default rules).
	tagNames := map[string]bool{}
	for _, tg := range c.get("/projects/"+s.Project+"/tags").listBody(t) {
		tagNames[tg["name"].(string)] = true
	}
	if !tagNames["T: bug"] {
		t.Fatalf("project tags missing 'T: bug': %v", tagNames)
	}

	// The GitLab milestone was mirrored as a native project milestone.
	msTitles := map[string]bool{}
	for _, m := range c.get("/projects/"+s.Project+"/milestones").listBody(t) {
		msTitles[m["title"].(string)] = true
	}
	if !msTitles["Sprint 1"] {
		t.Fatalf("milestones missing 'Sprint 1': %v", msTitles)
	}

	// The member roster synced; the PAT owner maps to the Tessera user.
	members := c.get("/workspaces/" + s.WS + "/gitlab/members").listBody(t)
	if len(members) != 2 {
		t.Fatalf("gitlab members = %v, want 2", members)
	}
	for _, m := range members {
		if m["gl_username"] == f.username && m["tessera_user_id"] != c.UserID {
			t.Fatalf("member %s not mapped to tessera user: %v", f.username, m)
		}
	}

	// Journal actions: three pull-creates.
	actions := c.get("/workspaces/" + s.WS + "/gitlab/sync-runs/" + run["id"].(string) + "/actions").listBody(t)
	if len(actions) != 3 {
		t.Fatalf("run actions = %d, want 3\n%v", len(actions), actions)
	}
	for _, a := range actions {
		if a["direction"] != "pull" || a["op"] != "create" || a["status"] != "ok" {
			t.Fatalf("action: %v", a)
		}
	}

	// Remote change: retitle + close issue 1, then re-sync → task updated.
	is := f.findIssue(1)
	f.mu.Lock()
	is.Title = "Fix login bug v2"
	is.State = "closed"
	is.UpdatedAt = time.Now().UTC()
	f.mu.Unlock()

	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 2)

	tasks = c.get("/boards/" + s.Board + "/tasks").listBody(t)
	bug = taskByIid(t, tasks, 1)
	if bug["title"] != "Fix login bug v2" {
		t.Fatalf("second sync title = %v", bug["title"])
	}
	if bug["column_id"] != s.col(t, 3) || bug["completed_at"] == nil {
		t.Fatalf("second sync close: col=%v completed=%v", bug["column_id"], bug["completed_at"])
	}

	// Third sync with nothing changed → an incremental no-op: the overlap window
	// re-delivers the issues, but dropUnchangedIssues skips every one (updatedAt +
	// title/labels hash unchanged), so no create/update lands. This is the headline
	// perf behaviour — a pull with an empty delta does no work.
	triggerSync(t, c, s.WS, integID)
	runs3 := waitSyncRuns(t, c, s.WS, 3)
	newest := runs3[0]
	if newest["mode"] != "incremental" {
		t.Fatalf("third sync mode = %v, want incremental", newest["mode"])
	}
	if newest["created_count"] != float64(0) || newest["updated_count"] != float64(0) {
		t.Fatalf("third sync should be a no-op: created=%v updated=%v",
			newest["created_count"], newest["updated_count"])
	}

	// A brand-new issue created after the last sync IS pulled by an incremental sync
	// (its updatedAt lands in the delta) — incremental adds new tasks, not only
	// updates existing ones.
	f.addIssue(glIssue{IID: 42, Title: "Fresh incremental issue"})
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 4)
	tasks = c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if fresh := taskByIid(t, tasks, 42); fresh["title"] != "Fresh incremental issue" {
		t.Fatalf("incremental sync did not create the new issue: %v", fresh)
	}

	// Delete issue 1 in GitLab, then a FULL sync (?mode=full) detects the orphaned
	// link and archives the task — an incremental delta can't tell "deleted" from
	// "unchanged, so not re-sent", so this is full-sweep only.
	f.mu.Lock()
	kept := f.issues[:0]
	for _, is := range f.issues {
		if is.IID != 1 {
			kept = append(kept, is)
		}
	}
	f.issues = kept
	f.mu.Unlock()

	if r := c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/sync?mode=full", nil); r.Status != http.StatusAccepted {
		t.Fatalf("full sync: status %d\n%s", r.Status, r.Body)
	}
	waitSyncRuns(t, c, s.WS, 5)

	tasks = c.get("/boards/" + s.Board + "/tasks").listBody(t)
	for _, tk := range tasks {
		if tk["gitlab_iid"] == float64(1) {
			t.Fatalf("issue 1 deleted in GitLab but its task is still active on the board")
		}
	}
}

// Issue templates: empty without a binding, served from the repo through the fake.
// Linked items → relations, over the GraphQL widget path (the default). This is
// the case that regressed in #2591: the widget reports the WorkItemRelatedLinkType
// enum (RELATED / BLOCKED_BY), which RelationKind used to drop, so relations never
// appeared on real GitLab even though the REST-shaped tests passed. The fake now
// serves the true enum (see graphqlLinkType), so a broken RelationKind fails here.
func TestGitlabLinkedItemsRelations(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-rel-user", "grp-rel")

	// #1 relates to #2 and is blocked by #3 — the two link types the enum bug hid
	// (only "blocks" survived it). Fixtures are written in REST spelling; the GraphQL
	// responder upper-cases them to RELATED / BLOCKED_BY like the real widget.
	f.addIssue(glIssue{IID: 1, Title: "Central"})
	f.addIssue(glIssue{IID: 2, Title: "Related one"})
	f.addIssue(glIssue{IID: 3, Title: "Blocker"})
	f.links = []glLink{
		{SrcIID: 1, DstIID: 2, LinkType: "relates_to", LinkID: 501},
		{SrcIID: 1, DstIID: 3, LinkType: "is_blocked_by", LinkID: 502},
	}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	central := taskByIid(t, tasks, 1)
	related := taskByIid(t, tasks, 2)
	blocker := taskByIid(t, tasks, 3)

	rels := c.get("/tasks/" + central["id"].(string) + "/relations").listBody(t)
	if len(rels) != 2 {
		t.Fatalf("relations = %d, want 2 (relates + blocked_by)\n%v", len(rels), rels)
	}
	byKind := map[string]map[string]any{}
	for _, r := range rels {
		byKind[r["kind"].(string)] = r
	}
	if rr := byKind["relates"]; rr == nil || rr["source"] != "gitlab" || rr["related_task_id"] != related["id"] {
		t.Fatalf("relates relation wrong (enum RELATED dropped?): %v", byKind["relates"])
	}
	if br := byKind["blocked_by"]; br == nil || br["source"] != "gitlab" || br["related_task_id"] != blocker["id"] {
		t.Fatalf("blocked_by relation wrong (enum BLOCKED_BY dropped?): %v", byKind["blocked_by"])
	}

	// The link is bidirectional in GitLab, so it must show on BOTH tasks even though
	// only #1's side declared it here — #2591 rework. #2 sees the reverse "relates",
	// #3 the inverse of "blocked_by", i.e. it "blocks" #1.
	relOf := func(taskID string) map[string]any {
		got := c.get("/tasks/" + taskID + "/relations").listBody(t)
		if len(got) != 1 {
			t.Fatalf("task %s relations = %d, want 1 (reverse of #1's link)\n%v", taskID, len(got), got)
		}
		return got[0]
	}
	if r := relOf(related["id"].(string)); r["kind"] != "relates" || r["source"] != "gitlab" || r["related_task_id"] != central["id"] {
		t.Fatalf("reverse relates on #2 wrong: %v", r)
	}
	if r := relOf(blocker["id"].(string)); r["kind"] != "blocks" || r["source"] != "gitlab" || r["related_task_id"] != central["id"] {
		t.Fatalf("reverse blocks on #3 wrong: %v", r)
	}
}

func TestGitlabIssueTemplates(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-tpl-user", "grp-tpl")
	f.templates["Bug"] = "## Bug report\nSteps:"

	connectGitlab(t, c, f)

	// No binding yet → empty list (soft contract for the create-issue modal).
	if tpls := c.get("/workspaces/" + s.WS + "/gitlab/issue-templates").listBody(t); len(tpls) != 0 {
		t.Fatalf("templates without binding: %v", tpls)
	}

	createIntegration(t, c, s.WS, s.Board, f, nil)
	tpls := c.get("/workspaces/" + s.WS + "/gitlab/issue-templates").listBody(t)
	if len(tpls) != 1 || tpls[0]["name"] != "Bug" || tpls[0]["content"] != "## Bug report\nSteps:" {
		t.Fatalf("templates: %v", tpls)
	}
}

// Create-issue-from-task: gated on writeback.push_create, links the task, and
// refuses a second link.
func TestGitlabCreateIssueFromTask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-create-user", "grp-create")

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)

	task := mkTask(t, c, s.Board, s.col(t, 0), "Ship the widget")
	taskID := task["id"].(string)

	// push_create disabled (the default) → refused.
	r := c.post("/tasks/"+taskID+"/gitlab-issue", map[string]any{})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("create issue without push_create: status %d\n%s", r.Status, r.Body)
	}

	// Enable push_create and retry.
	c.expect(t, c.put("/workspaces/"+s.WS+"/gitlab/integrations/"+integID, map[string]any{
		"name": "GL", "project_path": f.projectPath, "board_id": s.Board,
		"enabled": true, "scope": "all",
		"writeback": map[string]any{"enabled": true, "push_create": true},
	}), http.StatusOK)

	link := c.expect(t, c.post("/tasks/"+taskID+"/gitlab-issue",
		map[string]any{"description": "From template"}), http.StatusOK)
	iid, _ := link["iid"].(float64)
	if iid == 0 || link["web_url"] == "" {
		t.Fatalf("created link: %v", link)
	}
	created := f.findIssue(int64(iid))
	if created == nil || created.Title != "Ship the widget" || created.Description != "From template" {
		t.Fatalf("fake issue store: %+v", created)
	}

	// The board task now carries the GitLab provenance.
	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if got := taskByIid(t, tasks, iid); got["id"] != taskID {
		t.Fatalf("linked task mismatch: %v", got)
	}

	// A second create on the same task conflicts.
	r = c.post("/tasks/"+taskID+"/gitlab-issue", map[string]any{})
	if r.Status != http.StatusConflict {
		t.Fatalf("second create: status %d\n%s", r.Status, r.Body)
	}
}

// Push a native milestone to GitLab: creates the remote milestone, links it,
// and refuses a re-push.
func TestGitlabPushMilestone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-ms-user", "grp-ms")

	connectGitlab(t, c, f)
	createIntegration(t, c, s.WS, s.Board, f, nil)

	ms := c.expect(t, c.post("/projects/"+s.Project+"/milestones", map[string]any{
		"title": "Native sprint", "description": "Q3 push",
	}), http.StatusCreated)
	msID := ms["id"].(string)

	pushed := c.expect(t, c.post("/milestones/"+msID+"/gitlab", nil), http.StatusOK)
	if pushed["gl_linked"] != true || pushed["gl_url"] == "" {
		t.Fatalf("push milestone: %v", pushed)
	}

	r := c.post("/milestones/"+msID+"/gitlab", nil)
	if r.Status != http.StatusConflict {
		t.Fatalf("re-push milestone: status %d\n%s", r.Status, r.Body)
	}
}
