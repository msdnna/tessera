// Merge requests and pipelines in a task's «Разработка» (#2964), end to end
// against a real database and the fake GitLab: the poll that rides along with a
// pull, the merge_request and pipeline webhooks, the socket event an open task
// window repaints from, and the history line.
package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// glMR is one merge request of the fake project.
type glMR struct {
	ID           int64
	IID          int64
	Title        string
	Description  string
	State        string // opened | merged | closed
	Draft        bool
	SourceBranch string
	TargetBranch string
	UpdatedAt    time.Time
	// Head pipeline, if any.
	PipelineID     int64
	PipelineStatus string // GraphQL enum spelling: SUCCESS, RUNNING…
}

func (f *fakeGitlab) addMR(mr glMR) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if mr.ID == 0 {
		mr.ID = 5000 + mr.IID
	}
	if mr.State == "" {
		mr.State = "opened"
	}
	if mr.UpdatedAt.IsZero() {
		mr.UpdatedAt = time.Now().UTC()
	}
	f.mrs = append(f.mrs, mr)
}

// mergeRequestsQuery serves Project.mergeRequests with the iids / updatedAfter
// filters, in one page (pagination is covered by the client's unit test).
func (f *fakeGitlab) mergeRequestsQuery(w http.ResponseWriter, vars map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := map[int64]bool{}
	if raw, ok := vars["iids"].([]any); ok {
		for _, v := range raw {
			n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64)
			want[n] = true
		}
	}
	var after time.Time
	if s, ok := vars["updatedAfter"].(string); ok {
		after, _ = time.Parse(time.RFC3339, s)
	}
	nodes := []any{}
	for _, mr := range f.mrs {
		if len(want) > 0 && !want[mr.IID] {
			continue
		}
		if !after.IsZero() && !mr.UpdatedAt.After(after) {
			continue
		}
		node := map[string]any{
			"id": fmt.Sprintf("gid://gitlab/MergeRequest/%d", mr.ID), "iid": strconv.FormatInt(mr.IID, 10),
			"title": mr.Title, "description": mr.Description,
			"webUrl": f.srv.URL + "/" + f.projectPath + "/-/merge_requests/" + strconv.FormatInt(mr.IID, 10),
			"state":  mr.State, "draft": mr.Draft,
			"sourceBranch": mr.SourceBranch, "targetBranch": mr.TargetBranch,
			"updatedAt": mr.UpdatedAt.Format(time.RFC3339), "headPipeline": nil,
		}
		if mr.PipelineID > 0 {
			node["headPipeline"] = map[string]any{
				"id":     fmt.Sprintf("gid://gitlab/Ci::Pipeline/%d", mr.PipelineID),
				"status": mr.PipelineStatus,
				"path":   "/" + f.projectPath + "/-/pipelines/" + strconv.FormatInt(mr.PipelineID, 10),
			}
		}
		nodes = append(nodes, node)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"project": map[string]any{"mergeRequests": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			"nodes":    nodes,
		}},
	}})
}

// closesIssues serves GET merge_requests/:iid/closes_issues.
func (f *fakeGitlab) closesIssues(w http.ResponseWriter, rest string) {
	iid, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(rest, "merge_requests/"), "/closes_issues"), 10, 64)
	f.mu.Lock()
	ids := append([]int64(nil), f.closes[iid]...)
	f.mu.Unlock()
	out := []any{}
	for _, n := range ids {
		out = append(out, map[string]any{"iid": n, "web_url": f.issueURL(n)})
	}
	writeJSON(w, http.StatusOK, out)
}

// codeLinks reads a task's «Разработка» list off the task detail.
func codeLinks(t *testing.T, c *client, taskID string) []map[string]any {
	t.Helper()
	detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	raw, ok := detail["code_links"].([]any)
	if !ok {
		t.Fatalf("task detail has no code_links list: %v", detail["code_links"])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func linkByNumber(links []map[string]any, n float64) map[string]any {
	for _, l := range links {
		if l["number"] == n {
			return l
		}
	}
	return nil
}

func TestGitlabMergeRequestsFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-mr-user", "grp-mr")
	f.closes = map[int64][]int64{}

	f.addIssue(glIssue{IID: 1, Title: "Login"})
	f.addIssue(glIssue{IID: 2, Title: "Board v2"})
	f.addIssue(glIssue{IID: 3, Title: "Tags"})
	f.addIssue(glIssue{IID: 4, Title: "Untouched"})

	// !418 closes #2 in its description and has a green pipeline.
	f.addMR(glMR{IID: 418, Title: "Редизайн доски v2", Description: "Closes #2",
		SourceBranch: "feat/board-v2", TargetBranch: "main", PipelineID: 90, PipelineStatus: "SUCCESS"})
	// !419 names nothing — only its branch was cut from issue 1. Merged.
	f.addMR(glMR{IID: 419, Title: "Fix", SourceBranch: "1-login-fix", TargetBranch: "main", State: "merged"})
	// !420 is linked to #1 only by GitLab's own closes list (an open MR).
	f.addMR(glMR{IID: 420, Title: "Draft: auth", Draft: true, SourceBranch: "auth", TargetBranch: "main"})
	f.closes[420] = []int64{1}
	// !421 is about another project's issue — never linked here.
	f.addMR(glMR{IID: 421, Title: "Bump", Description: "See other/repo#4", SourceBranch: "bump", TargetBranch: "main"})

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	t1 := taskByIid(t, tasks, 1)["id"].(string)
	t2 := taskByIid(t, tasks, 2)["id"].(string)
	t3 := taskByIid(t, tasks, 3)["id"].(string)
	t4 := taskByIid(t, tasks, 4)["id"].(string)

	// The poll runs at the end of the detached pull: wait for its rows.
	var l2 []map[string]any
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if l2 = codeLinks(t, c, t2); len(l2) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(l2) != 1 {
		t.Fatalf("task #2 links = %v, want !418", l2)
	}
	mr := l2[0]
	if mr["number"] != float64(418) || mr["state"] != "open" || mr["draft"] != false ||
		mr["source_branch"] != "feat/board-v2" || mr["target_branch"] != "main" ||
		mr["ci_status"] != "success" || mr["via"] != "closes" || mr["provider"] != "gitlab" ||
		mr["title"] != "Редизайн доски v2" || !strings.HasSuffix(mr["url"].(string), "/grp-mr/-/merge_requests/418") ||
		!strings.HasSuffix(mr["ci_url"].(string), "/grp-mr/-/pipelines/90") {
		t.Fatalf("!418 on task #2 = %v", mr)
	}

	l1 := codeLinks(t, c, t1)
	if len(l1) != 2 {
		t.Fatalf("task #1 links = %v, want !419 and !420", l1)
	}
	// Open first: the draft !420 heads the list, the merged !419 follows.
	if l1[0]["number"] != float64(420) || l1[0]["draft"] != true || l1[0]["via"] != "closes" || l1[0]["ci_status"] != "" {
		t.Fatalf("!420 on task #1 = %v", l1[0])
	}
	if l1[1]["number"] != float64(419) || l1[1]["state"] != "merged" || l1[1]["via"] != "branch" {
		t.Fatalf("!419 on task #1 = %v", l1[1])
	}
	if l := codeLinks(t, c, t4); len(l) != 0 {
		t.Fatalf("task #4 has links it never earned: %v", l)
	}

	// ── webhooks ──
	enabled := c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/webhook", nil), http.StatusOK)
	secret := enabled["secret"].(string)

	conn, status := dialWS(t, c.token, false)
	if conn == nil {
		t.Fatalf("handshake failed (status %d)", status)
	}
	defer conn.Close()

	pipeline := func(id int64, status, mrPart, ref string) {
		t.Helper()
		body := fmt.Sprintf(`{"object_kind":"pipeline","project":{"path_with_namespace":"grp-mr","web_url":"%s/grp-mr"},
			"object_attributes":{"id":%d,"ref":%q,"status":%q},"merge_request":%s}`, f.url(), id, ref, status, mrPart)
		if r := postHook(t, integID, secret, body); r.Status != http.StatusAccepted {
			t.Fatalf("pipeline hook: %d\n%s", r.Status, r.Body)
		}
	}

	// A new pipeline of !418 starts → «идёт», and the open window hears about it.
	pipeline(91, "running", `{"iid":418}`, "feat/board-v2")
	ev := awaitWSEvent(t, conn, "task.code_links", 5*time.Second)
	if ev == nil {
		t.Fatal("no task.code_links event after a pipeline hook")
	}
	data := ev["data"].(map[string]any)
	if data["task_id"] != t2 || ev["actor"] != nil && ev["actor"] != "" {
		t.Fatalf("event = %v", ev)
	}
	if got := data["code_links"].([]any)[0].(map[string]any); got["ci_status"] != "running" {
		t.Fatalf("event links = %v", data["code_links"])
	}

	// An older run arriving late must not overwrite the newer one.
	pipeline(89, "failed", `{"iid":418}`, "feat/board-v2")
	if got := codeLinks(t, c, t2)[0]; got["ci_status"] != "running" || !strings.HasSuffix(got["ci_url"].(string), "/pipelines/91") {
		t.Fatalf("a late older pipeline overwrote the newer one: %v", got)
	}
	// A branch pipeline (no MR in the payload) lands on the open MR of its branch.
	pipeline(92, "failed", `null`, "feat/board-v2")
	if got := codeLinks(t, c, t2)[0]; got["ci_status"] != "failed" {
		t.Fatalf("branch pipeline not applied: %v", got)
	}

	mrHook := func(body string) {
		t.Helper()
		if r := postHook(t, integID, secret, body); r.Status != http.StatusAccepted {
			t.Fatalf("merge_request hook: %d\n%s", r.Status, r.Body)
		}
	}
	// !418 merged — the state moves, the pipeline stays, the history says so.
	mrHook(`{"object_kind":"merge_request","project":{"path_with_namespace":"grp-mr"},"object_attributes":{
		"id":5418,"iid":418,"title":"Редизайн доски v2","description":"Closes #2","url":"` + f.url() + `/grp-mr/-/merge_requests/418",
		"state":"merged","draft":false,"source_branch":"feat/board-v2","target_branch":"main",
		"updated_at":"` + time.Now().UTC().Add(time.Minute).Format(time.RFC3339) + `"}}`)
	got := codeLinks(t, c, t2)[0]
	if got["state"] != "merged" || got["ci_status"] != "failed" {
		t.Fatalf("after the merge hook: %v", got)
	}
	// An MR the poll never saw arrives by hook and links issue 3 by its text.
	mrHook(`{"object_kind":"merge_request","project":{"path_with_namespace":"grp-mr"},"object_attributes":{
		"id":5422,"iid":422,"title":"Tags polish","description":"Fixes #3","url":"` + f.url() + `/grp-mr/-/merge_requests/422",
		"state":"opened","work_in_progress":true,"source_branch":"tags","target_branch":"main","updated_at":"2026-10-02 10:11:12 UTC"}}`)
	l3 := codeLinks(t, c, t3)
	if len(l3) != 1 || l3[0]["number"] != float64(422) || l3[0]["draft"] != true || l3[0]["via"] != "closes" {
		t.Fatalf("task #3 after the hook = %v", l3)
	}
	// A hook carries no closes list, so it must not drop the link GitLab's list made.
	mrHook(`{"object_kind":"merge_request","project":{"path_with_namespace":"grp-mr"},"object_attributes":{
		"id":5420,"iid":420,"title":"Draft: auth","description":"","state":"opened","draft":true,
		"source_branch":"auth","target_branch":"main","updated_at":"` + time.Now().UTC().Add(time.Minute).Format(time.RFC3339) + `"}}`)
	if l := codeLinks(t, c, t1); linkByNumber(l, 420) == nil {
		t.Fatalf("the hook dropped !420 from task #1: %v", l)
	}

	// History: «GitLab · открыт !418 …» when it appeared, then «смержен».
	events := c.get("/tasks/" + t2 + "/events").listBody(t)
	var states []string
	for _, e := range events {
		if e["kind"] != "code_change" {
			continue
		}
		d := e["data"].(map[string]any)
		if d["number"] != float64(418) || d["source"] != "gitlab" {
			t.Fatalf("code_change event = %v", e)
		}
		states = append(states, d["state"].(string))
	}
	if strings.Join(states, ",") != "open,merged" && strings.Join(states, ",") != "merged,open" {
		t.Fatalf("code_change events for !418 = %v", states)
	}

	// Deleting the binding takes its merge requests with it.
	c.expect(t, c.del("/workspaces/"+s.WS+"/gitlab/integrations/"+integID), http.StatusNoContent)
	if l := codeLinks(t, c, t1); len(l) != 0 {
		t.Fatalf("links survived the binding: %v", l)
	}
}

// The poll re-reads an MR whose pipeline it last saw running, even though a
// finished pipeline does not make the MR show up as updated.
func TestGitlabMergeRequestRunningPipelineRefresh(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-mr-poll", "grp-mr-poll")
	f.addIssue(glIssue{IID: 7, Title: "Search"})
	f.addMR(glMR{IID: 30, Title: "Search", SourceBranch: "7-search", TargetBranch: "main",
		PipelineID: 300, PipelineStatus: "RUNNING", UpdatedAt: time.Now().Add(-time.Hour).UTC()})

	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, nil)["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	taskID := taskByIid(t, c.get("/boards/"+s.Board+"/tasks").listBody(t), 7)["id"].(string)
	waitCI := func(want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		var l []map[string]any
		for time.Now().Before(deadline) {
			if l = codeLinks(t, c, taskID); len(l) == 1 && l[0]["ci_status"] == want {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("ci_status never became %q: %v", want, l)
	}
	waitCI("running")

	// The pipeline finishes; the MR's own updatedAt stays in the past, so only the
	// running-pipeline re-read can see it.
	f.mu.Lock()
	f.mrs[0].PipelineStatus = "SUCCESS"
	f.mu.Unlock()
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 2)
	waitCI("success")
}
