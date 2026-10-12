package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// integrationByID reads one binding back from the workspace list.
func integrationByID(t *testing.T, c *client, wsID, id string) map[string]any {
	t.Helper()
	list := c.expect(t, c.get("/workspaces/"+wsID+"/gitlab/integrations"), http.StatusOK)
	for _, raw := range list["integrations"].([]any) {
		if b := raw.(map[string]any); b["id"] == id {
			return b
		}
	}
	t.Fatalf("binding %s not listed", id)
	return nil
}

// Two GitLab projects mirror into one board (#3082), each card belonging to its
// own binding; the first imports only issues labelled «backend» (#3085, the
// filter rides in the issues query itself). Creating an issue from a task on such
// a board asks which project, and goes where it is told.
func TestGitlabMultiProjectBoardAndImportFilter(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-multi-user", "grp-multi")
	const other = "grp-multi-b"
	f.addIssue(glIssue{IID: 1, Title: "Backend one", Labels: []glLabel{{Title: "backend"}}})
	f.addIssue(glIssue{IID: 2, Title: "Frontend two"})
	f.addIssue(glIssue{IID: 3, Title: "Other project", Project: other})

	connectGitlab(t, c, f)
	wb := map[string]any{"enabled": true, "push_create": true}
	a := createIntegration(t, c, s.WS, s.Board, f, map[string]any{"import_labels": []string{" backend ", "backend"}, "writeback": wb})
	b := c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/integrations", map[string]any{
		"name": "B", "project_path": other, "board_id": s.Board, "enabled": true, "scope": "all", "writeback": wb,
	}), http.StatusOK)
	if got, _ := json.Marshal(a["import_labels"]); string(got) != `["backend"]` {
		t.Fatalf("import_labels = %s, want [\"backend\"]", got)
	}

	triggerSync(t, c, s.WS, a["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)
	triggerSync(t, c, s.WS, b["id"].(string))
	waitSyncRuns(t, c, s.WS, 2)
	tasks := waitBoardTaskCount(t, c, s.Board, 2)
	if taskByIid(t, tasks, 1)["title"] != "Backend one" || taskByIid(t, tasks, 3)["title"] != "Other project" {
		t.Fatalf("board tasks = %v", tasks)
	}
	f.mu.Lock()
	var labelled bool
	for _, q := range f.issueQueries {
		if q.iids == nil && len(q.labels) == 1 && q.labels[0] == "backend" {
			labelled = true
		}
	}
	f.mu.Unlock()
	if !labelled {
		t.Fatal("the list query of binding A did not carry its label filter")
	}

	task := mkTask(t, c, s.Board, s.col(t, 0), "Pick a project")
	taskID := task["id"].(string)
	r := c.post("/tasks/"+taskID+"/gitlab-issue", map[string]any{})
	if r.Status != http.StatusConflict {
		t.Fatalf("create without a project: status %d\n%s", r.Status, r.Body)
	}
	var choose struct {
		Code         string           `json:"code"`
		Integrations []map[string]any `json:"integrations"`
	}
	_ = json.Unmarshal([]byte(r.Body), &choose)
	if choose.Code != "choose_integration" || len(choose.Integrations) != 2 {
		t.Fatalf("choose answer = %+v", choose)
	}
	link := c.expect(t, c.post("/tasks/"+taskID+"/gitlab-issue", map[string]any{"integration_id": a["id"]}), http.StatusOK)
	if link["project_path"] != f.projectPath {
		t.Fatalf("issue opened in %v, want %s", link["project_path"], f.projectPath)
	}
}

// A refused credential pauses the binding (#3084) and a reconnect lifts the pause;
// the connection and the binding report when the PAT runs out.
func TestGitlabPausedSyncAndReconnect(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-pause-user", "grp-pause")
	f.tokenExpires = "2027-01-12"
	f.addIssue(glIssue{IID: 1, Title: "One"})

	connectGitlab(t, c, f)
	conn := c.expect(t, c.get("/gitlab/connection"), http.StatusOK)
	if conn["auth_kind"] != "pat" || conn["token_expires_at"] != "2027-01-12" {
		t.Fatalf("connection = %v, want a PAT until 2027-01-12", conn)
	}
	integID := createIntegration(t, c, s.WS, s.Board, f, nil)["id"].(string)

	// The token is revoked in GitLab: the next sync is refused and the binding
	// pauses.
	f.mu.Lock()
	f.token = "rotated-token"
	f.mu.Unlock()
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	deadline := time.Now().Add(5 * time.Second)
	var b map[string]any
	for time.Now().Before(deadline) {
		if b = integrationByID(t, c, s.WS, integID); b["paused_at"] != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if b["paused_at"] == nil || b["pause_reason"] != "unauthorized" {
		t.Fatalf("binding after a refused sync = %v, want paused", b)
	}

	// «Переподключить»: the new token goes in, the pause is lifted.
	c.expect(t, c.post("/gitlab/connection", map[string]any{"base_url": f.url(), "token": "rotated-token"}), http.StatusOK)
	if b = integrationByID(t, c, s.WS, integID); b["paused_at"] != nil {
		t.Fatalf("binding after the reconnect = %v, want resumed", b)
	}
}
