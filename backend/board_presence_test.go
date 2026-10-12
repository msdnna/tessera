// Presence on a board (#3116): a socket announces the board it shows with a
// client→server frame; the workspace hears who is there, a stranger does not,
// and closing the socket takes its user off the board at once.
package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// presenceOf waits for a board.presence event about board and returns its users.
func presenceOf(t *testing.T, conn *websocket.Conn, board string) []any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ev := readEvent(t, conn, time.Until(deadline))
		if ev == nil {
			break
		}
		d, _ := ev["data"].(map[string]any)
		if ev["type"] == "board.presence" && d["board_id"] == board {
			users, _ := d["user_ids"].([]any)
			return users
		}
	}
	t.Fatalf("no board.presence for %s", board)
	return nil
}

func TestBoardPresenceOverSocket(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	mate := signup(t)
	stranger := signup(t)
	st := mkStack(t, owner)
	owner.expect(t, owner.post("/workspaces/"+st.WS+"/members",
		map[string]any{"email": mate.Email, "role": "member"}), http.StatusCreated)

	ownerConn, _ := dialWS(t, owner.token, false)
	defer ownerConn.Close()
	mateConn, _ := dialWS(t, mate.token, false)
	strangerConn, _ := dialWS(t, stranger.token, false)
	defer strangerConn.Close()
	if ownerConn == nil || mateConn == nil || strangerConn == nil {
		t.Fatal("handshake failed")
	}

	frame := map[string]any{"type": "presence", "board_id": st.Board}
	if err := mateConn.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	if users := presenceOf(t, ownerConn, st.Board); len(users) != 1 || users[0] != mate.UserID {
		t.Fatalf("owner sees on the board: %v, want the mate", users)
	}

	// A stranger can neither hear about the board nor stand on it.
	// (A read timeout breaks a gorilla socket for good — the negative check reads
	// on a second socket of the owner, so ownerConn stays usable.)
	watch, _ := dialWS(t, owner.token, false)
	defer watch.Close()
	if err := strangerConn.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	if ev := readEvent(t, watch, 500*time.Millisecond); ev != nil && ev["type"] == "board.presence" {
		t.Fatalf("a stranger's frame was announced: %+v", ev)
	}

	// The stranger never heard of the board (read last: a timeout ends the socket).
	if leaked := readEvent(t, strangerConn, 300*time.Millisecond); leaked != nil {
		t.Fatalf("stranger got %+v", leaked)
	}

	// Closing the socket takes the mate off the board.
	_ = mateConn.Close()
	if users := presenceOf(t, ownerConn, st.Board); len(users) != 0 {
		t.Fatalf("after the socket closed: %v", users)
	}
}
