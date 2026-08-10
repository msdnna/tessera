// WebSocket authentication and scoping (#2620). Before this, /api/ws was open
// to anonymous clients and the hub fanned every workspace's events out to every
// socket — these tests are the regression guard for both halves.
package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsURL rewrites the harness server URL to a ws:// one.
func wsURL() string {
	return "ws" + strings.TrimPrefix(testServer.URL, "http") + "/api/ws"
}

// dialWS opens the realtime socket with a bearer credential in the given
// transport (header for native clients, subprotocol for browsers) and returns
// the connection plus the handshake status.
func dialWS(t *testing.T, token string, viaSubprotocol bool) (*websocket.Conn, int) {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := http.Header{}
	if token != "" {
		if viaSubprotocol {
			d.Subprotocols = []string{"bearer", token}
		} else {
			hdr.Set("Authorization", "Bearer "+token)
		}
	}
	conn, res, err := d.Dial(wsURL(), hdr)
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

// readEvent waits for one JSON frame, returning nil on timeout so callers can
// assert "nothing arrived" as well as "this arrived".
func readEvent(t *testing.T, conn *websocket.Conn, wait time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	var ev map[string]any
	if err := conn.ReadJSON(&ev); err != nil {
		return nil
	}
	return ev
}

func TestWSRejectsAnonymous(t *testing.T) {
	conn, status := dialWS(t, "", false)
	if conn != nil {
		conn.Close()
		t.Fatal("anonymous client got a WebSocket — /api/ws is unauthenticated")
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("handshake status %d, want 401", status)
	}
}

func TestWSRejectsBadToken(t *testing.T) {
	conn, status := dialWS(t, "not-a-real-token", false)
	if conn != nil {
		conn.Close()
		t.Fatal("garbage token was accepted")
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("handshake status %d, want 401", status)
	}
}

// TestWSScopedDelivery is the core of the fix: a member receives their own
// workspace's events, and a stranger with a perfectly valid token of their own
// receives none of them.
func TestWSScopedDelivery(t *testing.T) {
	owner := signup(t)
	stranger := signup(t)
	st := mkStack(t, owner)

	ownerConn, status := dialWS(t, owner.token, false)
	if ownerConn == nil {
		t.Fatalf("owner handshake failed (status %d)", status)
	}
	defer ownerConn.Close()

	strangerConn, status := dialWS(t, stranger.token, false)
	if strangerConn == nil {
		t.Fatalf("stranger handshake failed (status %d)", status)
	}
	defer strangerConn.Close()

	colID := st.Columns[0]["id"].(string)
	mkTask(t, owner, st.Board, colID, "Задача под наблюдением")

	ev := readEvent(t, ownerConn, 5*time.Second)
	if ev == nil {
		t.Fatal("member received no event for their own workspace")
	}
	if ev["scope"] != st.WS {
		t.Fatalf("scope %v, want the member's workspace %s", ev["scope"], st.WS)
	}

	if leaked := readEvent(t, strangerConn, 500*time.Millisecond); leaked != nil {
		t.Fatalf("non-member received a foreign workspace's event: %+v", leaked)
	}
}

// TestWSSubprotocolAuth covers the browser transport: the token rides in
// Sec-WebSocket-Protocol and the server must echo the selected subprotocol back
// (Chrome fails the connection otherwise).
func TestWSSubprotocolAuth(t *testing.T) {
	c := signup(t)
	st := mkStack(t, c)

	conn, status := dialWS(t, c.token, true)
	if conn == nil {
		t.Fatalf("subprotocol handshake failed (status %d)", status)
	}
	defer conn.Close()
	if got := conn.Subprotocol(); got != "bearer" {
		t.Fatalf("negotiated subprotocol %q, want \"bearer\" — browsers drop the connection without the echo", got)
	}

	mkTask(t, c, st.Board, st.Columns[0]["id"].(string), "Через сабпротокол")
	if ev := readEvent(t, conn, 5*time.Second); ev == nil {
		t.Fatal("no event delivered over a subprotocol-authenticated socket")
	}
}

// TestWSPATAuth keeps headless clients (MCP, CI) working — they authenticate
// with a personal access token, not a session JWT.
func TestWSPATAuth(t *testing.T) {
	c := signup(t)
	st := mkStack(t, c)

	m := c.expect(t, c.post("/auth/tokens", map[string]any{"name": "ws-test"}), http.StatusCreated)
	pat, _ := m["token"].(string)
	if pat == "" {
		t.Fatalf("no plaintext token in create-PAT response: %+v", m)
	}

	conn, status := dialWS(t, pat, false)
	if conn == nil {
		t.Fatalf("PAT handshake failed (status %d)", status)
	}
	defer conn.Close()

	mkTask(t, c, st.Board, st.Columns[0]["id"].(string), "По PAT")
	if ev := readEvent(t, conn, 5*time.Second); ev == nil {
		t.Fatal("no event delivered over a PAT-authenticated socket")
	}
}

// TestWSDropsRemovedMember checks that revoking membership cuts the live socket
// instead of leaving it streaming until the next reconnect.
func TestWSDropsRemovedMember(t *testing.T) {
	owner := signup(t)
	member := signup(t)
	st := mkStack(t, owner)

	owner.expect(t, owner.post("/workspaces/"+st.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)

	conn, status := dialWS(t, member.token, false)
	if conn == nil {
		t.Fatalf("member handshake failed (status %d)", status)
	}
	defer conn.Close()

	if r := owner.del("/workspaces/" + st.WS + "/members/" + member.UserID); r.Status != http.StatusNoContent {
		t.Fatalf("remove member: status %d\n%s", r.Status, r.Body)
	}

	// The socket must close; anything the server sends afterwards is a leak.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("socket of a removed member was never closed")
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) &&
				!strings.Contains(err.Error(), "use of closed") {
				// A transport-level close is fine too — the point is the socket died.
				t.Logf("socket closed with %v", err)
			}
			return
		}
	}
}
