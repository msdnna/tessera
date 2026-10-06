// A notification belongs to exactly one person, but deliverNotification pushed it
// with the plain workspace broadcast — and the hub's only filter is the workspace
// scope. Every member's socket therefore received every other member's
// notification, text and payload included; the web client dropped the foreign
// ones by comparing data.user_id to the signed-in user, which is authorization
// done at the wrong end of the wire. The reminder kind makes the cost concrete:
// its text IS the user's own private reminder message.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNotificationGoesOnlyToItsRecipientSocket(t *testing.T) {
	owner := signup(t)
	s := mkStack(t, owner)

	victim := signup(t)
	nosy := signup(t)
	for _, u := range []*client{victim, nosy} {
		owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
			map[string]any{"email": u.Email, "role": "member"}), http.StatusCreated)
	}

	// Linked to a task so reminderScope resolves to this workspace rather than
	// the reminder owner's own first one — that is the path where the reminder's
	// text reaches colleagues.
	task := mkTask(t, owner, s.Board, s.col(t, 0), "Ремонт склада")
	taskID := task["id"].(string)

	victimConn, st := dialWS(t, victim.token, false)
	if st != http.StatusSwitchingProtocols || victimConn == nil {
		t.Fatalf("victim socket handshake: %d", st)
	}
	defer victimConn.Close()
	nosyConn, st := dialWS(t, nosy.token, false)
	if st != http.StatusSwitchingProtocols || nosyConn == nil {
		t.Fatalf("nosy socket handshake: %d", st)
	}
	defer nosyConn.Close()

	// The reminder is created only after both sockets are live, so the harness's
	// own scanner tick cannot deliver it before anyone is listening.
	const secret = "забрать анализы, диагноз Д-84"
	victim.expect(t, victim.post("/reminders", map[string]any{
		"task_id":   taskID,
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   secret,
	}), http.StatusCreated)
	if !testAPI.ScanNotificationsOnce(context.Background()) {
		// Another instance of the sweep held the lock; the background ticker will
		// have done the work instead, which broadcasts just the same.
		t.Log("scanner tick skipped — the harness ticker delivered it")
	}

	t.Run("the recipient gets it", func(t *testing.T) {
		if ev := awaitNotification(t, victimConn, secret); ev == nil {
			t.Fatal("the reminder never reached its own owner's socket")
		}
	})

	t.Run("a colleague's socket never sees it", func(t *testing.T) {
		if ev := awaitNotification(t, nosyConn, secret); ev != nil {
			t.Fatalf("another member's socket received the private reminder: %v", ev)
		}
	})
}

// awaitNotification drains frames until one is a notification event carrying
// want, or the budget runs out (nil). Other traffic — task/board events from the
// fixture build — is skipped rather than failing the read.
func awaitNotification(t *testing.T, conn *websocket.Conn, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ev := readEvent(t, conn, time.Until(deadline))
		if ev == nil {
			return nil
		}
		if ev["type"] != "notification" {
			continue
		}
		raw, err := json.Marshal(ev)
		if err == nil && strings.Contains(string(raw), want) {
			return ev
		}
	}
	return nil
}
