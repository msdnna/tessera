package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHealthAndVersion(t *testing.T) {
	t.Parallel()
	r := doReq(t, "", http.MethodGet, "/health", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("health: %d", r.Status)
	}
	m := doReq(t, "", http.MethodGet, "/version", nil).mapBody(t)
	if m["api"] == nil {
		t.Fatalf("version body: %v", m)
	}
}

// The /ws endpoint upgrades an authenticated request and registers it with the
// hub; a broadcast triggered by an API write must reach the socket. Auth and
// scoping have their own suite in ws_auth_test.go.
func TestWebSocketConnect(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/api/ws"
	conn, res, err := websocket.DefaultDialer.Dial(wsURL,
		http.Header{"Authorization": []string{"Bearer " + c.token}})
	if err != nil {
		t.Fatalf("dial: %v (res=%v)", err, res)
	}
	defer conn.Close()

	// Generate some broadcast traffic in the socket's own workspace.
	mkTask(t, c, s.Board, s.col(t, 0), "WS ping")

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, rerr := conn.ReadMessage(); rerr != nil {
		t.Fatalf("no broadcast reached the socket: %v", rerr)
	}
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}
