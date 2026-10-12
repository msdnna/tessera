// Offline capture (#3042): a note or reminder queued on the device carries the id
// the client gave it; resending it after a lost answer finds the first record
// instead of creating a twin, and nobody else can claim that id.
package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestInboxCaptureClientID(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	other := signup(t)
	ws := mkWorkspace(t, owner, "WS offline capture")
	otherWS := mkWorkspace(t, other, "WS other")

	id := uuid.NewString()
	body := map[string]any{"title": "Купить молоко", "client_id": id}
	first := owner.expect(t, owner.post("/workspaces/"+ws+"/notes", body), http.StatusCreated)
	if first["id"] != id {
		t.Fatalf("note id = %v, want the client's %s", first["id"], id)
	}
	again := owner.expect(t, owner.post("/workspaces/"+ws+"/notes", body), http.StatusOK)
	if again["id"] != id {
		t.Fatalf("resend answered %v", again["id"])
	}
	n := 0
	for _, x := range owner.get("/workspaces/" + ws + "/notes").listBody(t) {
		if x["title"] == "Купить молоко" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("resend made %d notes, want 1", n)
	}
	if r := other.post("/workspaces/"+otherWS+"/notes", body); r.Status != http.StatusConflict {
		t.Fatalf("someone else's client_id: status %d, want 409", r.Status)
	}

	rid := uuid.NewString()
	rem := map[string]any{
		"message": "Позвонить", "remind_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		"workspace_id": ws, "client_id": rid,
	}
	r1 := owner.expect(t, owner.post("/reminders", rem), http.StatusCreated)
	r2 := owner.expect(t, owner.post("/reminders", rem), http.StatusOK)
	if r1["id"] != rid || r2["id"] != rid {
		t.Fatalf("reminder ids %v / %v, want %s", r1["id"], r2["id"], rid)
	}
	if r := other.post("/reminders", rem); r.Status != http.StatusConflict {
		t.Fatalf("someone else's reminder client_id: status %d, want 409", r.Status)
	}
}
