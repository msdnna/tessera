package main

import (
	"context"
	"net/http"
	"testing"
)

// The member list carries each member's GitLab login so the frontend's
// @-autocomplete can insert `@login` rather than a display name — a name with
// spaces resolves to nothing once the comment is pushed to GitLab (#2715).
//
// The second identity is the point of the test, not padding: oauth_identities
// is unique by (provider, provider_user_id), not by user_id, so a user linked
// to two GitLab instances would duplicate their row under a naive LEFT JOIN.
func TestListMembersCarriesGitlabLogin(t *testing.T) {
	owner := signup(t)
	s := mkStack(t, owner)

	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)

	for _, id := range []struct{ providerUserID, username string } {
		{"9001", "e.polyansky"},
		{"9002", "e.polyansky.mirror"},
	} {
		if _, err := testPool.Exec(context.Background(),
			`INSERT INTO oauth_identities (user_id, provider, provider_user_id, provider_username)
			 VALUES ($1, 'gitlab', $2, $3)`, member.UserID, id.providerUserID, id.username); err != nil {
			t.Fatalf("seed oauth identity %s: %v", id.providerUserID, err)
		}
	}

	rows := owner.get("/workspaces/" + s.WS + "/members").listBody(t)
	if len(rows) != 2 {
		t.Fatalf("members = %d, want 2 (owner + member) — a second identity must not duplicate a row\n%v", len(rows), rows)
	}

	var seen bool
	for _, m := range rows {
		if m["user_id"] != member.UserID {
			continue
		}
		seen = true
		// Either identity is an acceptable pick; the query orders by created_at
		// then id, so it is deterministic, not arbitrary.
		if got := m["gl_username"]; got != "e.polyansky" && got != "e.polyansky.mirror" {
			t.Fatalf("gl_username = %v, want one of the seeded logins", got)
		}
	}
	if !seen {
		t.Fatalf("seeded member missing from /members\n%v", rows)
	}

	// A member with no GitLab identity reports an empty login (not null), so the
	// frontend's `m.gl_username || …` fallback chain stays honest.
	for _, m := range rows {
		if m["user_id"] == owner.UserID && m["gl_username"] != "" {
			t.Fatalf("owner gl_username = %v, want \"\"", m["gl_username"])
		}
	}
}
