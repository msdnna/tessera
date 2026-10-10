// The v2 notifications (#3048) reach the people they are about: a closed stage
// its assignees, a mention in a document or a meeting's chat the mentioned, and
// «встреча через 5 минут» everyone invited who has not declined.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func payloadOf(n map[string]any) map[string]any {
	p, _ := n["payload"].(map[string]any)
	return p
}

func TestClosingAStageTellsItsAssignees(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	ms := actor.expect(t, actor.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Спринт 41"}), http.StatusCreated)
	msID := ms["id"].(string)
	for i, title := range []string{"Первая", "Вторая", "Третья"} {
		id := mkTask(t, actor, s.Board, s.col(t, 0), title)["id"].(string)
		actor.expect(t, actor.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": msID}), http.StatusNoContent)
		assign(t, actor, id, member.UserID)
		if i < 2 {
			actor.expect(t, actor.patch("/tasks/"+id, map[string]any{"title": title, "completed": true}), http.StatusOK)
		}
	}

	actor.expect(t, actor.patch("/milestones/"+msID, map[string]any{"title": "Спринт 41"}), http.StatusOK)
	if got := notificationsOfKind(t, member, "milestone_closed"); len(got) != 0 {
		t.Fatalf("an edit that did not close the stage notified: %v", got)
	}
	actor.expect(t, actor.patch("/milestones/"+msID, map[string]any{"title": "Спринт 41", "state": "closed"}), http.StatusOK)
	got := notificationsOfKind(t, member, "milestone_closed")
	if len(got) != 1 {
		t.Fatalf("milestone_closed = %d, want 1", len(got))
	}
	p := payloadOf(got[0])
	if p["done"] != float64(2) || p["total"] != float64(3) || p["milestone_id"] != msID || p["project_slug"] == "" {
		t.Fatalf("payload = %v", p)
	}
	if len(notificationsOfKind(t, actor, "milestone_closed")) != 0 {
		t.Fatal("the one who closed the stage was told about it")
	}
	// Saving it closed again is not a second closing.
	actor.expect(t, actor.patch("/milestones/"+msID, map[string]any{"title": "Спринт 41", "state": "closed"}), http.StatusOK)
	if n := len(notificationsOfKind(t, member, "milestone_closed")); n != 1 {
		t.Fatalf("milestone_closed after a second save = %d, want 1", n)
	}
}

func TestMentionsInADocumentAndAMeetingChat(t *testing.T) {
	t.Parallel()
	actor, member, s := aboutMeFixture(t)
	doc := actor.expect(t, actor.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "План релиза"}), http.StatusCreated)
	actor.expect(t, actor.post("/documents/"+doc["id"].(string)+"/comments", map[string]any{
		"body": "@Мария глянь раздел", "mentions": []string{member.UserID, member.UserID, actor.UserID},
	}), http.StatusCreated)

	got := notificationsOfKind(t, member, "mention")
	if len(got) != 1 {
		t.Fatalf("document mention = %d, want 1 (deduplicated)", len(got))
	}
	p := payloadOf(got[0])
	if p["event"] != "document_mention" || p["name"] != "План релиза" || p["document_slug"] != doc["slug"] {
		t.Fatalf("payload = %v", p)
	}
	if len(notificationsOfKind(t, actor, "mention")) != 0 {
		t.Fatal("the author was told about their own mention")
	}

	conf := actor.expect(t, actor.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Стендап"}), http.StatusCreated)
	actor.expect(t, actor.post("/conferences/"+conf["id"].(string)+"/messages", map[string]any{
		"body": "@Мария ты с нами?", "mentions": []string{member.UserID},
	}), http.StatusCreated)
	got = notificationsOfKind(t, member, "mention")
	if len(got) != 2 || payloadOf(got[0])["event"] != "conference_mention" || payloadOf(got[0])["conference_id"] != conf["id"] {
		t.Fatalf("chat mention = %v", got)
	}

	stranger := signup(t)
	actor.expect(t, actor.post("/conferences/"+conf["id"].(string)+"/messages", map[string]any{
		"body": "чужой", "mentions": []string{stranger.UserID},
	}), http.StatusCreated)
	if n := len(notificationsOfKind(t, stranger, "mention")); n != 0 {
		t.Fatalf("someone outside the workspace was told: %d", n)
	}
}

func TestMeetingStartingSoonRingsOncePerStart(t *testing.T) {
	owner := signup(t)
	s := mkStack(t, owner)
	guest := addMember(t, owner, s.WS)
	refuser := addMember(t, owner, s.WS)
	at := time.Now().Add(3 * time.Minute).UTC()
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Синк по синхронизации GitLab", "scheduled_at": at.Format(time.RFC3339),
	}), http.StatusCreated)
	id := conf["id"].(string)
	owner.expect(t, owner.post("/conferences/"+id+"/invite",
		map[string]any{"user_ids": []string{guest.UserID, refuser.UserID}}), http.StatusOK)
	refuser.expect(t, refuser.put("/conferences/"+id+"/rsvp", map[string]any{"response": "declined"}), http.StatusOK)

	api := newScannerAPI(t)
	api.ScanNotificationsOnce(context.Background())
	api.ScanNotificationsOnce(context.Background())

	for _, c := range []*client{owner, guest} {
		got := notificationsOfKind(t, c, "conference_soon")
		if len(got) != 1 {
			t.Fatalf("conference_soon for %s = %d, want 1", c.Email, len(got))
		}
		p := payloadOf(got[0])
		if p["conference_id"] != id || p["minutes"] == nil || p["event"] != "conference_starting" {
			t.Fatalf("payload = %v", p)
		}
	}
	if n := len(notificationsOfKind(t, refuser, "conference_soon")); n != 0 {
		t.Fatalf("someone who declined was told: %d", n)
	}

	// Moved to a new start inside the lead: it rings again.
	owner.expect(t, owner.patch("/conferences/"+id, map[string]any{
		"title": "Синк по синхронизации GitLab", "scheduled_at": at.Add(time.Minute).Format(time.RFC3339),
	}), http.StatusOK)
	api.ScanNotificationsOnce(context.Background())
	if n := len(notificationsOfKind(t, guest, "conference_soon")); n != 2 {
		t.Fatalf("after moving the meeting conference_soon = %d, want 2", n)
	}
}
