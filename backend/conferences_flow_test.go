package main

import (
	"net/http"
	"testing"
)

// mkConference schedules a conference in a fresh workspace stack and returns
// both, so the tests below don't repeat the fixture chain.
func mkConference(t *testing.T, c *client, title string) (s stack, confID string) {
	t.Helper()
	s = mkStack(t, c)
	conf := c.expect(t, c.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": title}), http.StatusCreated)
	return s, conf["id"].(string)
}

// addMember adds a fresh user to a workspace and returns their client.
func addMember(t *testing.T, owner *client, wsID string) *client {
	t.Helper()
	m := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": m.Email}), http.StatusCreated)
	return m
}

// TestConferenceLifecycle walks the whole call: scheduled → live on the first
// join → ended when the room empties. The status is what the list screen colours
// a row by, so each transition is checked where it actually happens rather than
// inferred from the endpoint returning 200.
func TestConferenceLifecycle(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка")

	got := owner.expect(t, owner.get("/conferences/"+confID), http.StatusOK)
	conf := got["conference"].(map[string]any)
	if conf["status"] != "scheduled" {
		t.Fatalf("a freshly created conference is %q, want scheduled", conf["status"])
	}
	// The creator is enrolled as host by the create transaction — a conference
	// nobody can moderate would need a database edit to fix.
	parts := got["participants"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["role"] != "host" {
		t.Fatalf("creator was not enrolled as host: %#v", parts)
	}

	joined := owner.expect(t, owner.post("/conferences/"+confID+"/join", nil), http.StatusOK)
	if st := joined["conference"].(map[string]any)["status"]; st != "live" {
		t.Fatalf("status after the first join = %q, want live", st)
	}
	if joined["participant"].(map[string]any)["joined_at"] == nil {
		t.Fatalf("join did not stamp joined_at: %#v", joined["participant"])
	}

	left := owner.expect(t, owner.post("/conferences/"+confID+"/leave", nil), http.StatusOK)
	if st := left["conference"].(map[string]any)["status"]; st != "ended" {
		t.Fatalf("status after the room emptied = %q, want ended", st)
	}

	// A stale tab that reconnects after the meeting is over must not put the
	// call back on the board as live.
	if r := owner.post("/conferences/"+confID+"/join", nil); r.Status != http.StatusConflict {
		t.Fatalf("re-joining an ended conference: status %d, want 409\n%s", r.Status, r.Body)
	}

	list := owner.get("/workspaces/" + s.WS + "/conferences?status=ended").listBody(t)
	if len(list) != 1 || list[0]["id"] != confID {
		t.Fatalf("status filter did not return the ended conference: %#v", list)
	}
	if live := owner.get("/workspaces/" + s.WS + "/conferences?status=live").listBody(t); len(live) != 0 {
		t.Fatalf("ended conference still listed as live: %#v", live)
	}
}

// TestConferenceRoomEmptiesOnlyWhenEveryoneLeft guards the rule that ends a call
// automatically: it is "the room is empty", not "somebody left". Getting this
// wrong drops the remaining participants out of a meeting that is still running.
func TestConferenceRoomEmptiesOnlyWhenEveryoneLeft(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Планёрка")
	guest := addMember(t, owner, s.WS)

	owner.expect(t, owner.post("/conferences/"+confID+"/join", nil), http.StatusOK)
	guest.expect(t, guest.post("/conferences/"+confID+"/join", nil), http.StatusOK)

	left := owner.expect(t, owner.post("/conferences/"+confID+"/leave", nil), http.StatusOK)
	if st := left["conference"].(map[string]any)["status"]; st != "live" {
		t.Fatalf("call ended while %s was still in the room (status %q)", guest.Email, st)
	}

	last := guest.expect(t, guest.post("/conferences/"+confID+"/leave", nil), http.StatusOK)
	if st := last["conference"].(map[string]any)["status"]; st != "ended" {
		t.Fatalf("status after the last participant left = %q, want ended", st)
	}
}

// TestConferenceModerationIsServerSide covers the permission split. A plain
// member may walk into a call and talk, but must not be able to cancel it, end
// it for everyone, or hand out a host seat — least of all to themselves.
func TestConferenceModerationIsServerSide(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Ретро")
	member := addMember(t, owner, s.WS)

	member.expect(t, member.post("/conferences/"+confID+"/join", nil), http.StatusOK)

	if r := member.patch("/conferences/"+confID, map[string]any{"title": "Отменено"}); r.Status != http.StatusForbidden {
		t.Fatalf("member editing a conference: status %d, want 403\n%s", r.Status, r.Body)
	}
	if r := member.post("/conferences/"+confID+"/end", nil); r.Status != http.StatusForbidden {
		t.Fatalf("member ending a conference: status %d, want 403\n%s", r.Status, r.Body)
	}
	if r := member.del("/conferences/" + confID); r.Status != http.StatusForbidden {
		t.Fatalf("member deleting a conference: status %d, want 403\n%s", r.Status, r.Body)
	}
	if r := member.post("/conferences/"+confID+"/invite", map[string]any{
		"user_ids": []string{member.UserID}, "role": "host",
	}); r.Status != http.StatusForbidden {
		t.Fatalf("member promoting themselves to host: status %d, want 403\n%s", r.Status, r.Body)
	}
	// Inviting a colleague as a plain member is not moderation, and stays open.
	member.expect(t, member.post("/conferences/"+confID+"/invite",
		map[string]any{"user_ids": []string{owner.UserID}}), http.StatusOK)

	ended := owner.expect(t, owner.post("/conferences/"+confID+"/end", nil), http.StatusOK)
	if ended["status"] != "ended" {
		t.Fatalf("host ending the call: status %q", ended["status"])
	}
	// Ending an already-ended call is the desired state, not a 404 — the button
	// may well be clicked twice, or by two moderators at once.
	owner.expect(t, owner.post("/conferences/"+confID+"/end", nil), http.StatusOK)
}

// TestConferenceIsScopedToItsWorkspace is the authorization boundary: an
// outsider must not see the conference at all, and the task a conference is
// pinned to has to live in the same workspace — the link is what later surfaces
// the meeting on the task page.
func TestConferenceIsScopedToItsWorkspace(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Приватная летучка")

	outsider := signup(t)
	if r := outsider.get("/conferences/" + confID); r.Status != http.StatusForbidden {
		t.Fatalf("outsider reading a conference: status %d, want 403\n%s", r.Status, r.Body)
	}
	if r := outsider.post("/conferences/"+confID+"/join", nil); r.Status != http.StatusForbidden {
		t.Fatalf("outsider joining a conference: status %d, want 403\n%s", r.Status, r.Body)
	}
	// An invitation is also a permission to see the meeting, so it may not be
	// handed to someone outside the workspace.
	if r := owner.post("/conferences/"+confID+"/invite",
		map[string]any{"user_ids": []string{outsider.UserID}}); r.Status != http.StatusBadRequest {
		t.Fatalf("inviting a non-member: status %d, want 400\n%s", r.Status, r.Body)
	}

	foreign := mkStack(t, outsider)
	foreignTask := mkTask(t, outsider, foreign.Board, foreign.col(t, 0), "Чужая задача")
	if r := owner.patch("/conferences/"+confID, map[string]any{
		"title": "Приватная летучка", "task_id": foreignTask["id"],
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("pinning a conference to a task of another workspace: status %d, want 400\n%s", r.Status, r.Body)
	}

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Обсудить на летучке")
	owner.expect(t, owner.patch("/conferences/"+confID, map[string]any{
		"title": "Летучка по задаче", "task_id": task["id"],
	}), http.StatusOK)
	byTask := owner.get("/tasks/" + task["id"].(string) + "/conferences").listBody(t)
	if len(byTask) != 1 || byTask[0]["id"] != confID {
		t.Fatalf("task side did not list the conference: %#v", byTask)
	}
}

// TestConferenceTokenIsGatedBeforeTheSFU checks the media seam (#2871) from the
// only side that matters: nobody gets a LiveKit warrant without passing our own
// checks first. A join token is a bearer credential for the room — once minted
// it is out of our hands, so every refusal has to happen before it exists.
//
// This suite runs without LIVEKIT_* configured, which makes the not-configured
// branch the observable one. That is the point: the assertion is that an
// unconfigured install answers 503 rather than handing out a token signed with
// an empty secret, and that the membership and lifecycle refusals are reached
// *before* the SFU is ever consulted — an outsider must see 403, not 503.
func TestConferenceTokenIsGatedBeforeTheSFU(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Летучка с видео")

	// Ordering, not just the code: an outsider is turned away by the workspace
	// check, so the answer must not leak whether an SFU exists at all.
	outsider := signup(t)
	if r := outsider.post("/conferences/"+confID+"/token", nil); r.Status != http.StatusForbidden {
		t.Fatalf("outsider asking for a media token: status %d, want 403\n%s", r.Status, r.Body)
	}

	// A member of the workspace gets past authorization and lands on the real
	// answer for this install — no SFU deployed.
	if r := owner.post("/conferences/"+confID+"/token", nil); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("token without LIVEKIT_* configured: status %d, want 503\n%s", r.Status, r.Body)
	}

	// The remaining refusals — ended conference, kick cooldown — sit *behind* the
	// not-configured check in the handler, so this suite cannot tell them from
	// the 503 above and deliberately does not pretend to. They are exercised on a
	// stand with LIVEKIT_* set; asserting them here would only assert the 503
	// twice.
}

// TestConferenceInviteIsIdempotent covers the race the ON CONFLICT clause is
// there for: invitations are sent from several places at once (the dialog, a
// notification, an /invite call), and the loser must not get a 500 — nor should
// re-inviting a host quietly demote them to member.
func TestConferenceInviteIsIdempotent(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Созвон")
	guest := addMember(t, owner, s.WS)

	owner.expect(t, owner.post("/conferences/"+confID+"/invite",
		map[string]any{"user_ids": []string{guest.UserID}}), http.StatusOK)
	owner.expect(t, owner.post("/conferences/"+confID+"/invite",
		map[string]any{"user_ids": []string{guest.UserID}}), http.StatusOK)

	parts := owner.get("/conferences/" + confID + "/participants").listBody(t)
	if len(parts) != 2 {
		t.Fatalf("double invite produced %d participants, want 2: %#v", len(parts), parts)
	}
	for _, p := range parts {
		if p["user_id"] == owner.UserID && p["role"] != "host" {
			t.Fatalf("re-invite demoted the creator to %q", p["role"])
		}
	}

	// Re-inviting the creator must leave their host seat alone — the ON CONFLICT
	// clause updates the invite stamp, not the role.
	owner.expect(t, owner.post("/conferences/"+confID+"/invite",
		map[string]any{"user_ids": []string{owner.UserID}}), http.StatusOK)
	after := owner.get("/conferences/" + confID + "/participants").listBody(t)
	for _, p := range after {
		if p["user_id"] == owner.UserID && p["role"] != "host" {
			t.Fatalf("re-inviting the host as a member demoted them to %q", p["role"])
		}
	}

	list := owner.get("/workspaces/" + s.WS + "/conferences").listBody(t)
	if len(list) != 1 {
		t.Fatalf("workspace lists %d conferences, want 1", len(list))
	}
	if got := list[0]["participant_count"]; got != float64(2) {
		t.Fatalf("participant_count = %v, want 2", got)
	}
	if got := list[0]["active_count"]; got != float64(0) {
		t.Fatalf("active_count before anyone joined = %v, want 0", got)
	}

	guest.expect(t, guest.post("/conferences/"+confID+"/join", nil), http.StatusOK)
	list = owner.get("/workspaces/" + s.WS + "/conferences").listBody(t)
	if got := list[0]["active_count"]; got != float64(1) {
		t.Fatalf("active_count after one join = %v, want 1", got)
	}
}

// TestConferenceValidation pins the input the handlers refuse: an unknown status
// filter (a typo would otherwise silently return everything), a negative
// recording TTL, and an empty invite list.
func TestConferenceValidation(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Проверка")

	if r := owner.get("/workspaces/" + s.WS + "/conferences?status=running"); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown status filter: status %d, want 400\n%s", r.Status, r.Body)
	}
	if r := owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Плохой TTL", "recording_ttl_days": -1,
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("negative recording_ttl_days: status %d, want 400\n%s", r.Status, r.Body)
	}
	if r := owner.post("/conferences/"+confID+"/invite", map[string]any{"user_ids": []string{}}); r.Status != http.StatusBadRequest {
		t.Fatalf("empty invite list: status %d, want 400\n%s", r.Status, r.Body)
	}
	if r := owner.post("/conferences/"+confID+"/invite", map[string]any{
		"user_ids": []string{owner.UserID}, "role": "owner",
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown participant role: status %d, want 400\n%s", r.Status, r.Body)
	}

	owner.expect(t, owner.del("/conferences/"+confID), http.StatusNoContent)
	if r := owner.get("/conferences/" + confID); r.Status != http.StatusNotFound {
		t.Fatalf("deleted conference: status %d, want 404\n%s", r.Status, r.Body)
	}
}
