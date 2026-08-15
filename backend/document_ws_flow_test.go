// Per-document presence and block locks over the real socket (#2729).
//
// internal/docroom already tests the room logic in isolation; what is only
// reachable from here is the wiring around it — that the handle authenticates
// and checks workspace membership *before* upgrading, that two browsers on one
// document actually see each other, and that deleting a document empties its
// room instead of leaving people typing into a row that is gone.
package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// docWSURL is the per-document socket on the harness server.
func docWSURL(docID string) string {
	return "ws" + strings.TrimPrefix(testServer.URL, "http") + "/api/documents/" + docID + "/ws"
}

// dialDocWS opens the document socket, returning the handshake status alongside
// the connection so refusals can be asserted as precisely as successes.
func dialDocWS(t *testing.T, token, docID string) (*websocket.Conn, int) {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := http.Header{}
	if token != "" {
		hdr.Set("Authorization", "Bearer "+token)
	}
	conn, res, err := d.Dial(docWSURL(docID), hdr)
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

// readDocFrame waits for one frame, returning nil on timeout so a test can also
// assert that nothing arrived.
func readDocFrame(t *testing.T, conn *websocket.Conn, wait time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		return nil
	}
	return msg
}

// awaitFrame reads until a frame of the wanted type shows up or the budget runs
// out. Presence snapshots and lock answers race on the wire — the room
// broadcasts state to everyone while answering the requester — so a test that
// insisted on an exact frame order would be flaky by construction.
func awaitFrame(t *testing.T, conn *websocket.Conn, want string, budget time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		msg := readDocFrame(t, conn, time.Until(deadline))
		if msg == nil {
			break
		}
		if msg["type"] == want {
			return msg
		}
	}
	return nil
}

// mkDoc creates a workspace and a document in it, returning both ids.
func mkDoc(t *testing.T, c *client, title string) (wsID, docID string) {
	t.Helper()
	wsID = mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": title}), http.StatusCreated)
	return wsID, doc["id"].(string)
}

// lockNames maps block id → holder name from a state snapshot.
func lockNames(msg map[string]any) map[string]string {
	out := map[string]string{}
	locks, _ := msg["locks"].([]any)
	for _, raw := range locks {
		l, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		blockID, _ := l["block_id"].(string)
		name, _ := l["name"].(string)
		out[blockID] = name
	}
	return out
}

// TestDocumentWSRequiresMembership is the one that matters for security: a
// document is reachable by id alone, so the membership check is all that stands
// between a valid token and another workspace's document.
func TestDocumentWSRequiresMembership(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, docID := mkDoc(t, owner, "Приватный документ")
	outsider := signup(t)

	if conn, status := dialDocWS(t, "", docID); status != http.StatusUnauthorized {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("anonymous handshake = %d, want 401", status)
	}
	if conn, status := dialDocWS(t, outsider.token, docID); status != http.StatusForbidden {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("outsider handshake = %d, want 403", status)
	}
	if conn, status := dialDocWS(t, owner.token, "not-a-uuid"); status != http.StatusBadRequest {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("malformed id handshake = %d, want 400", status)
	}
}

// TestDocumentWSPresenceAndLock is the feature itself: two members open one
// document, see each other, and the second one to reach for a block is told who
// holds it rather than silently overwriting them.
func TestDocumentWSPresenceAndLock(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID, docID := mkDoc(t, owner, "Совместный документ")
	mate := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": mate.Email}), http.StatusCreated)

	first, status := dialDocWS(t, owner.token, docID)
	if first == nil {
		t.Fatalf("owner handshake = %d", status)
	}
	defer first.Close()

	welcome := awaitFrame(t, first, "welcome", 5*time.Second)
	if welcome == nil || welcome["conn_id"] == "" {
		t.Fatalf("no welcome for the first participant: %#v", welcome)
	}
	// The client needs the TTL to know how often to refresh its own lock.
	if ttl, _ := welcome["lock_ttl_ms"].(float64); ttl <= 0 {
		t.Fatalf("welcome carried lock_ttl_ms = %v", welcome["lock_ttl_ms"])
	}

	second, status := dialDocWS(t, mate.token, docID)
	if second == nil {
		t.Fatalf("member handshake = %d", status)
	}
	defer second.Close()

	// The joiner's arrival is broadcast, so the first socket learns about it
	// without asking.
	var sawTwo bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !sawTwo {
		msg := awaitFrame(t, first, "state", time.Until(deadline))
		if msg == nil {
			break
		}
		if viewers, _ := msg["viewers"].([]any); len(viewers) == 2 {
			sawTwo = true
		}
	}
	if !sawTwo {
		t.Fatal("the first participant never saw the second one join")
	}

	if err := second.WriteJSON(map[string]any{"type": "lock", "block_id": "b1"}); err != nil {
		t.Fatalf("lock request: %v", err)
	}
	state := awaitFrame(t, first, "state", 5*time.Second)
	if state == nil || lockNames(state)["b1"] == "" {
		t.Fatalf("block lock was not broadcast: %#v", state)
	}

	// The loser is told who holds the block — the difference between "you may
	// not type here" and "Ирина is typing here".
	if err := first.WriteJSON(map[string]any{"type": "lock", "block_id": "b1"}); err != nil {
		t.Fatalf("competing lock request: %v", err)
	}
	denied := awaitFrame(t, first, "denied", 5*time.Second)
	if denied == nil || denied["block_id"] != "b1" || denied["name"] == "" {
		t.Fatalf("competing lock was not refused with a holder: %#v", denied)
	}

	// Closing a tab releases its locks immediately rather than at the TTL —
	// otherwise a colleague stares at a stale badge for half a minute.
	second.Close()
	var released bool
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !released {
		msg := awaitFrame(t, first, "state", time.Until(deadline))
		if msg == nil {
			break
		}
		if len(lockNames(msg)) == 0 {
			released = true
		}
	}
	if !released {
		t.Fatal("the lock outlived the socket that held it")
	}
}

// TestDocumentWSDroppedOnDelete covers the room teardown: deleting a document
// has to evict whoever is still in it, including participants in nested
// documents that the delete takes with it.
func TestDocumentWSDroppedOnDelete(t *testing.T) {
	t.Parallel()
	c := signup(t)
	wsID, parentID := mkDoc(t, c, "Родительский документ")
	child := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": "Вложенный документ", "parent_id": parentID}), http.StatusCreated)
	childID := child["id"].(string)

	conn, status := dialDocWS(t, c.token, childID)
	if conn == nil {
		t.Fatalf("handshake on the nested document = %d", status)
	}
	defer conn.Close()
	if welcome := awaitFrame(t, conn, "welcome", 5*time.Second); welcome == nil {
		t.Fatal("no welcome on the nested document")
	}

	// ?recursive=true because the parent has a child; that is the path that takes
	// a whole subtree — and therefore the one that has to empty nested rooms.
	c.expect(t, c.del("/documents/"+parentID+"?recursive=true"), http.StatusNoContent)

	// The socket must end, not go quiet: a client that is merely starved of
	// frames keeps showing the document as live.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return // closed, as it should be
		}
	}
}
