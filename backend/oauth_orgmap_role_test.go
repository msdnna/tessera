// org_map must not take away a role it never gave.
//
// "Continue with GitLab" re-applies the group→workspace mapping on every login,
// and CreateMembership is an upsert that rewrites the role of anyone but the
// owner. So a workspace admin who happens to sit in a mapped group with
// `users: true` — and is not spelled out in that entry's `admins` — is demoted
// to plain member by their own sign-in. The owner re-grants, the next login
// takes it away again. The mapping is documented as additive; only the grant
// belongs to GitLab, the local role is Tessera's to keep.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newFakeIDPWithGroups is newFakeIDP plus the /groups endpoint applyOrgMap
// reads; without it the group lookup fails and the mapping is skipped entirely
// (best-effort by design), which would make these tests pass for the wrong
// reason.
func newFakeIDPWithGroups(t *testing.T, glID int64, username, email, groupPath string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-access-token"})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": glID, "username": username, "email": email, "name": "GitLab User",
		})
	})
	mux.HandleFunc("/api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// One page, then the paginator stops (no X-Next-Page).
		if r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"full_path": groupPath}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// configureIDPWithOrgMap points the instance at the fake and installs a mapping.
// Sequential-only, like configureIDP: the provider row is a global singleton.
func configureIDPWithOrgMap(t *testing.T, baseURL string, orgMap map[string]any) {
	t.Helper()
	admin := signup(t)
	makeAdmin(t, admin)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`UPDATE oauth_providers SET gl_base_url = '', enabled = false, client_id = '',
			    client_secret_enc = '', org_map = '{}'::jsonb WHERE provider = 'gitlab'`)
	})
	admin.expect(t, admin.put("/admin/oauth/gitlab", map[string]any{
		"gl_base_url": baseURL, "client_id": "tessera-app", "client_secret": "s3cret",
		"enabled": true, "org_map": orgMap,
	}), http.StatusOK)
}

// roleOf reads a user's membership role straight from the table — the members
// list is manager-gated and the point here is the row, not the view.
func roleOf(t *testing.T, wsID, userID string) string {
	t.Helper()
	var role string
	if err := testPool.QueryRow(context.Background(),
		`SELECT role FROM memberships WHERE workspace_id = $1::uuid AND user_id = $2::uuid`,
		wsID, userID).Scan(&role); err != nil {
		t.Fatalf("read membership role: %v", err)
	}
	return role
}

// A workspace admin in a mapped group keeps their role through a GitLab login.
func TestOrgMapDoesNotDemoteWorkspaceAdmin(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	email := fmt.Sprintf("gl-admin-%d@test.local", n)
	group := fmt.Sprintf("acme/dev-%d", n)

	owner := signup(t)
	wsID := mkWorkspace(t, owner, fmt.Sprintf("OrgMap WS %d", n))

	member := registerWith(t, email, "")
	verifyEmailOf(t, member)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": email, "role": "admin"}), http.StatusCreated)

	configureIDPWithOrgMap(t, newFakeIDPWithGroups(t, 930000+n, fmt.Sprintf("gl-admin-%d", n), email, group).URL,
		map[string]any{group: map[string]any{"workspace_id": wsID, "users": true}})

	frag := gitlabSignIn(t)
	if tok := frag.Get("access_token"); tok == "" {
		t.Fatalf("подтверждённая учётка не вошла через GitLab: %v", frag)
	}
	if got := roleOf(t, wsID, member.UserID); got != "admin" {
		t.Fatalf("вход через GitLab понизил роль в пространстве: %q, want admin — org_map выдаёт доступ, а не отбирает", got)
	}
}

// The grant itself still works: a mapped group with users:true makes a stranger
// a member.
func TestOrgMapGrantsMembership(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	email := fmt.Sprintf("gl-grant-%d@test.local", n)
	group := fmt.Sprintf("acme/grant-%d", n)

	owner := signup(t)
	wsID := mkWorkspace(t, owner, fmt.Sprintf("OrgMap Grant WS %d", n))

	newcomer := registerWith(t, email, "")
	verifyEmailOf(t, newcomer)

	configureIDPWithOrgMap(t, newFakeIDPWithGroups(t, 940000+n, fmt.Sprintf("gl-grant-%d", n), email, group).URL,
		map[string]any{group: map[string]any{"workspace_id": wsID, "users": true}})

	frag := gitlabSignIn(t)
	if tok := frag.Get("access_token"); tok == "" {
		t.Fatalf("учётка не вошла через GitLab: %v", frag)
	}
	if got := roleOf(t, wsID, newcomer.UserID); got != "member" {
		t.Fatalf("org_map не выдал членство: %q, want member", got)
	}
}

// And the admins list still promotes: a plain member named there comes back an
// admin. This is the one direction the mapping is allowed to rewrite a role.
func TestOrgMapPromotesListedAdmin(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	email := fmt.Sprintf("gl-promote-%d@test.local", n)
	username := fmt.Sprintf("gl-promote-%d", n)
	group := fmt.Sprintf("acme/promote-%d", n)

	owner := signup(t)
	wsID := mkWorkspace(t, owner, fmt.Sprintf("OrgMap Promote WS %d", n))

	member := registerWith(t, email, "")
	verifyEmailOf(t, member)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)

	configureIDPWithOrgMap(t, newFakeIDPWithGroups(t, 950000+n, username, email, group).URL,
		map[string]any{group: map[string]any{"workspace_id": wsID, "users": true, "admins": []string{username}}})

	frag := gitlabSignIn(t)
	if tok := frag.Get("access_token"); tok == "" {
		t.Fatalf("учётка не вошла через GitLab: %v", frag)
	}
	if got := roleOf(t, wsID, member.UserID); got != "admin" {
		t.Fatalf("org_map не поднял до admin по списку admins: %q", got)
	}
}
