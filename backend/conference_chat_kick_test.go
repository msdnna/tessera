// A kick has to close the chat too (#2932).
//
// internal/confroom proves the cooldown exists and conference_moderation_flow
// proves a kick reaches the participant row. What neither can show is the hole
// between them: the cooldown was consulted by the presence socket and the media
// token, and by nothing else. The chat is a plain authenticated REST route, so
// the credential the kicked person is still holding kept working on it — and
// posting nudges every socket in the call, which is exactly the audience the
// kick was meant to take away.
package main

import (
	"net/http"
	"testing"
	"time"
)

// TestKickedMemberCannotPostToConferenceChat: after a host kicks someone, their
// messages must stop landing in the call's rail.
//
// Three assertions, in order, because only the sequence is evidence. The member
// posts successfully first — otherwise a 403 at the end could just as well mean
// they were never able to post. Then the kick is awaited through the roster
// (enforcement is asynchronous; the row going left_at is the observable edge).
// Then the same request is refused. The host posting afterwards is the negative
// control on the gate itself: the refusal has to be about this one person, not
// about the conference having become read-only.
func TestKickedMemberCannotPostToConferenceChat(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с выдворением из чата")
	mate := addMember(t, owner, s.WS)
	mate.expect(t, mate.post("/conferences/"+confID+"/join", nil), http.StatusOK)

	host, _ := dialConfWS(t, owner.token, confID)
	if host == nil {
		t.Fatal("host handshake failed")
	}
	defer host.Close()
	guest, _ := dialConfWS(t, mate.token, confID)
	if guest == nil {
		t.Fatal("member handshake failed")
	}
	defer guest.Close()

	welcome := awaitConfFrame(t, guest, "welcome", 5*time.Second)
	if welcome == nil {
		t.Fatal("no welcome for the member")
	}
	guestID, _ := welcome["user_id"].(string)

	mate.expect(t, mate.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "до кика"}), http.StatusCreated)

	if err := host.WriteJSON(map[string]any{"type": "kick", "user_id": guestID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if after := awaitParticipant(t, owner, confID, guestID, func(p map[string]any) bool {
		return p["left_at"] != nil
	}); after == nil || after["left_at"] == nil {
		t.Fatalf("the kick never landed: %#v", after)
	}

	// The room records the cooldown while it handles the kick frame, before the
	// asynchronous half that stamps the row — so by the time left_at is visible
	// the gate is already in force and this needs no second wait.
	mate.expect(t, mate.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "после кика"}), http.StatusForbidden)

	owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "ведущий пишет дальше"}), http.StatusCreated)
}
