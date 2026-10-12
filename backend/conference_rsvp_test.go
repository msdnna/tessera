// «Принять / Отклонить» an invitation to a meeting (#3051): the answer is kept
// on the participant's seat, read back by the meetings list and the feed, and
// closes the invitation in the reader's feed.
package main

import (
	"net/http"
	"testing"
)

func myRsvp(t *testing.T, c *client, wsID, confID string) any {
	t.Helper()
	for _, row := range c.get("/workspaces/" + wsID + "/conferences").listBody(t) {
		if row["id"] == confID {
			return row["my_rsvp"]
		}
	}
	t.Fatalf("conference %s is not in the list", confID)
	return nil
}

func inviteRow(t *testing.T, c *client) map[string]any {
	t.Helper()
	for _, row := range c.get("/notifications").listBody(t) {
		if row["kind"] == "conference_invite" {
			return row
		}
	}
	t.Fatal("no invitation in the feed")
	return nil
}

func TestConferenceRsvp(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Синк по синхронизации GitLab")
	guest := addMember(t, owner, s.WS)
	outsider := addMember(t, owner, s.WS)

	if got := myRsvp(t, owner, s.WS, confID); got != "accepted" {
		t.Fatalf("the host's answer = %v, want accepted — creating the meeting is a yes", got)
	}
	if got := myRsvp(t, guest, s.WS, confID); got != nil {
		t.Fatalf("an uninvited member's answer = %v, want null", got)
	}

	owner.expect(t, owner.post("/conferences/"+confID+"/invite",
		map[string]any{"user_ids": []string{guest.UserID}}), http.StatusOK)
	if got := myRsvp(t, guest, s.WS, confID); got != "pending" {
		t.Fatalf("an invited member's answer = %v, want pending", got)
	}
	if n := inviteRow(t, guest); n["rsvp"] != "pending" || n["read_at"] != nil {
		t.Fatalf("the invitation in the feed = rsvp %v read %v, want pending and unread", n["rsvp"], n["read_at"])
	}

	t.Run("an answer is kept and closes the invitation", func(t *testing.T) {
		part := guest.expect(t, guest.put("/conferences/"+confID+"/rsvp",
			map[string]any{"response": "accepted"}), http.StatusOK)
		if part["rsvp"] != "accepted" || part["rsvp_at"] == nil {
			t.Fatalf("answer = %v", part)
		}
		if got := myRsvp(t, guest, s.WS, confID); got != "accepted" {
			t.Fatalf("list says %v after accepting", got)
		}
		n := inviteRow(t, guest)
		if n["rsvp"] != "accepted" || n["read_at"] == nil {
			t.Fatalf("the invitation in the feed = rsvp %v read %v, want accepted and read", n["rsvp"], n["read_at"])
		}
		page := getNotifPage(t, guest, "")
		if len(page.Items) == 0 || page.Items[0]["rsvp"] != "accepted" {
			t.Fatalf("the page of all notifications = %v", page.Items)
		}
	})

	t.Run("the answer can be changed, and the host sees it", func(t *testing.T) {
		guest.expect(t, guest.put("/conferences/"+confID+"/rsvp",
			map[string]any{"response": "declined"}), http.StatusOK)
		detail := owner.expect(t, owner.get("/conferences/"+confID), http.StatusOK)
		parts, _ := detail["participants"].([]any)
		found := false
		for _, p := range parts {
			row := p.(map[string]any)
			if row["user_id"] == guest.UserID {
				found = row["rsvp"] == "declined"
			}
		}
		if !found {
			t.Fatalf("the host does not see the guest's «no»: %v", parts)
		}
	})

	t.Run("re-inviting someone who declined asks again", func(t *testing.T) {
		before := countNotifications(t, guest, "conference_invite")
		owner.expect(t, owner.post("/conferences/"+confID+"/invite",
			map[string]any{"user_ids": []string{guest.UserID}}), http.StatusOK)
		if got := myRsvp(t, guest, s.WS, confID); got != "pending" {
			t.Fatalf("answer after a re-invitation = %v, want pending", got)
		}
		if got := countNotifications(t, guest, "conference_invite"); got != before+1 {
			t.Fatalf("re-inviting someone who declined produced %d new notifications, want 1", got-before)
		}
	})

	t.Run("walking in is a yes", func(t *testing.T) {
		guest.expect(t, guest.post("/conferences/"+confID+"/join", nil), http.StatusOK)
		if got := myRsvp(t, guest, s.WS, confID); got != "accepted" {
			t.Fatalf("answer after joining = %v, want accepted", got)
		}
		guest.expect(t, guest.post("/conferences/"+confID+"/leave", nil), http.StatusOK)
	})

	t.Run("only an invitation is answered", func(t *testing.T) {
		if r := outsider.put("/conferences/"+confID+"/rsvp", map[string]any{"response": "accepted"}); r.Status != http.StatusNotFound {
			t.Fatalf("an uninvited member answered: %d", r.Status)
		}
		if r := guest.put("/conferences/"+confID+"/rsvp", map[string]any{"response": "maybe"}); r.Status != http.StatusBadRequest {
			t.Fatalf("«maybe» = %d, want 400", r.Status)
		}
		stranger := signup(t)
		if r := stranger.put("/conferences/"+confID+"/rsvp", map[string]any{"response": "accepted"}); r.Status != http.StatusForbidden && r.Status != http.StatusNotFound {
			t.Fatalf("someone outside the workspace answered: %d", r.Status)
		}
	})
}
