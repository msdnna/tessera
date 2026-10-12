// Admin panel v2 (#3064): the users list knows each person's workspaces and
// when they were last in, and «Приглашения» lists every open invitation of the
// instance with a revoke that ignores workspace membership.
package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestAdminUsersSpacesAndInvitations(t *testing.T) {
	t.Parallel()
	adm := signup(t)
	owner := signup(t)
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = true WHERE id = $1`, adm.UserID); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = false WHERE id = $1`, owner.UserID); err != nil {
		t.Fatalf("demote owner: %v", err)
	}
	wsID := mkWorkspace(t, owner, "WS admin v2")

	// «Пространства» and «Последний вход»: signing up issued tokens, so the
	// owner has been seen; the workspace is listed with the owner's role.
	var seen map[string]any
	for _, u := range adm.get("/admin/users").listBody(t) {
		if u["email"] == owner.Email {
			seen = u
		}
	}
	if seen == nil {
		t.Fatalf("admin list misses %s", owner.Email)
	}
	if seen["last_seen_at"] == nil {
		t.Fatalf("last_seen_at is empty for a user who signed in: %v", seen)
	}
	spaces, _ := seen["workspaces"].([]any)
	found := false
	for _, s := range spaces {
		m, _ := s.(map[string]any)
		if m["id"] == wsID && m["name"] == "WS admin v2" && m["role"] == "owner" {
			found = true
		}
	}
	if !found {
		t.Fatalf("workspaces of the owner miss %s: %v", wsID, spaces)
	}

	// «Приглашения»: an invitation of a workspace the admin is not in.
	email := fmt.Sprintf("adm-v2-%s@test.local", owner.UserID[:8])
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)
	invID, _ := inv["id"].(string)
	var row map[string]any
	for _, r := range adm.get("/admin/invitations").listBody(t) {
		if r["id"] == invID {
			row = r
		}
	}
	if row == nil {
		t.Fatalf("admin invitations miss %s", invID)
	}
	if row["workspace_name"] != "WS admin v2" || row["email"] != email || row["invited_by_name"] == "" {
		t.Fatalf("invitation row: %v", row)
	}

	// Only the instance admin sees and revokes them.
	if r := owner.get("/admin/invitations"); r.Status != http.StatusForbidden {
		t.Fatalf("non-admin list invitations: status %d, want 403", r.Status)
	}
	if r := owner.del("/admin/invitations/" + invID); r.Status != http.StatusForbidden {
		t.Fatalf("non-admin revoke: status %d, want 403", r.Status)
	}
	if r := adm.del("/admin/invitations/" + invID); r.Status != http.StatusNoContent {
		t.Fatalf("admin revoke: status %d, want 204", r.Status)
	}
	if r := adm.del("/admin/invitations/" + invID); r.Status != http.StatusNotFound {
		t.Fatalf("revoke twice: status %d, want 404", r.Status)
	}
	for _, r := range owner.get("/workspaces/" + wsID + "/invitations").listBody(t) {
		if r["id"] == invID {
			t.Fatalf("revoked invitation still listed in the workspace")
		}
	}
}
