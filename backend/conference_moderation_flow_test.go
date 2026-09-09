// Moderation that outlives the room (#2864, subtask #2872).
//
// internal/confroom already proves a host may kick and a member may not, and
// internal/livekit proves the SFU calls are shaped right. What only this level
// can show is the part that made those two worth wiring together: a kick and a
// force-mute reach the database, so the participant row stops claiming the
// person is in the call and the mute is still in force on a fresh connection.
//
// The SFU half is not exercised here — the harness runs without LIVEKIT_*, so
// the client reports itself disabled and the enforcer skips it. That is the
// same path a self-hosted install without conferences takes, and showing the
// persistence still happens on it is worth having on its own.
package main

import (
	"net/http"
	"testing"
	"time"
)

// awaitParticipant polls the roster until a participant satisfies ok, returning
// the last row it saw either way.
//
// Polling rather than reading a response body: enforcement is deliberately
// asynchronous — the host's socket must not block on Postgres and the SFU while
// a kick lands — so there is no reply that carries the result.
func awaitParticipant(t *testing.T, c *client, confID, userID string, ok func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		for _, p := range c.get("/conferences/" + confID + "/participants").listBody(t) {
			if p["user_id"] != userID {
				continue
			}
			last = p
			if ok(p) {
				return p
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}

// TestConferenceKickIsPersisted: a kicked participant's row has to stop saying
// they are in the call. The list screen counts exactly those rows, and so does
// the check that ends an empty conference — leaving the row behind would keep
// the meeting advertising someone who was thrown out of it.
func TestConferenceKickIsPersisted(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с исключением")
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

	before := awaitParticipant(t, owner, confID, guestID, func(p map[string]any) bool {
		return p["joined_at"] != nil && p["left_at"] == nil
	})
	if before == nil || before["left_at"] != nil {
		t.Fatalf("the member was not recorded as present before the kick: %#v", before)
	}

	if err := host.WriteJSON(map[string]any{"type": "kick", "user_id": guestID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	after := awaitParticipant(t, owner, confID, guestID, func(p map[string]any) bool {
		return p["left_at"] != nil
	})
	if after == nil || after["left_at"] == nil {
		t.Fatalf("the kick never reached the participant row: %#v", after)
	}
}

// TestConferenceForceMuteIsPersistedAndHeld is why the flag is a column and not
// only a field in the room: the room's memory dies with its last participant,
// and a mute that died with it would be undone by waiting for the call to empty
// and rejoining first.
//
// Two things are asserted, and they are not the same thing. That the flag
// reaches conference_participants is checked through the REST roster. That a
// reconnecting client is *held* to it — the room refuses their own report of a
// live microphone rather than repainting the badge afterwards — is checked over
// a fresh socket. The seeding path in between, room-from-row, is pinned
// deterministically by TestForceMuteSurvivesAnEmptyRoom in internal/confroom;
// from out here the room may or may not have been reclaimed by the time we
// reconnect, and either way the behaviour below must hold.
func TestConferenceForceMuteIsPersistedAndHeld(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с принудительным мьютом")
	mate := addMember(t, owner, s.WS)
	mate.expect(t, mate.post("/conferences/"+confID+"/join", nil), http.StatusOK)

	host, _ := dialConfWS(t, owner.token, confID)
	if host == nil {
		t.Fatal("host handshake failed")
	}
	guest, _ := dialConfWS(t, mate.token, confID)
	if guest == nil {
		host.Close()
		t.Fatal("member handshake failed")
	}
	welcome := awaitConfFrame(t, guest, "welcome", 5*time.Second)
	if welcome == nil {
		host.Close()
		guest.Close()
		t.Fatal("no welcome for the member")
	}
	guestID, _ := welcome["user_id"].(string)

	if err := host.WriteJSON(map[string]any{"type": "mute", "user_id": guestID, "muted": true}); err != nil {
		host.Close()
		guest.Close()
		t.Fatalf("mute: %v", err)
	}
	seen := awaitState(t, host, 5*time.Second, func(m map[string]any) bool {
		p, ok := people(m)[guestID]
		return ok && p["force_muted"] == true
	})
	stored := awaitParticipant(t, owner, confID, guestID, func(p map[string]any) bool {
		return p["force_muted"] == true
	})
	host.Close()
	guest.Close()
	if seen == nil {
		t.Fatal("force-mute never reached the room snapshot")
	}
	if stored == nil || stored["force_muted"] != true {
		t.Fatalf("force-mute was not persisted: %#v", stored)
	}

	back, _ := dialConfWS(t, mate.token, confID)
	if back == nil {
		t.Fatal("the muted member could not reconnect")
	}
	defer back.Close()
	if err := back.WriteJSON(map[string]any{"type": "media", "mic": true, "cam": false}); err != nil {
		t.Fatalf("media: %v", err)
	}
	if awaitState(t, back, 5*time.Second, func(m map[string]any) bool {
		p, ok := people(m)[guestID]
		return ok && p["force_muted"] == true && p["mic"] == false
	}) == nil {
		t.Fatal("a reconnected member talked their way out of the force-mute")
	}
}
