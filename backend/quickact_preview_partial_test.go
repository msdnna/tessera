package main

import (
	"net/http"
	"strings"
	"testing"
)

// The dry run under the comment editor («Будет применено: …») and the real run
// must agree. They diverged on the two commands that take a *list* of arguments:
// `/assign` and `/tag` apply what they can resolve and name the rest as skipped,
// while the preview refused the whole command as soon as one entry was unknown.
// So `/assign @known @typo` was shown as a failure and then went on to assign
// @known — the hint said the opposite of what happened.

// previewCommands runs the dry run and returns its per-command outcomes.
func previewCommands(t *testing.T, c *client, taskID, body string) []map[string]any {
	t.Helper()
	m := c.expect(t, c.post("/tasks/"+taskID+"/commands/preview",
		map[string]any{"body": body}), http.StatusOK)
	raw, _ := m["commands"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		oc, _ := item.(map[string]any)
		out = append(out, oc)
	}
	return out
}

// outcomeText reports whether an outcome carries an error, plus its text.
func outcomeText(oc map[string]any) (string, string) {
	summary, _ := oc["summary"].(string)
	errText, _ := oc["error"].(string)
	return summary, errText
}

// TestPreviewMatchesApplyOnPartialAssign is the headline case: one known login,
// one typo. The preview must promise the partial success the run delivers.
func TestPreviewMatchesApplyOnPartialAssign(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)
	known, _, _ := strings.Cut(member.Email, "@")

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Превью против применения")
	taskID := task["id"].(string)
	body := "/assign @" + known + " @такого-нет"

	cmds := previewCommands(t, owner, taskID, body)
	if len(cmds) != 1 {
		t.Fatalf("preview returned %d commands, want 1", len(cmds))
	}
	previewSummary, previewErr := outcomeText(cmds[0])
	if previewErr != "" {
		t.Fatalf("preview refused a partially resolvable /assign: %q — the run applies it", previewErr)
	}
	if !strings.Contains(previewSummary, known) {
		t.Fatalf("preview summary = %q, want the resolvable login %q in it", previewSummary, known)
	}
	if !strings.Contains(previewSummary, "такого-нет") {
		t.Fatalf("preview summary = %q, want the unresolvable login named as skipped", previewSummary)
	}

	r := owner.post("/tasks/"+taskID+"/comments", map[string]any{"body": body})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	applied, failed := commandSummary(t, r)
	if len(applied) != 1 || len(failed) != 0 {
		t.Fatalf("run applied=%v failed=%v, want the command applied — preview promised it", applied, failed)
	}

	assignees := owner.get("/tasks/" + taskID).mapBody(t)["assignees"]
	list, _ := assignees.([]any)
	if len(list) != 1 {
		t.Fatalf("task has %d assignees, want 1", len(list))
	}
}

// TestPreviewMatchesApplyOnPartialTag is the same divergence on the other
// list-taking command.
func TestPreviewMatchesApplyOnPartialTag(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "backend", "color": "#7c5cff"}), http.StatusCreated)

	task := mkTask(t, c, s.Board, s.col(t, 0), "Теги в превью")
	taskID := task["id"].(string)
	body := "/tag backend, такого-тега-нет"

	cmds := previewCommands(t, c, taskID, body)
	if len(cmds) != 1 {
		t.Fatalf("preview returned %d commands, want 1", len(cmds))
	}
	previewSummary, previewErr := outcomeText(cmds[0])
	if previewErr != "" {
		t.Fatalf("preview refused a partially resolvable /tag: %q — the run applies it", previewErr)
	}
	if !strings.Contains(previewSummary, "backend") || !strings.Contains(previewSummary, "такого-тега-нет") {
		t.Fatalf("preview summary = %q, want both the applied and the skipped tag", previewSummary)
	}

	r := c.post("/tasks/"+taskID+"/comments", map[string]any{"body": body})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	if applied, failed := commandSummary(t, r); len(applied) != 1 || len(failed) != 0 {
		t.Fatalf("run applied=%v failed=%v, want the command applied", applied, failed)
	}
}

// TestPreviewStillRefusesWhenNothingResolves is the negative control: the
// preview must keep failing when *no* entry resolves, exactly as the run does.
func TestPreviewStillRefusesWhenNothingResolves(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Ничего не нашлось")
	taskID := task["id"].(string)

	for _, body := range []string{"/assign @нет-такого @и-такого", "/tag нет-тега, и-этого-нет"} {
		cmds := previewCommands(t, c, taskID, body)
		if len(cmds) != 1 {
			t.Fatalf("preview of %q returned %d commands, want 1", body, len(cmds))
		}
		if _, previewErr := outcomeText(cmds[0]); previewErr == "" {
			t.Fatalf("preview of %q reported no error, want one — nothing resolves", body)
		}
		r := c.post("/tasks/"+taskID+"/comments", map[string]any{"body": body})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("run of %q: status %d, want 400\n%s", body, r.Status, r.Body)
		}
	}
}

// TestPreviewNamesEveryUnresolvedEntry guards the error text itself: the run
// lists all the misses, and the preview used to stop at the first one.
func TestPreviewNamesEveryUnresolvedEntry(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Все промахи")
	taskID := task["id"].(string)

	cmds := previewCommands(t, c, taskID, "/assign @первый-нет @второй-нет")
	_, previewErr := outcomeText(cmds[0])
	if !strings.Contains(previewErr, "первый-нет") || !strings.Contains(previewErr, "второй-нет") {
		t.Fatalf("preview error = %q, want both unresolved logins named", previewErr)
	}
}
