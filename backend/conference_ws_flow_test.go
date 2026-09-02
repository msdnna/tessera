// The conference room socket over the real thing (#2869).
//
// internal/confroom already tests the arbitration in isolation; what is only
// reachable from here is the wiring around it — that the handle authenticates
// and checks membership in the conference's workspace *before* upgrading, that
// two browsers in one call actually see each other, that the screen-share queue
// survives the round trip, and that ending the call empties the room instead of
// leaving people talking into a meeting that is over.
package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// confWSURL is the conference socket on the harness server.
func confWSURL(confID string) string {
	return "ws" + strings.TrimPrefix(testServer.URL, "http") + "/api/conferences/" + confID + "/ws"
}

// dialConfWS opens the conference socket, returning the handshake status
// alongside the connection so refusals can be asserted as precisely as
// successes.
func dialConfWS(t *testing.T, token, confID string) (*websocket.Conn, int) {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := http.Header{}
	if token != "" {
		hdr.Set("Authorization", "Bearer "+token)
	}
	conn, res, err := d.Dial(confWSURL(confID), hdr)
	status := 0
	if res != nil {
		status = res.StatusCode
		defer res.Body.Close()
	}
	if err != nil && conn != nil {
		conn.Close()
		conn = nil
	}
	return conn, status
}

// awaitConfFrame reads until a frame of the wanted type shows up or the budget
// runs out. Snapshots and answers race on the wire — the room broadcasts state
// to everyone while answering the requester — so a test that insisted on an
// exact frame order would be flaky by construction.
func awaitConfFrame(t *testing.T, conn *websocket.Conn, want string, budget time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			return nil
		}
		if msg["type"] == want {
			return msg
		}
	}
	return nil
}

// awaitState reads until a snapshot satisfying ok arrives. The room broadcasts
// on every change, so "the state I want" is a later frame than "a state".
func awaitState(t *testing.T, conn *websocket.Conn, budget time.Duration, ok func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		msg := awaitConfFrame(t, conn, "state", time.Until(deadline))
		if msg == nil {
			return nil
		}
		if ok(msg) {
			return msg
		}
	}
	return nil
}

// people maps user id → participant view from a snapshot.
func people(msg map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	list, _ := msg["participants"].([]any)
	for _, raw := range list {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := p["user_id"].(string)
		out[id] = p
	}
	return out
}

// TestConferenceWSRequiresMembership is the one that matters for security: a
// conference is reachable by id alone, so the membership check is all that
// stands between a valid token and another workspace's meeting.
func TestConferenceWSRequiresMembership(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Закрытая летучка")
	outsider := signup(t)

	if conn, status := dialConfWS(t, "", confID); status != http.StatusUnauthorized {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("anonymous handshake = %d, want 401", status)
	}
	if conn, status := dialConfWS(t, outsider.token, confID); status != http.StatusForbidden {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("outsider handshake = %d, want 403", status)
	}
	if conn, status := dialConfWS(t, owner.token, "not-a-uuid"); status != http.StatusBadRequest {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("malformed id handshake = %d, want 400", status)
	}
}

// TestConferenceWSPresenceAndStage is the feature itself: two members join one
// call, see each other with the right roles, and the second one to reach for
// the screen is queued behind the first rather than both sharing at once.
func TestConferenceWSPresenceAndStage(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с демонстрацией")
	mate := addMember(t, owner, s.WS)

	host, status := dialConfWS(t, owner.token, confID)
	if host == nil {
		t.Fatalf("host handshake = %d", status)
	}
	defer host.Close()
	welcome := awaitConfFrame(t, host, "welcome", 5*time.Second)
	if welcome == nil || welcome["role"] != "host" {
		t.Fatalf("the conference creator is not welcomed as host: %#v", welcome)
	}

	guest, status := dialConfWS(t, mate.token, confID)
	if guest == nil {
		t.Fatalf("member handshake = %d", status)
	}
	defer guest.Close()
	if w := awaitConfFrame(t, guest, "welcome", 5*time.Second); w == nil || w["role"] != "member" {
		t.Fatalf("a plain member was welcomed as %#v", w)
	}

	both := awaitState(t, host, 5*time.Second, func(m map[string]any) bool {
		return len(people(m)) == 2
	})
	if both == nil {
		t.Fatal("the host never saw the second participant")
	}

	// The member takes the free stage; the host's request is granted too, but
	// only because a host preempts a member — so ask in the other order to test
	// the queue: host first, member queued behind them.
	if err := host.WriteJSON(map[string]any{"type": "screen.request"}); err != nil {
		t.Fatalf("screen.request: %v", err)
	}
	onStage := awaitState(t, guest, 5*time.Second, func(m map[string]any) bool {
		stage, _ := m["stage"].(map[string]any)
		return stage != nil
	})
	if onStage == nil {
		t.Fatal("the stage never went to the host")
	}
	if got := onStage["stage"].(map[string]any)["user_id"]; got != welcome["user_id"] {
		t.Fatalf("stage held by %v, want the host %v", got, welcome["user_id"])
	}

	if err := guest.WriteJSON(map[string]any{"type": "screen.request"}); err != nil {
		t.Fatalf("screen.request: %v", err)
	}
	queued := awaitState(t, guest, 5*time.Second, func(m map[string]any) bool {
		q, _ := m["queue"].([]any)
		return len(q) == 1
	})
	if queued == nil {
		t.Fatal("a request for a busy stage was neither granted nor queued")
	}
	if stage, _ := queued["stage"].(map[string]any); stage == nil || stage["user_id"] != welcome["user_id"] {
		t.Fatalf("the queued request took the stage from the host: %#v", queued["stage"])
	}

	// Releasing hands the stage straight to whoever waited, without a round of
	// "the stage is free" that two clients could race for.
	if err := host.WriteJSON(map[string]any{"type": "screen.release"}); err != nil {
		t.Fatalf("screen.release: %v", err)
	}
	promoted := awaitState(t, guest, 5*time.Second, func(m map[string]any) bool {
		stage, _ := m["stage"].(map[string]any)
		return stage != nil && stage["user_id"] != welcome["user_id"]
	})
	if promoted == nil {
		t.Fatal("the queued participant was not promoted on release")
	}
	if q, _ := promoted["queue"].([]any); len(q) != 0 {
		t.Fatalf("the promoted participant is still in the queue: %#v", q)
	}
}

// TestConferenceWSModeration walks a kick over the real socket: the victim is
// told why before the close, and cannot simply dial back in.
func TestConferenceWSModeration(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с модерацией")
	mate := addMember(t, owner, s.WS)

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

	guestWelcome := awaitConfFrame(t, guest, "welcome", 5*time.Second)
	if guestWelcome == nil {
		t.Fatal("no welcome for the member")
	}
	guestID, _ := guestWelcome["user_id"].(string)

	// A member's kick is refused by the server, not merely hidden in the UI.
	if err := guest.WriteJSON(map[string]any{"type": "kick", "user_id": guestID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if denied := awaitConfFrame(t, guest, "denied", 5*time.Second); denied == nil || denied["action"] != "kick" {
		t.Fatalf("a member's kick was not refused: %#v", denied)
	}

	if err := host.WriteJSON(map[string]any{"type": "mute", "user_id": guestID, "muted": true}); err != nil {
		t.Fatalf("mute: %v", err)
	}
	muted := awaitState(t, host, 5*time.Second, func(m map[string]any) bool {
		p, ok := people(m)[guestID]
		return ok && p["force_muted"] == true
	})
	if muted == nil {
		t.Fatal("force-mute never reached the room snapshot")
	}

	if err := host.WriteJSON(map[string]any{"type": "kick", "user_id": guestID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if ended := awaitConfFrame(t, guest, "ended", 5*time.Second); ended == nil || ended["reason"] != "kicked" {
		t.Fatalf("the kicked member was not told why: %#v", ended)
	}
	// The cooldown is in memory, so a reconnect is refused at the handshake
	// rather than by a socket that closes itself a moment later.
	if conn, status := dialConfWS(t, mate.token, confID); status != http.StatusForbidden {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("a kicked member reconnected: status %d, want 403", status)
	}
}

// TestConferenceWSEndEmptiesTheRoom: a call that ended must not leave people
// connected to it. They are told once, over the socket they are already
// holding, instead of finding out when their next request 404s.
func TestConferenceWSEndEmptiesTheRoom(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Летучка, которую закроют")

	conn, status := dialConfWS(t, owner.token, confID)
	if conn == nil {
		t.Fatalf("handshake = %d", status)
	}
	defer conn.Close()
	if awaitConfFrame(t, conn, "welcome", 5*time.Second) == nil {
		t.Fatal("no welcome")
	}

	owner.expect(t, owner.post("/conferences/"+confID+"/end", nil), http.StatusOK)
	if ended := awaitConfFrame(t, conn, "ended", 5*time.Second); ended == nil || ended["reason"] != "ended" {
		t.Fatalf("the room was not told the call is over: %#v", ended)
	}
	// And a stale tab cannot dial back into it in the morning.
	if again, status := dialConfWS(t, owner.token, confID); status != http.StatusConflict {
		if again != nil {
			again.Close()
		}
		t.Fatalf("re-dialling an ended conference: status %d, want 409", status)
	}
}
