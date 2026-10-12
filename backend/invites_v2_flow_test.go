// Invitations v2 (#3035: #3059 preview and decline, #3061 resend / extend,
// #3062 the shared invitation link).
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func tokenOf(link string) string {
	if i := strings.Index(link, "token="); i >= 0 {
		return link[i+len("token="):]
	}
	return link[strings.LastIndex(link, "/")+1:]
}

func freshEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@test.local", prefix, userSeq.Add(1))
}

// The preview tells the token holder who invites them into what, in counts;
// declining closes the invitation for good, including the verified-address path.
func TestInvitationPreviewAndDecline(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Студия «Север»")
	g := mkGroup(t, owner, wsID, "Группа")
	mkProject(t, owner, wsID, g, "Проект 1")
	mkProject(t, owner, wsID, g, "Проект 2")
	email := freshEmail("preview")
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations", map[string]any{"email": email}), http.StatusCreated)
	tok := tokenOf(inv["link"].(string))

	p := doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": tok}, http.StatusOK)
	ws := p["workspace"].(map[string]any)
	if p["kind"] != "email" || p["state"] != "pending" || ws["name"] != "Студия «Север»" || p["role"] != "member" {
		t.Fatalf("preview: %v", p)
	}
	if p["projects"].(float64) != 2 || p["members"].(float64) != 1 || p["email"] != email {
		t.Fatalf("preview counts: %v", p)
	}
	if p["inviter"].(map[string]any)["name"] == "" {
		t.Fatalf("preview must name the inviter: %v", p)
	}
	if _, ok := p["members_list"]; ok {
		t.Fatal("the preview must not list people")
	}
	doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": "nope"}, http.StatusNotFound)

	doReqExpect(t, http.MethodPost, "/invitations/decline", map[string]any{"token": tok}, http.StatusNoContent)
	doReqExpect(t, http.MethodPost, "/invitations/decline", map[string]any{"token": tok}, http.StatusBadRequest)
	p = doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": tok}, http.StatusOK)
	if p["state"] != "declined" || p["workspace"] != nil {
		t.Fatalf("a declined invitation describes nothing: %v", p)
	}

	// The addressee signs up and proves the address: the declined invitation
	// stays declined, and the token itself is refused.
	invitee := registerWith(t, email, tok)
	verifyByCode(t, invitee)
	if isMember(t, wsID, invitee.UserID) {
		t.Fatal("a declined invitation was applied")
	}
	invitee.expect(t, invitee.post("/invitations/accept", map[string]any{"token": tok}), http.StatusBadRequest)

	list := owner.get("/workspaces/" + wsID + "/invitations").listBody(t)
	if len(list) != 1 || list[0]["declined_at"] == nil {
		t.Fatalf("the manager sees the refusal: %v", list)
	}
}

func isMember(t *testing.T, wsID, userID string) bool {
	t.Helper()
	var ok bool
	if err := testPool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM memberships WHERE workspace_id = $1 AND user_id = $2)`, wsID, userID).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// «Ещё раз» / «Продлить»: a new token and a fresh week, the old link dead,
// paused between sends, scoped to the workspace, managers only.
func TestInvitationResend(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Resend")
	otherWS := mkWorkspace(t, owner, "Elsewhere")
	email := freshEmail("resend")
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations", map[string]any{"email": email}), http.StatusCreated)
	invID := inv["id"].(string)
	oldTok := tokenOf(inv["link"].(string))
	testMail.waitLetter(t, email, 0)

	owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations/"+invID+"/resend", nil), http.StatusTooManyRequests)
	// Expired and declined, to exercise «Продлить» on the worst case.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE workspace_invitations SET sent_at = now() - interval '2 minutes',
		 expires_at = now() - interval '1 minute', declined_at = now() WHERE id = $1`, invID); err != nil {
		t.Fatal(err)
	}
	owner.expect(t, owner.post("/workspaces/"+otherWS+"/invitations/"+invID+"/resend", nil), http.StatusNotFound)
	stranger := signup(t)
	stranger.expect(t, stranger.post("/workspaces/"+wsID+"/invitations/"+invID+"/resend", nil), http.StatusForbidden)

	m := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations/"+invID+"/resend", nil), http.StatusOK)
	newTok := tokenOf(m["link"].(string))
	if newTok == oldTok || m["declined_at"] != nil || m["send_count"].(float64) != 2 {
		t.Fatalf("resend: %v", m)
	}
	letter := testMail.waitLetter(t, email, 1)
	if !strings.Contains(letter.body, newTok) {
		t.Fatalf("the new letter must carry the new link:\n%s", letter.body)
	}
	doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": oldTok}, http.StatusNotFound)
	p := doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": newTok}, http.StatusOK)
	if p["state"] != "pending" {
		t.Fatalf("the reissued invitation is live again: %v", p)
	}
	invitee := registerWith(t, email, newTok)
	if !isMember(t, wsID, invitee.UserID) {
		t.Fatal("the reissued link did not admit its addressee")
	}
	// Accepted: nothing left to resend.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE workspace_invitations SET sent_at = now() - interval '2 minutes' WHERE id = $1`, invID); err != nil {
		t.Fatal(err)
	}
	owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations/"+invID+"/resend", nil), http.StatusNotFound)
}

// The shared link: shown once, hashed at rest, limited, never spends a place on
// someone already inside, and «Сбросить» kills the previous one.
func TestInviteLinkLifecycle(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Link space")
	base := "/workspaces/" + wsID + "/invite-link"

	if m := owner.expect(t, owner.get(base), http.StatusOK); m["link"] != nil {
		t.Fatalf("no link yet: %v", m)
	}
	for _, bad := range []map[string]any{
		{"ttl_days": 31}, {"max_uses": 501}, {"max_uses": -1}, {"role": "owner"},
	} {
		owner.expect(t, owner.post(base, bad), http.StatusBadRequest)
	}
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members", map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)
	member.expect(t, member.post(base, map[string]any{}), http.StatusForbidden)
	member.expect(t, member.get(base), http.StatusForbidden)

	created := owner.expect(t, owner.post(base, map[string]any{"max_uses": 2, "ttl_days": 3}), http.StatusCreated)["link"].(map[string]any)
	url := created["url"].(string)
	if !strings.Contains(url, "/join/") || created["max_uses"].(float64) != 2 {
		t.Fatalf("created link: %v", created)
	}
	tok := tokenOf(url)
	var stored string
	if err := testPool.QueryRow(context.Background(),
		`SELECT token_hash FROM workspace_invite_links WHERE workspace_id = $1`, wsID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, tok) {
		t.Fatal("the link token must be stored hashed")
	}
	got := owner.expect(t, owner.get(base), http.StatusOK)["link"].(map[string]any)
	if _, ok := got["url"]; ok {
		t.Fatalf("the URL is shown once, never listed: %v", got)
	}

	p := doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": tok}, http.StatusOK)
	if p["kind"] != "link" || p["state"] != "pending" || p["email"] != nil {
		t.Fatalf("link preview: %v", p)
	}

	a := signup(t)
	a.expect(t, a.post("/invitations/accept", map[string]any{"token": tok}), http.StatusOK)
	a.expect(t, a.post("/invitations/accept", map[string]any{"token": tok}), http.StatusOK) // already in: no place spent
	// The existing member opening the link spends nothing either.
	member.expect(t, member.post("/invitations/accept", map[string]any{"token": tok}), http.StatusOK)
	if u := owner.expect(t, owner.get(base), http.StatusOK)["link"].(map[string]any)["uses"].(float64); u != 1 {
		t.Fatalf("uses = %v, want 1", u)
	}
	// Signing up with it is the second place.
	b := registerWith(t, freshEmail("link-b"), tok)
	if !isMember(t, wsID, b.UserID) {
		t.Fatal("register with a link token did not join")
	}
	c := signup(t)
	m := c.expect(t, c.post("/invitations/accept", map[string]any{"token": tok}), http.StatusBadRequest)
	if m["error"] != "invalid or expired invitation" {
		t.Fatalf("an exhausted link: %v", m)
	}
	if p := doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": tok}, http.StatusOK); p["state"] != "exhausted" {
		t.Fatalf("exhausted preview: %v", p)
	}
	var audited int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM workspace_invite_link_uses u JOIN workspace_invite_links l ON l.id = u.link_id
		 WHERE l.workspace_id = $1`, wsID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 2 {
		t.Fatalf("every join is recorded: %d", audited)
	}

	// «Сбросить»: the old link is revoked, the new one admits.
	reset := owner.expect(t, owner.post(base, map[string]any{}), http.StatusCreated)["link"].(map[string]any)
	newTok := tokenOf(reset["url"].(string))
	if p := doReqExpect(t, http.MethodPost, "/invitations/lookup", map[string]any{"token": tok}, http.StatusOK); p["state"] != "revoked" {
		t.Fatalf("reset must revoke the previous link: %v", p)
	}
	c.expect(t, c.post("/invitations/accept", map[string]any{"token": newTok}), http.StatusOK)
	// Switched off: nobody else.
	owner.expect(t, owner.del(base), http.StatusNoContent)
	d := signup(t)
	d.expect(t, d.post("/invitations/accept", map[string]any{"token": newTok}), http.StatusBadRequest)
	if m := owner.expect(t, owner.get(base), http.StatusOK); m["link"] != nil {
		t.Fatalf("revoked link still live: %v", m)
	}
}

// The limit holds when many people open the link at once.
func TestInviteLinkLimitUnderRace(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Race")
	link := owner.expect(t, owner.post("/workspaces/"+wsID+"/invite-link", map[string]any{"max_uses": 3}), http.StatusCreated)["link"].(map[string]any)
	tok := tokenOf(link["url"].(string))

	joiners := make([]*client, 10)
	for i := range joiners {
		joiners[i] = signup(t)
	}
	var ok atomic.Int32
	var wg sync.WaitGroup
	for _, j := range joiners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := doReq(t, j.token, http.MethodPost, "/invitations/accept", map[string]any{"token": tok}); r.Status == http.StatusOK {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 {
		t.Fatalf("%d joined, the limit is 3", ok.Load())
	}
}
