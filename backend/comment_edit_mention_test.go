package main

import (
	"net/http"
	"testing"
)

// TestEditedCommentNotifiesTheNewlyMentioned pins the mention seam on the EDIT
// path (#2932).
//
// Both editors offer the same @-popup the main composer does, so a «@Иван,
// посмотри» added by editing a comment draws a resolved chip — and the chip a
// reader sees IS the promise that the person was told (utils/mentions.js says so
// in as many words). PATCH /comments/:id took only `body`, so that promise was
// broken silently: nobody was notified, ever, and the chip looked exactly like
// one that had been.
//
// The ids come from the client because only it can produce them: a handle
// becomes a chip by being rendered against the roster, and the server holds the
// old body as plain text. For the same reason it is the *added* ids that travel
// — see the count checks below, which are what keeps a typo fix from pinging
// everyone named in the comment all over again.
func TestEditedCommentNotifiesTheNewlyMentioned(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	mate := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": mate.Email, "role": "member"}), http.StatusCreated)
	outsider := signup(t) // never a member of this workspace

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Смета на подрядчика")
	taskID := task["id"].(string)

	// The comment as first written names nobody: the mention is what the edit adds.
	created := owner.expect(t, owner.post("/tasks/"+taskID+"/comments",
		map[string]any{"body": "Черновик готов"}), http.StatusCreated)
	commentID := created["id"].(string)

	if n := mentionCount(t, mate); n != 0 {
		t.Fatalf("mate has %d mention notifications before the edit, want 0", n)
	}

	owner.expect(t, owner.patch("/comments/"+commentID, map[string]any{
		"body":     "Черновик готов. @" + mate.Email + ", посмотри",
		"mentions": []string{mate.UserID},
	}), http.StatusOK)

	t.Run("the person the edit names is notified", func(t *testing.T) {
		if n := mentionCount(t, mate); n != 1 {
			t.Fatalf("mate has %d mention notifications after the edit, want 1", n)
		}
	})

	t.Run("a later edit naming nobody new stays quiet", func(t *testing.T) {
		// What a typo fix looks like on the wire: the text still mentions mate, but
		// the client sends no *added* ids, so the notification must not repeat.
		owner.expect(t, owner.patch("/comments/"+commentID, map[string]any{
			"body":     "Черновик готов. @" + mate.Email + ", посмотри его",
			"mentions": []string{},
		}), http.StatusOK)
		if n := mentionCount(t, mate); n != 1 {
			t.Fatalf("mate has %d mention notifications after a second edit, want still 1", n)
		}
	})

	t.Run("an edit is not a second comment", func(t *testing.T) {
		// The participants were told when the comment was written; an edit must not
		// go out as a fresh "commented" notice on top of the mention.
		for _, n := range mate.get("/notifications").listBody(t) {
			if kind, _ := n["kind"].(string); kind == "comment" {
				t.Fatalf("the edit was announced as a new comment: %v", n)
			}
		}
	})

	t.Run("a non-member named by the client hears nothing", func(t *testing.T) {
		owner.expect(t, owner.patch("/comments/"+commentID, map[string]any{
			"body":     "Черновик готов. @" + outsider.Email + ", посмотри",
			"mentions": []string{outsider.UserID},
		}), http.StatusOK)
		if n := mentionCount(t, outsider); n != 0 {
			t.Fatalf("an outsider got %d notifications from a workspace they are not in, want 0", n)
		}
	})

	t.Run("the body is still saved", func(t *testing.T) {
		// The notification is a side effect; the edit itself must keep working.
		for _, cm := range owner.get("/tasks/" + taskID + "/comments").listBody(t) {
			if cm["id"] != commentID {
				continue
			}
			if body, _ := cm["body"].(string); body == "Черновик готов" {
				t.Fatal("the comment kept its original body — the edit did not land")
			}
			return
		}
		t.Fatal("the edited comment is gone from the task")
	})
}

// mentionCount counts the "mention" notifications a client currently holds.
func mentionCount(t *testing.T, c *client) int {
	t.Helper()
	n := 0
	for _, row := range c.get("/notifications").listBody(t) {
		if kind, _ := row["kind"].(string); kind == "mention" {
			n++
		}
	}
	return n
}
