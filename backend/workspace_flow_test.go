package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// registerAs registers a user with a specific email (for invitation flows where
// the email must match) and returns an authenticated client.
func registerAs(t *testing.T, email string) *client {
	t.Helper()
	r := doReq(t, "", http.MethodPost, "/auth/register", map[string]any{
		"email": email, "name": "Invited User", "password": "password-123",
	})
	if r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("register %s: status %d\n%s", email, r.Status, r.Body)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("register decode: %v\n%s", err, r.Body)
	}
	return &client{t: t, token: out.AccessToken, Refresh: out.RefreshToken, UserID: out.User.ID, Email: email}
}

// listHasID reports whether a JSON array of objects contains an "id" equal to id.
func listHasID(list []map[string]any, id string) bool {
	for _, m := range list {
		if m["id"] == id {
			return true
		}
	}
	return false
}

// Workspace CRUD: create/get/patch, owner-only delete.
func TestWorkspaceCRUD(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	other := signup(t)

	ws := owner.expect(t, owner.post("/workspaces", map[string]any{"name": "WS crud"}), http.StatusCreated)
	wsID := ws["id"].(string)
	if ws["owner_id"] != owner.UserID {
		t.Fatalf("owner_id = %v, want %s", ws["owner_id"], owner.UserID)
	}

	got := owner.expect(t, owner.get("/workspaces/"+wsID), http.StatusOK)
	if got["name"] != "WS crud" {
		t.Fatalf("get name = %v", got["name"])
	}

	// Non-member cannot read it (requireMember → 403).
	other.expect(t, other.get("/workspaces/"+wsID), http.StatusForbidden)

	upd := owner.expect(t, owner.patch("/workspaces/"+wsID, map[string]any{"name": "WS renamed"}), http.StatusOK)
	if upd["name"] != "WS renamed" {
		t.Fatalf("patch name = %v", upd["name"])
	}

	// Non-owner delete → 403 (member or not: only the owner may delete).
	other.expect(t, other.del("/workspaces/"+wsID), http.StatusForbidden)
	owner.expect(t, owner.del("/workspaces/"+wsID), http.StatusNoContent)
	owner.expect(t, owner.get("/workspaces/"+wsID), http.StatusForbidden) // membership gone with the workspace
}

// Members: add by email, change role, remove; plain members can't manage.
func TestWorkspaceMembers(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	member := signup(t)
	stranger := signup(t)
	wsID := mkWorkspace(t, owner, "WS members")

	// Unknown email → 404.
	r := owner.post("/workspaces/"+wsID+"/members", map[string]any{"email": "nobody-here@test.local"})
	if r.Status != http.StatusNotFound {
		t.Fatalf("add unknown email: status %d\n%s", r.Status, r.Body)
	}

	// Add by email, default role "member".
	m := owner.expect(t, owner.post("/workspaces/"+wsID+"/members", map[string]any{"email": member.Email}), http.StatusCreated)
	if m["role"] != "member" || m["user_id"] != member.UserID {
		t.Fatalf("add member: %v", m)
	}

	members := owner.get("/workspaces/" + wsID + "/members").listBody(t)
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}

	// A plain member cannot manage members (requireManager → 403).
	member.expect(t, member.post("/workspaces/"+wsID+"/members", map[string]any{"email": stranger.Email}), http.StatusForbidden)
	member.expect(t, member.patch("/workspaces/"+wsID+"/members/"+member.UserID, map[string]any{"role": "admin"}), http.StatusForbidden)
	member.expect(t, member.del("/workspaces/"+wsID+"/members/"+member.UserID), http.StatusForbidden)
	// ...nor rename the workspace.
	member.expect(t, member.patch("/workspaces/"+wsID, map[string]any{"name": "hijack"}), http.StatusForbidden)

	// Owner promotes to admin.
	up := owner.expect(t, owner.patch("/workspaces/"+wsID+"/members/"+member.UserID, map[string]any{"role": "admin"}), http.StatusOK)
	if up["role"] != "admin" {
		t.Fatalf("role after promote = %v", up["role"])
	}
	// Invalid role → 400; owner's own role untouchable → 403.
	if r := owner.patch("/workspaces/"+wsID+"/members/"+member.UserID, map[string]any{"role": "owner"}); r.Status != http.StatusBadRequest {
		t.Fatalf("grant owner role: status %d", r.Status)
	}
	owner.expect(t, owner.patch("/workspaces/"+wsID+"/members/"+owner.UserID, map[string]any{"role": "member"}), http.StatusForbidden)
	// Owner cannot be removed.
	owner.expect(t, owner.del("/workspaces/"+wsID+"/members/"+owner.UserID), http.StatusForbidden)

	// Remove the member.
	owner.expect(t, owner.del("/workspaces/"+wsID+"/members/"+member.UserID), http.StatusNoContent)
	members = owner.get("/workspaces/" + wsID + "/members").listBody(t)
	if len(members) != 1 {
		t.Fatalf("members after remove = %d, want 1", len(members))
	}
	member.expect(t, member.get("/workspaces/"+wsID), http.StatusForbidden)
}

// invitationToken extracts the raw token from the "link" field of a created
// invitation ("…/invite?token=<raw>").
func invitationToken(t *testing.T, inv map[string]any) string {
	t.Helper()
	link, _ := inv["link"].(string)
	i := strings.Index(link, "token=")
	if i < 0 {
		t.Fatalf("invitation link has no token: %v", inv)
	}
	return link[i+len("token="):]
}

// Invitations: create/list/delete, accept-by-token, auto-accept on register.
func TestWorkspaceInvitations(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	registered := signup(t)
	wsID := mkWorkspace(t, owner, "WS invites")

	// Invite a not-yet-registered email.
	freshEmail := fmt.Sprintf("invitee-%s@test.local", strings.ToLower(strings.ReplaceAll(owner.UserID[:8], "-", "")))
	inv1 := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": freshEmail, "role": "member"}), http.StatusCreated)
	if inv1["email"] != freshEmail {
		t.Fatalf("invitation email = %v", inv1["email"])
	}

	// Invite an already-registered user too.
	inv2 := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": registered.Email, "role": "admin"}), http.StatusCreated)
	tok2 := invitationToken(t, inv2)

	// A second, revocable invitation for the delete path.
	inv3 := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": "revoked@test.local"}), http.StatusCreated)

	list := owner.get("/workspaces/" + wsID + "/invitations").listBody(t)
	if len(list) != 3 {
		t.Fatalf("invitations = %d, want 3\n%v", len(list), list)
	}
	// Listing is manager-only.
	registered.expect(t, registered.get("/workspaces/"+wsID+"/invitations"), http.StatusForbidden)

	owner.expect(t, owner.del("/workspaces/"+wsID+"/invitations/"+inv3["id"].(string)), http.StatusNoContent)
	if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 2 {
		t.Fatalf("invitations after delete = %d, want 2", got)
	}

	// Accept with a token belonging to a different email → 403.
	third := signup(t)
	third.expect(t, third.post("/invitations/accept", map[string]any{"token": tok2}), http.StatusForbidden)

	// The invited registered user accepts → 200 + the workspace payload.
	acc := registered.expect(t, registered.post("/invitations/accept", map[string]any{"token": tok2}), http.StatusOK)
	if acc["id"] != wsID {
		t.Fatalf("accept returned ws %v, want %s", acc["id"], wsID)
	}
	if !listHasID(registered.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("accepted workspace missing from /workspaces")
	}
	// Reusing a consumed token → 400.
	if r := registered.post("/invitations/accept", map[string]any{"token": tok2}); r.Status != http.StatusBadRequest {
		t.Fatalf("reuse token: status %d", r.Status)
	}

	// Registering with the invited email auto-accepts the pending invitation.
	newcomer := registerAs(t, freshEmail)
	if !listHasID(newcomer.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("auto-accept on register: workspace not joined")
	}
}

// Summary counters and per-user workspace list scoping.
func TestWorkspaceSummaryAndListScoping(t *testing.T) {
	t.Parallel()
	c := signup(t)
	other := signup(t)
	s := mkStack(t, c)

	mkTask(t, c, s.Board, s.col(t, 0), "Открытая задача")
	done := mkTask(t, c, s.Board, s.col(t, 0), "Готовая задача")
	// Toggle completed via full update.
	c.expect(t, c.patch("/tasks/"+done["id"].(string), map[string]any{
		"title": "Готовая задача", "completed": true,
	}), http.StatusOK)

	sum := c.expect(t, c.get("/workspaces/"+s.WS+"/summary"), http.StatusOK)
	if sum["total"] != float64(2) || sum["completed"] != float64(1) || sum["active"] != float64(1) {
		t.Fatalf("summary = %v", sum)
	}

	// Only members see the workspace in their list.
	if !listHasID(c.get("/workspaces").listBody(t), s.WS) {
		t.Fatalf("owner does not see own workspace")
	}
	if listHasID(other.get("/workspaces").listBody(t), s.WS) {
		t.Fatalf("stranger sees a foreign workspace")
	}
	other.expect(t, other.get("/workspaces/"+s.WS+"/summary"), http.StatusForbidden)
}

// Every summary bucket, including the day boundaries. The counts are derived in SQL
// from caller-supplied midnights (WorkspaceTaskSummary), so the edges matter: a task
// due exactly at today+7d is outside due_week, a completed task is never overdue, and
// subtasks are not counted at all.
func TestWorkspaceSummaryBuckets(t *testing.T) {
	t.Parallel()
	c := signup(t)
	helper := signup(t)
	s := mkStack(t, c)
	c.expect(t, c.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": helper.Email}), http.StatusCreated)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	col := s.col(t, 0)

	due := func(title string, at time.Time) map[string]any {
		t.Helper()
		return c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": col, "due_date": at.Format(time.RFC3339),
		}), http.StatusCreated)
	}

	due("просрочена", today.AddDate(0, 0, -1).Add(12*time.Hour))
	due("сегодня", today.Add(time.Hour))
	due("через три дня", today.AddDate(0, 0, 3))
	due("ровно через неделю", today.AddDate(0, 0, 7)) // week_end is exclusive
	due("через десять дней", today.AddDate(0, 0, 10))

	// Completed tasks drop out of the due buckets even when the date has passed.
	pastDone := due("готовая просроченная", today.AddDate(0, 0, -2))
	c.expect(t, c.patch("/tasks/"+pastDone["id"].(string), map[string]any{
		"title": "готовая просроченная", "completed": true,
	}), http.StatusOK)

	// Assignment: one task on the caller, one on someone else (so "assigned" is the
	// caller's own count, not "has any assignee").
	mine := mkTask(t, c, s.Board, col, "моя")
	c.expect(t, c.post("/tasks/"+mine["id"].(string)+"/assignees",
		map[string]any{"user_id": c.UserID}), http.StatusNoContent)
	theirs := mkTask(t, c, s.Board, col, "чужая")
	c.expect(t, c.post("/tasks/"+theirs["id"].(string)+"/assignees",
		map[string]any{"user_id": helper.UserID}), http.StatusNoContent)

	// A subtask is excluded from every count, overdue date notwithstanding.
	child := due("подзадача просрочена", today.AddDate(0, 0, -1))
	c.expect(t, c.patch("/tasks/"+child["id"].(string)+"/parent",
		map[string]any{"parent_id": mine["id"].(string)}), http.StatusOK)

	sum := c.expect(t, c.get("/workspaces/"+s.WS+"/summary"), http.StatusOK)
	for _, want := range []struct {
		key string
		n   float64
	}{
		{"total", 8}, {"completed", 1}, {"active", 7}, {"assigned", 1},
		{"overdue", 1}, {"due_today", 1}, {"due_week", 2}, {"unassigned", 6},
	} {
		if sum[want.key] != want.n {
			t.Errorf("summary[%s] = %v, want %v\nfull: %v", want.key, sum[want.key], want.n, sum)
		}
	}
}
