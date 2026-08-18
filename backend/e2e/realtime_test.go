//go:build e2e

// Realtime over a real WebSocket. The in-process tests upgrade against
// httptest, which never leaves the process; this dials the running binary, so
// the handshake, the subprotocol echo and the fan-out all cross an actual
// socket — the layer where a reverse proxy or a header change breaks things.
package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialWS opens the realtime socket against the running server, carrying the
// bearer credential the way the given client would: a header (Android, desktop,
// CLI) or the second subprotocol entry (browsers, which cannot set headers).
func dialWS(t *testing.T, s *server, token string, viaSubprotocol bool) *websocket.Conn {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	hdr := http.Header{}
	if viaSubprotocol {
		d.Subprotocols = []string{"bearer", token}
	} else {
		hdr.Set("Authorization", "Bearer "+token)
	}
	conn, res, err := d.Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/api/ws", hdr)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial /api/ws (subprotocol=%v): %v (handshake status %d)", viaSubprotocol, err, status)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// awaitEvent reads frames until one of type want arrives, or the wait expires.
// Skipping other types matters: a board mutation also emits column/journal
// traffic, and a strict "next frame" assertion would be flaky for no reason.
func awaitEvent(t *testing.T, conn *websocket.Conn, want string, wait time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	for {
		var ev map[string]any
		if err := conn.ReadJSON(&ev); err != nil {
			t.Fatalf("waiting for %q: %v", want, err)
		}
		if ev["type"] == want {
			return ev
		}
	}
}

func TestRealtimeFanOutOverRealSocket(t *testing.T) {
	s := startServer(t, nil)
	owner := s.register(t, "rt")
	st := s.mkStack(t, owner)
	colID := st.Columns[0]["id"].(string)

	native := dialWS(t, s, owner.Access, false)
	browser := dialWS(t, s, owner.Access, true)

	if got := browser.Subprotocol(); got != "bearer" {
		t.Errorf("negotiated subprotocol = %q, want \"bearer\" — Chrome fails the handshake without the echo", got)
	}

	created := expect(t, s.post(t, "/boards/"+st.Board+"/tasks", owner.Access,
		map[string]any{"title": "e2e realtime", "column_id": colID}), http.StatusCreated)
	taskID := created["id"].(string)

	for name, conn := range map[string]*websocket.Conn{"header client": native, "subprotocol client": browser} {
		ev := awaitEvent(t, conn, "task.created", 10*time.Second)
		if ev["scope"] != st.WS {
			t.Errorf("%s: event scope %v, want the workspace %s", name, ev["scope"], st.WS)
		}
		if ev["actor"] != owner.UserID {
			t.Errorf("%s: event actor %v, want %s", name, ev["actor"], owner.UserID)
		}
		var data map[string]any
		raw, _ := json.Marshal(ev["data"])
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatalf("%s: decode event data: %v", name, err)
		}
		if data["id"] != taskID {
			t.Errorf("%s: event carried task %v, want %s", name, data["id"], taskID)
		}
	}
}

// TestWSClosesCleanlyOnShutdown covers the promise drain() makes to browsers:
// a redeploy hands every socket a normal close frame, so clients reconnect to
// the replacement instead of reporting a broken connection. Only observable
// against a real process — the in-process tests never shut a hub down under a
// live socket.
func TestWSClosesCleanlyOnShutdown(t *testing.T) {
	s := startServer(t, nil)
	owner := s.register(t, "ws-close")
	conn := dialWS(t, s, owner.Access, false)

	s.stop() // SIGTERM + wait, exactly what a deploy does

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("socket stayed open after the server shut down")
	}
	// The hub writes an empty close frame, which gorilla surfaces as 1005 "no
	// status received" — still a close *handshake*. What must not happen is 1006
	// (abnormal closure): that is a dropped TCP connection, and it is what the
	// browser reports as a network error instead of reconnecting quietly.
	if !websocket.IsCloseError(err,
		websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
		t.Errorf("client saw %v, want a close frame rather than a dropped connection", err)
	}
}
