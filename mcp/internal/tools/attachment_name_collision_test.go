package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// Two attachments on one task may carry the same filename — nothing forbids it
// (no unique index on (task_id, filename), and UploadAttachment stores the
// picked basename as it came). These pin what download does with that: one file
// per row, and every reported path holding the bytes of the row beside it
// (#2932).

// dupTaskMux is a task whose attachments collide by name, with distinct bodies
// per id so a truncated write shows up as the wrong size on disk.
func dupTaskMux(t *testing.T) (*client.Client, string, string) {
	t.Helper()
	c, _ := newMux(t, map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
		"/api/tasks/t1/attachments": []model.Attachment{
			{ID: "a1", Filename: "screenshot.png", Size: 6},
			{ID: "a2", Filename: "screenshot.png", Size: 11},
			{ID: "a3", Filename: "trace.log", Size: 5},
		},
		"/api/attachments/a1/download": "older",
		"/api/attachments/a2/download": "newer-after-fix",
		"/api/attachments/a3/download": "only",
	})
	return c, "older", "newer-after-fix"
}

func TestDownloadKeepsSameNamedAttachmentsApart(t *testing.T) {
	c, firstBody, secondBody := dupTaskMux(t)
	dest := t.TempDir()

	_, out, err := downloadAttachment(c)(context.Background(), nil, downloadAttachmentInput{
		TaskID: "t1", DestDir: dest,
	})
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	if len(out.Attachments) != 3 {
		t.Fatalf("got %d attachments, want 3", len(out.Attachments))
	}

	seen := map[string]string{} // path -> attachment id that claimed it
	for _, a := range out.Attachments {
		if prev, dup := seen[a.Path]; dup {
			t.Fatalf("attachments %s and %s both report %q — one overwrote the other", prev, a.ID, a.Path)
		}
		seen[a.Path] = a.ID
	}

	// Each reported path must hold that attachment's own bytes, and the size in
	// the reply must be what is actually on disk.
	want := map[string]string{"a1": firstBody, "a2": secondBody, "a3": "only"}
	for _, a := range out.Attachments {
		b, rErr := os.ReadFile(a.Path)
		if rErr != nil {
			t.Fatalf("%s: file not written: %v", a.ID, rErr)
		}
		// The mux encodes bodies as JSON, so the bytes are the quoted string.
		if got := string(b); got != `"`+want[a.ID]+`"`+"\n" {
			t.Errorf("%s at %q holds %q, want the body of %s", a.ID, a.Path, got, a.ID)
		}
		if int64(len(b)) != a.Size {
			t.Errorf("%s: reply says %d bytes, file has %d", a.ID, a.Size, len(b))
		}
	}

	// The first row keeps the plain name; only the collision is numbered, and the
	// attachment with a name of its own is untouched.
	byID := map[string]string{}
	for _, a := range out.Attachments {
		byID[a.ID] = filepath.Base(a.Path)
	}
	if byID["a1"] != "screenshot.png" {
		t.Errorf("first row renamed to %q, want screenshot.png", byID["a1"])
	}
	if byID["a2"] == "screenshot.png" || filepath.Ext(byID["a2"]) != ".png" {
		t.Errorf("second row got %q, want a distinct name keeping the extension", byID["a2"])
	}
	if byID["a3"] != "trace.log" {
		t.Errorf("uncontested name became %q, want trace.log", byID["a3"])
	}
}

// Downloading the same task twice into one directory refreshes it in place: the
// suffix is per call, so nothing accumulates -2, -3 across runs.
func TestDownloadIntoSameDirIsStable(t *testing.T) {
	c, _, _ := dupTaskMux(t)
	dest := t.TempDir()
	in := downloadAttachmentInput{TaskID: "t1", DestDir: dest}

	_, first, err := downloadAttachment(c)(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	_, second, err := downloadAttachment(c)(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	for i := range first.Attachments {
		if first.Attachments[i].Path != second.Attachments[i].Path {
			t.Errorf("%s moved from %q to %q on a repeat run",
				first.Attachments[i].ID, first.Attachments[i].Path, second.Attachments[i].Path)
		}
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("two runs left %d files, want 3", len(entries))
	}
}

func TestPickAttachmentsByNameReturnsEveryMatch(t *testing.T) {
	all := []model.Attachment{
		{ID: "a1", Filename: "screenshot.png"},
		{ID: "a2", Filename: "trace.log"},
		{ID: "a3", Filename: "SCREENSHOT.PNG"},
	}
	got, err := pickAttachments(all, "", "screenshot.png")
	if err != nil {
		t.Fatalf("pickAttachments: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a1" || got[1].ID != "a3" {
		t.Fatalf("by filename returned %+v, want both rows carrying that name", got)
	}

	// Controls: an id still names exactly one row, a unique name still returns
	// one, and an unknown name still fails with the list of what is there.
	if one, oErr := pickAttachments(all, "a3", ""); oErr != nil || len(one) != 1 || one[0].ID != "a3" {
		t.Errorf("by id: %+v %v", one, oErr)
	}
	if one, oErr := pickAttachments(all, "", "trace.log"); oErr != nil || len(one) != 1 || one[0].ID != "a2" {
		t.Errorf("unique filename: %+v %v", one, oErr)
	}
	if _, oErr := pickAttachments(all, "", "nope.txt"); oErr == nil {
		t.Error("expected an error for an unknown filename")
	}
}

func TestUniqueName(t *testing.T) {
	taken := map[string]bool{}
	cases := []struct{ in, want string }{
		{"a.png", "a.png"},
		{"a.png", "a-2.png"},
		{"a.png", "a-3.png"},
		// Case-insensitive: the same file on a case-folding filesystem.
		{"A.PNG", "A-4.PNG"},
		{"b.tar.gz", "b.tar.gz"},
		{"b.tar.gz", "b.tar-2.gz"},
		// No extension, and a dotfile whose whole name is the "extension".
		{"dump", "dump"},
		{"dump", "dump-2"},
		{".gitignore", ".gitignore"},
		{".gitignore", ".gitignore-2"},
	}
	for _, c := range cases {
		if got := uniqueName(taken, c.in); got != c.want {
			t.Errorf("uniqueName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
