package realtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// The hub tests exercise the fan-out decision with nil-conn clients. These drive
// the wire side of a real Client — the read/write pumps over an actual socket —
// which nothing else covers at unit level (only indirectly via the ws_auth
// integration test). No time.Sleep: every wait is on a socket read or on
// ClientCount via waitFor (defined in hub_test.go), both deterministic.

// dialClient stands up a hub, upgrades one socket to a scoped Client and returns
// the client-side dialer plus the workspace scope its events must carry. The
// server handler hands the *Client back so a test can inspect it (e.g. needResync).
func dialClient(t *testing.T) (h *Hub, c *Client, dialer *websocket.Conn, scope string) {
	t.Helper()
	h = NewHub()
	go h.Run()

	scope = uuid.New().String()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	clientCh := make(chan *Client, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		cl := NewClient(h, conn, uuid.New(), []string{scope})
		cl.Start() // returns only once Run has registered the client
		clientCh <- cl
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = dialer.SetReadDeadline(time.Now().Add(2 * time.Second))

	c = <-clientCh // registration is complete: Start blocks on h.register
	t.Cleanup(func() {
		_ = dialer.Close()
		srv.Close()
		h.Close()
	})
	return h, c, dialer, scope
}

// A broadcast for the client's workspace must reach the wire as the JSON Event
// the browser/Android clients parse — this is the writePump WriteJSON path.
func TestClientDeliversEventAsJSON(t *testing.T) {
	h, _, dialer, scope := dialClient(t)

	actor := uuid.New().String()
	h.Broadcast(Event{Scope: scope, Type: "task.updated", Actor: actor})

	var got Event
	if err := dialer.ReadJSON(&got); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if got.Scope != scope || got.Type != "task.updated" || got.Actor != actor {
		t.Fatalf("wire event = %+v, want scope=%s type=task.updated actor=%s", got, scope, actor)
	}
}

// When the peer drops the socket, the read pump must notice and unregister the
// client — otherwise a flapping network leaks one client per reconnect.
func TestClientDisconnectUnregisters(t *testing.T) {
	h, _, dialer, _ := dialClient(t)
	waitFor(t, "registration", func() bool { return h.ClientCount() == 1 })

	_ = dialer.Close()

	waitFor(t, "unregister after disconnect", func() bool { return h.ClientCount() == 0 })
}

// Hub shutdown closes the client's send channel; the write pump must turn that
// into a normal close frame so the browser reconnects instead of reporting a
// broken socket (abnormal 1006).
func TestClientHubCloseSendsCloseFrame(t *testing.T) {
	h, _, dialer, _ := dialClient(t)

	h.Close()

	_, _, err := dialer.ReadMessage()
	if err == nil {
		t.Fatal("expected a close frame, got a message")
	}
	if !websocket.IsCloseError(err, websocket.CloseNoStatusReceived) {
		t.Fatalf("got %v, want a normal close (not an abnormal 1006 broken socket)", err)
	}
}

// SetReadLimit(512) is the only guard against a fat frame on the read-only
// socket: a client that sends one over the limit is disconnected.
func TestClientReadLimitDisconnects(t *testing.T) {
	h, _, dialer, _ := dialClient(t)
	waitFor(t, "registration", func() bool { return h.ClientCount() == 1 })

	oversized := strings.Repeat("x", 513)
	if err := dialer.WriteMessage(websocket.TextMessage, []byte(oversized)); err != nil {
		t.Fatalf("write oversized frame: %v", err)
	}

	waitFor(t, "disconnect on oversized frame", func() bool { return h.ClientCount() == 0 })
}

// A client the hub flagged for resync (its buffer had overflowed) gets a single
// resync marker piggy-backed on the next successful write, so a recovered-but-
// still-connected client reloads without waiting for a reconnect.
func TestClientFlushesResyncMarker(t *testing.T) {
	h, c, dialer, scope := dialClient(t)

	c.needResync.Store(true)
	h.Broadcast(Event{Scope: scope, Type: "task.updated"})

	var first, second Event
	if err := dialer.ReadJSON(&first); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if first.Type != "task.updated" {
		t.Fatalf("first frame = %q, want task.updated", first.Type)
	}
	if err := dialer.ReadJSON(&second); err != nil {
		t.Fatalf("read resync: %v", err)
	}
	if second.Type != EventResync {
		t.Fatalf("second frame = %q, want the resync marker", second.Type)
	}
	if c.needResync.Load() {
		t.Fatal("resync flag not cleared after a successful write")
	}
}
