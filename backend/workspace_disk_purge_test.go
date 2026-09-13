package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// What a deleted *workspace* leaves on disk beyond its tasks (#2932).
//
// Three things this application writes to disk are owned by rows that hang off a
// workspace through ON DELETE CASCADE: a task's attachments, a document's images,
// and a conference's recordings + chat uploads. Deleting the workspace takes all
// three sets of rows at once — and with them storage_path, the src attribute and
// file_path, which are the only records of where the bytes live.
//
// The per-object deletes all know this. DeleteTask sweeps <uploadDir>/<taskID>/,
// DeleteDocument calls purgeDocAssets, DeleteConference reads its recording paths
// before the cascade and then purges rec/ and conf/ (#2877 — "the largest thing
// this application ever writes to disk"). DeleteWorkspace swept the tasks only:
// the other two directories stayed full for good, on the one gesture after which
// nobody is left to notice, and on the one object an owner deletes precisely to
// reclaim what it was using.
//
// Each test keeps a second workspace of the same owner alive, so the cheapest
// wrong fix — clearing documents/ or rec/ or conf/ wholesale — fails here.

// confRecDir is where a conference's recordings live (handlers.purgeRecordingDir).
func confRecDir(confID string) string {
	return filepath.Join(testUploadDir, "rec", confID)
}

// TestDeletingAWorkspaceTakesItsDocumentsImages: a document's images are named
// by nothing but the body of the document itself, so once the cascade has taken
// the row the directory is unreachable forever.
func TestDeletingAWorkspaceTakesItsDocumentsImages(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())

	doc := mkDocument(t, owner, ws, "Регламент удаляемого пространства")
	putDocImages(t, owner, doc, "scheme.png", "screenshot.png")
	dir := docAssetDir(doc)
	if n, ok := countFiles(t, dir); !ok || n != 2 {
		t.Fatalf("document images did not land: %d file(s) in %s (exists=%v)", n, dir, ok)
	}

	// A second workspace of the same owner: its images must stay.
	otherWS := mkWorkspace(t, owner, "WS-2 "+t.Name())
	neighbour := mkDocument(t, owner, otherWS, "Документ соседнего пространства")
	putDocImages(t, owner, neighbour, "neighbour.png")

	owner.expect(t, owner.del("/workspaces/"+ws), http.StatusNoContent)

	if got := owner.get("/documents/" + doc); got.Status != http.StatusNotFound {
		t.Fatalf("document of a deleted workspace still readable: %d", got.Status)
	}
	if n, ok := countFiles(t, dir); ok {
		t.Fatalf("workspace delete left %d document image(s) no body points at", n)
	}
	if n, ok := countFiles(t, docAssetDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a workspace that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingAWorkspaceTakesItsConferencesRecordings: the recordings are the
// largest files here, and their paths live in rows the cascade removes.
func TestDeletingAWorkspaceTakesItsConferencesRecordings(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := liveConference(t, owner, "Летучка удаляемого пространства")
	_, first := plantRecording(t, confID, "completed")
	_, second := plantRecording(t, confID, "failed")

	// A call in a second workspace, so "swept" cannot mean "swept rec/".
	_, otherConf := liveConference(t, owner, "Летучка соседнего пространства")
	_, kept := plantRecording(t, otherConf, "completed")

	owner.expect(t, owner.del("/workspaces/"+s.WS), http.StatusNoContent)

	if got := owner.get("/conferences/" + confID); got.Status != http.StatusNotFound {
		t.Fatalf("conference of a deleted workspace still readable: %d", got.Status)
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("recording %s survived the workspace: %v", filepath.Base(p), err)
		}
	}
	if n, ok := countFiles(t, confRecDir(confID)); ok {
		t.Fatalf("workspace delete left the recording directory with %d file(s)", n)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("the sweep reached a workspace that was not deleted: %v", err)
	}
}

// TestDeletingAWorkspaceTakesItsConferencesChatFiles: same obligation for the
// files pasted into a call's chat, whose attachment rows go the same way.
func TestDeletingAWorkspaceTakesItsConferencesChatFiles(t *testing.T) {
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

	// A call in a second workspace, so "swept" cannot mean "swept conf/".
	_, otherConf := mkConference(t, owner, "Летучка соседнего пространства")
	postChat(t, owner, otherConf, "тоже файл", map[string][]byte{
		"other.png": []byte("\x89PNG\r\n\x1a\nthird fake image"),
	})

	owner.expect(t, owner.del("/workspaces/"+s.WS), http.StatusNoContent)

	if n, ok := countFiles(t, dir); ok {
		t.Fatalf("workspace delete left %d chat file(s) no row points at", n)
	}
	if n, ok := countFiles(t, confChatDir(otherConf)); !ok || n != 1 {
		t.Fatalf("the sweep reached a workspace that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingAWorkspaceWithoutDocumentsOrCallsIsFine pins the common case: a
// workspace is deleted far more often with nothing on disk behind it, and a
// sweep that only works when there is something to sweep is one that fails the
// moment it matters.
func TestDeletingAWorkspaceWithoutDocumentsOrCallsIsFine(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())
	owner.expect(t, owner.del("/workspaces/"+ws), http.StatusNoContent)
}
