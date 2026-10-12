package main

import (
	"net/http"
	"testing"
)

// Removing a file from a task belongs in the task's journal (#2932).
//
// `attachment` was the only one-sided kind in task_events. Every other pair is
// written from both ends — tagged/untagged, assigned/unassigned,
// relation/relation_removed, archived/restored, completed/reopened — and only
// the attachment had no counterpart: DeleteAttachment dropped the row, removed
// the bytes and answered 204 without writing a line.
//
// That is the worst place to leave the gap, because attachments are the one
// contribution a member may destroy on someone else's behalf. A comment is
// author-only (DeleteComment answers 403 "not your comment"); an attachment is
// guarded by workspace membership alone, and the file itself goes with the row.
// So the history kept saying «прикрепил(а) файл «смета.pdf»» about a file the
// Files tab no longer listed, with nothing anywhere naming who took it or when.
//
// The entry records the file by NAME, like the tag on tagged/untagged: the row
// it came from is deleted in the same request, so an id would dangle.
func TestAttachmentRemovalIsJournalled(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Задача с вложением")["id"].(string)
	attachFile(t, owner, task, "смета.pdf", []byte("%PDF-1.7 smeta"))

	list := owner.get("/tasks/" + task + "/attachments").listBody(t)
	if len(list) != 1 {
		t.Fatalf("attachment did not land: %v", list)
	}
	attID := list[0]["id"].(string)

	// The other half of the pair is the one that already worked; asserting it
	// here keeps the two lines readable as a pair rather than as one entry with
	// a mystery beside it.
	if data := lastTagEvent(t, owner, task, "attachment"); data["filename"] != "смета.pdf" {
		t.Fatalf("attach journal = %v, want the file's name", data)
	}

	// Deleted by a DIFFERENT member: the whole point of the record is that the
	// person who lost the file is not the person who removed it, and the entry
	// has to name the remover rather than the uploader.
	other := addMember(t, owner, s.WS)
	other.expect(t, other.del("/attachments/"+attID), http.StatusNoContent)

	if l := owner.get("/tasks/" + task + "/attachments").listBody(t); len(l) != 0 {
		t.Fatalf("attachment survived the delete: %v", l)
	}
	data := lastTagEvent(t, owner, task, "attachment_removed")
	if data["filename"] != "смета.pdf" {
		t.Fatalf("removal journal = %v, want the file's name", data)
	}
	if n := countEvents(t, owner, task, "attachment_removed"); n != 1 {
		t.Fatalf("attachment_removed entries = %d, want 1", n)
	}

	// The actor is the remover, not the uploader — otherwise the line reads as
	// the author having deleted their own file.
	if actor := lastEventActor(t, owner, task, "attachment_removed"); actor != other.UserID {
		t.Fatalf("attachment_removed actor = %q, want the remover %q", actor, other.UserID)
	}
}

// lastEventActor returns the actor id of the newest journal entry of a kind.
func lastEventActor(t *testing.T, c *client, taskID, kind string) string {
	t.Helper()
	events := c.get("/tasks/" + taskID + "/events").listBody(t)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i]["kind"] != kind {
			continue
		}
		actor, _ := events[i]["actor_id"].(string)
		return actor
	}
	t.Fatalf("no %s event on task %s: %v", kind, taskID, events)
	return ""
}
