package main

import (
	"net/http"
	"strings"
	"testing"
)

// A pinned GitLab assignee must not carry an avatar URL the caller chose.
//
// Every other place that puts a GitLab avatar in front of a client wraps it in
// avatarProxyURL — a same-origin "/api/gitlab/avatar?…&sig=…" link the server
// signs and fetches itself. That is the whole reason GitlabAvatar exists: the
// browser never talks to the GitLab instance (or anywhere else) to paint a face.
//
// The pin route took gl_avatar_url straight off the request body, so any member
// could pin an assignee whose "avatar" was an arbitrary absolute URL. That URL is
// rendered as <img :src> by every client that shows the task (TaskModal,
// TaskCardPills, the mention list, the facet menus — and MemberAvatar on Android),
// the SPA sets no img-src policy and the Android client has none at all, so the
// address turns into a beacon: its owner learns the IP, the user agent and the
// exact moment of everyone who opened the task (#2932).
func TestPinnedGitlabAssigneeIgnoresCallerAvatarURL(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Pin a GitLab member")
	taskID := task["id"].(string)

	const beacon = "https://evil.example/beacon.png?who=viewer"
	c.expect(t, c.post("/tasks/"+taskID+"/gitlab-assignees", map[string]any{
		"gl_username":   "colleague",
		"gl_name":       "Colleague",
		"gl_avatar_url": beacon,
	}), http.StatusNoContent)

	got := gitlabAssigneeAvatar(t, c, taskID, "colleague")
	if strings.Contains(got, "evil.example") {
		t.Fatalf("pinned assignee kept the caller's avatar URL %q — it is painted as <img src> for every viewer", got)
	}
	// No roster row for this username, so there is no avatar to show: the clients
	// already fall back to initials on an empty string.
	if got != "" {
		t.Fatalf("pinned assignee avatar = %q, want empty (no roster entry)", got)
	}
}

// With a synced roster the pin takes the member's avatar from that roster — the
// URL the sync already ran through avatarProxyURL — whatever the caller sent.
func TestPinnedGitlabAssigneeTakesAvatarFromRoster(t *testing.T) {
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "glbot", "group/app")
	f.members = []glMember{
		{ID: 99, Username: "glbot", Name: "GL Bot"},
		{ID: 12, Username: "colleague", Name: "Colleague", Avatar: "https://gl.example/uploads/-/system/user/avatar/12/avatar.png"},
	}
	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	// What the roster holds is what the UI is allowed to load.
	var want string
	for _, m := range c.get("/workspaces/" + s.WS + "/gitlab/members").listBody(t) {
		if m["gl_username"] == "colleague" {
			want, _ = m["gl_avatar_url"].(string)
		}
	}
	if !strings.HasPrefix(want, "/api/gitlab/avatar?") {
		t.Fatalf("roster avatar = %q, want a signed same-origin proxy URL", want)
	}

	task := mkTask(t, c, s.Board, s.col(t, 0), "Pin a rostered member")
	taskID := task["id"].(string)
	c.expect(t, c.post("/tasks/"+taskID+"/gitlab-assignees", map[string]any{
		"gl_username":   "colleague",
		"gl_name":       "Not Colleague",
		"gl_avatar_url": "https://evil.example/beacon.png",
	}), http.StatusNoContent)

	if got := gitlabAssigneeAvatar(t, c, taskID, "colleague"); got != want {
		t.Fatalf("pinned assignee avatar = %q, want the roster's %q", got, want)
	}
}

// gitlabAssigneeAvatar reads one pinned GitLab assignee's avatar back off the
// task detail — the payload every client paints from.
func gitlabAssigneeAvatar(t *testing.T, c *client, taskID, username string) string {
	t.Helper()
	detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	raw, _ := detail["gitlab_assignees"].([]any)
	for _, item := range raw {
		a, ok := item.(map[string]any)
		if !ok || a["gl_username"] != username {
			continue
		}
		avatar, _ := a["gl_avatar_url"].(string)
		return avatar
	}
	t.Fatalf("task %s has no pinned GitLab assignee %q: %v", taskID, username, raw)
	return ""
}
