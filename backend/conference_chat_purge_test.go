package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// What a deleted conference leaves on disk (#2932).
//
// Deleting a conference takes its chat with it by cascade: the messages go, and
// with them the attachment rows that were the only record of where the bytes
// live. DeleteConferenceMessage knows this and reads the paths before the row
// goes; deleting the whole conference has the same obligation for every message
// at once, and the files are the same 25 MiB-a-piece uploads.
//
// Both halves are pinned below, because the failure is silent either way: on
// disk nothing complains, and the API answers 404 for the conference whether or
// not its files are still there.

// confChatDir is where a conference's chat uploads live (handlers.storeChatFiles).
func confChatDir(confID string) string {
	return filepath.Join(testUploadDir, "conf", confID)
}

// countFiles reports how many entries a directory holds, and whether it exists.
func countFiles(t *testing.T, dir string) (int, bool) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, false
	}
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	return len(entries), true
}

// TestDeletingAConferenceTakesItsChatFilesWithIt: the bytes behind a deleted
// call's chat must not outlive the rows that named them.
//
// A conference is deleted by a host who is finished with it — the same gesture
// that is meant to clear the recording. Every file ever pasted into its chat sits
// under conf/<id>/, reachable only through attachment rows that the cascade has
// just removed, so anything left behind is unreachable forever: no sweep walks
// that directory, and the moderator has no way to learn it is still full.
func TestDeletingAConferenceTakesItsChatFilesWithIt(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с файлами")
	mate := addMember(t, owner, s.WS)

	// Two posters, so the sweep cannot be mistaken for "the caller's own files".
	postChat(t, owner, confID, "смотрите скрин", map[string][]byte{
		"screenshot.png": []byte("\x89PNG\r\n\x1a\nfake image body"),
	})
	postChat(t, mate, confID, "и логи", map[string][]byte{
		"trace.png": []byte("\x89PNG\r\n\x1a\nanother fake image"),
	})

	dir := confChatDir(confID)
	if n, ok := countFiles(t, dir); !ok || n != 2 {
		t.Fatalf("chat uploads did not land: %d file(s) in %s (exists=%v)", n, dir, ok)
	}

	// A second call in the same workspace, so "swept" cannot mean "swept conf/".
	// The directories are siblings under one root and the cheapest wrong fix —
	// clearing the root — would pass every other assertion here.
	_, other := mkConference(t, owner, "Соседняя летучка")
	postChat(t, owner, other, "тоже файл", map[string][]byte{
		"other.png": []byte("\x89PNG\r\n\x1a\nthird fake image"),
	})

	owner.expect(t, owner.del("/conferences/"+confID), http.StatusNoContent)

	// The conference is gone from the API...
	if got := owner.get("/conferences/" + confID); got.Status != http.StatusNotFound {
		t.Fatalf("deleted conference still readable: %d", got.Status)
	}
	// ...so nothing can name these files any more.
	if n, ok := countFiles(t, dir); ok {
		t.Fatalf("chat directory survived the conference with %d file(s) no row points at", n)
	}
	// The live call next door still has its chat.
	if n, ok := countFiles(t, confChatDir(other)); !ok || n != 1 {
		t.Fatalf("the sweep reached a conference that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingAConferenceWithoutChatFilesIsFine pins the ordinary delete — the
// overwhelmingly common one, a call nobody attached anything to. A sweep that
// only works when there is something to sweep is a sweep that fails the moment
// it matters, so the empty case goes through the same path and must stay quiet.
func TestDeletingAConferenceWithoutChatFilesIsFine(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Летучка без файлов")

	owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "только текст"}), http.StatusCreated)

	owner.expect(t, owner.del("/conferences/"+confID), http.StatusNoContent)

	if n, ok := countFiles(t, confChatDir(confID)); ok {
		t.Fatalf("directory appeared for a conference with no uploads: %d entr(ies)", n)
	}
}
