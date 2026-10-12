package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// labelStageRules is the default taxonomy plus a «лейбл → этап» rule (#3086).
func labelStageRules(stageDisabled bool) map[string]any {
	return map[string]any{
		"default_column": "К работе", "default_action": "tag", "tag_keep_prefix": true,
		"rules": []map[string]any{
			{"match": "S: ", "match_type": "prefix", "action": "status", "value_map": map[string]string{"Done": "Готово"}},
			{"match": "sprint::", "match_type": "prefix", "action": "milestone", "template": "Спринт {value}", "disabled": stageDisabled},
			{"match": "M: ", "match_type": "prefix", "action": "group"},
		},
	}
}

func taskStageTitle(t *testing.T, taskID string) string {
	t.Helper()
	var title *string
	err := testPool.QueryRow(context.Background(),
		`SELECT m.title FROM tasks t LEFT JOIN milestones m ON m.id = t.milestone_id WHERE t.id = $1::uuid`, taskID).Scan(&title)
	if err != nil {
		t.Fatalf("stage of %s: %v", taskID, err)
	}
	if title == nil {
		return ""
	}
	return *title
}

// A label rule names the stage (#3086); a native GitLab milestone wins over it.
// The rule counters and «Проверить лейбл» (#3087) run on the edited rules, so a
// switched-off rule counts nothing and lets the label fall through.
func TestGitlabLabelStageRuleAndPreview(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-stage-user", "grp-stage")
	f.addIssue(glIssue{IID: 1, Title: "One", Labels: []glLabel{{Title: "sprint::42"}}})
	f.addIssue(glIssue{IID: 2, Title: "Two", Labels: []glLabel{{Title: "sprint::42"}, {Title: "S: Done"}}})
	f.addIssue(glIssue{IID: 3, Title: "Three", Labels: []glLabel{{Title: "sprint::7"}},
		Milestone: &glMilestone{ID: 77, IID: 7, Title: "Native", State: "active"}})

	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, map[string]any{"label_rules": labelStageRules(false)})["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	tasks := waitBoardTaskCount(t, c, s.Board, 3)

	one, two, three := taskByIid(t, tasks, 1), taskByIid(t, tasks, 2), taskByIid(t, tasks, 3)
	if got := taskStageTitle(t, one["id"].(string)); got != "Спринт 42" {
		t.Fatalf("issue 1 stage = %q, want «Спринт 42»", got)
	}
	if one["milestone_id"] != two["milestone_id"] {
		t.Fatalf("one label, one stage: %v vs %v", one["milestone_id"], two["milestone_id"])
	}
	if got := taskStageTitle(t, three["id"].(string)); got != "Native" {
		t.Fatalf("issue 3 stage = %q, want the native milestone", got)
	}

	preview := func(rules map[string]any, label string) map[string]any {
		return c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/rules/preview", map[string]any{
			"integration_id": integID, "label_rules": rules, "label": label,
		}), http.StatusOK)
	}
	p := preview(labelStageRules(false), "sprint::42")
	hits := p["hits"].([]any)
	if hits[0] != float64(1) || hits[1] != float64(3) || hits[2] != float64(0) {
		t.Fatalf("hits = %v, want [1 3 0]", hits)
	}
	if p["linked"] != float64(3) || p["recorded"] != float64(3) {
		t.Fatalf("linked/recorded = %v/%v, want 3/3", p["linked"], p["recorded"])
	}
	check := p["check"].(map[string]any)
	out := check["outcome"].(map[string]any)
	if out["rule"] != float64(1) || out["action"] != "milestone" || out["result"] != "Спринт 42" || check["tasks"] != float64(2) {
		t.Fatalf("check = %v", check)
	}

	p = preview(labelStageRules(true), "sprint::42")
	if hits := p["hits"].([]any); hits[1] != float64(0) {
		t.Fatalf("a disabled rule counted %v", hits[1])
	}
	out = p["check"].(map[string]any)["outcome"].(map[string]any)
	if out["rule"] != float64(-1) || out["action"] != "tag" {
		t.Fatalf("disabled rule outcome = %v, want the default tag", out)
	}
}

// setIssueDue moves the fake issue's own due date — a change made in GitLab.
func setIssueDue(w *wbFake, iid int64, due string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.findIssueLocked(iid).DueDate = due
}

// The conflict policy (#3088) settles a both-sides change without asking: GitLab
// wins → its value lands in the task and nothing is pushed; Tessera wins → the
// push goes out. «Спросить» parks it, and the batch endpoint (#3089) decides it.
func TestGitlabConflictPolicyAndBatchResolve(t *testing.T) {
	t.Parallel()
	st := newWritebackStand(t, "gl-policy-user", "grp-policy")
	c, s := st.c, st.s
	putPolicy := func(policy string) {
		c.expect(t, c.put("/workspaces/"+s.WS+"/gitlab/integrations/"+st.integID, map[string]any{
			"name": "GL", "project_path": st.w.projectPath, "board_id": s.Board, "enabled": true, "scope": "all",
			"writeback": map[string]any{"enabled": true, "conflict_policy": policy, "bindings": []map[string]any{{
				"enabled": true,
				"trigger": map[string]any{"type": "due", "date_kind": "due"},
				"action":  map[string]any{"type": "set_due", "date_kind": "due"},
			}}},
		}), http.StatusOK)
	}
	setTaskDue := func(day string) {
		c.expect(t, c.patch("/tasks/"+st.taskID, map[string]any{"title": "WB target", "due_date": day + "T00:00:00Z"}), http.StatusOK)
	}
	taskDue := func() string {
		d, _ := c.expect(t, c.get("/tasks/"+st.taskID), http.StatusOK)["due_date"].(string)
		at, err := time.Parse(time.RFC3339, d)
		if err != nil {
			return d
		}
		return at.UTC().Format("2006-01-02")
	}
	dueRowIs := func(status string) func() bool {
		return func() bool { return writebackRows(t, st.taskID)["due"].Status == status }
	}

	// GitLab wins.
	putPolicy("gitlab")
	setIssueDue(st.w, 1, "2026-12-01")
	setTaskDue("2026-11-01")
	drainOutboxUntil(t, dueRowIs("sent"))
	if got := taskDue(); got != "2026-12-01" {
		t.Fatalf("GitLab wins: task due = %q, want GitLab's 2026-12-01", got)
	}
	if puts := st.w.callsMatching(http.MethodPut, "/issues/1"); len(puts) != 0 {
		t.Fatalf("GitLab wins must push nothing: %+v", puts)
	}

	// Tessera wins.
	putPolicy("tessera")
	setIssueDue(st.w, 1, "2027-01-05")
	setTaskDue("2026-10-20")
	drainOutboxUntil(t, func() bool {
		for _, call := range st.w.callsMatching(http.MethodPut, "/issues/1") {
			if call.Form.Get("due_date") == "2026-10-20" {
				return true
			}
		}
		return false
	})

	// Ask: parked, then decided through the batch endpoint.
	putPolicy("ask")
	setIssueDue(st.w, 1, "2027-02-01")
	setTaskDue("2026-09-09")
	// writebackRows prefers a sent row per kind, and the earlier cases left two.
	drainOutboxUntil(t, func() bool {
		var n int
		_ = testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM gitlab_writebacks WHERE task_id = $1::uuid AND status = 'conflict'`, st.taskID).Scan(&n)
		return n == 1
	})
	conflicts := c.get("/workspaces/" + s.WS + "/gitlab/conflicts").listBody(t)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want 1", conflicts)
	}
	c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/conflicts/resolve", map[string]any{
		"items": []map[string]any{{
			"task_id": st.taskID, "conflict_id": conflicts[0]["id"],
			"resolution": "manual", "value": map[string]string{"due": "2026-09-30"},
		}},
	}), http.StatusOK)
	if got := taskDue(); got != "2026-09-30" {
		t.Fatalf("batch manual: task due = %q, want 2026-09-30", got)
	}
	if left := c.get("/workspaces/" + s.WS + "/gitlab/conflicts").listBody(t); len(left) != 0 {
		t.Fatalf("conflicts after the batch = %v", left)
	}
}
