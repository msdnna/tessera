package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tessera-mcp/internal/model"
)

func TestPickAttachments(t *testing.T) {
	all := []model.Attachment{
		{ID: "a1", Filename: "trace.log"},
		{ID: "a2", Filename: "dump.json"},
	}
	if _, err := pickAttachments(nil, "", ""); err == nil {
		t.Fatal("expected error for a task with no attachments")
	}
	if got, _ := pickAttachments(all, "", ""); len(got) != 2 {
		t.Fatalf("no selector should return all, got %d", len(got))
	}
	if got, err := pickAttachments(all, "a2", ""); err != nil || got[0].Filename != "dump.json" {
		t.Fatalf("by id: %+v %v", got, err)
	}
	if got, err := pickAttachments(all, "", "TRACE.LOG"); err != nil || got[0].ID != "a1" {
		t.Fatalf("by filename (case-insensitive): %+v %v", got, err)
	}
	if _, err := pickAttachments(all, "", "missing.txt"); err == nil {
		t.Fatal("expected error for an unknown filename")
	}
}

func TestUploadAttachmentValidates(t *testing.T) {
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": detail("t1", "b1")})
	ctx := context.Background()

	if _, _, err := uploadAttachment(c)(ctx, nil, uploadAttachmentInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error for empty file_paths")
	}
	if _, _, err := uploadAttachment(c)(ctx, nil,
		uploadAttachmentInput{TaskID: "t1", FilePaths: []string{"/nope/missing.log"}}); err == nil {
		t.Fatal("expected error for an unreadable file")
	}

	path := filepath.Join(t.TempDir(), "trace.log")
	if err := os.WriteFile(path, []byte("boom"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, out, err := uploadAttachment(c)(ctx, nil, uploadAttachmentInput{TaskID: "t1", FilePaths: []string{path}})
	if err != nil || len(out.Attachments) != 1 || out.Attachments[0].ID != "cm-new" {
		t.Fatalf("uploadAttachment: %+v %v", out, err)
	}
	if _, ok := mux.writes["POST /api/tasks/t1/attachments"]; !ok {
		t.Fatalf("attachment not posted: %v", mux.writes)
	}
}

func TestDownloadAttachmentWritesFiles(t *testing.T) {
	c, _ := newMux(t, map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
		"/api/tasks/t1/attachments": []model.Attachment{
			// A filename with a path in it must not escape dest_dir.
			{ID: "a1", Filename: "../../evil.log", Size: 4},
		},
		"/api/attachments/a1/download": "IGNORED", // the mux encodes it as JSON; only the bytes matter
	})
	dest := t.TempDir()
	_, out, err := downloadAttachment(c)(context.Background(), nil, downloadAttachmentInput{
		TaskID: "t1", DestDir: dest,
	})
	if err != nil || len(out.Attachments) != 1 {
		t.Fatalf("downloadAttachment: %+v %v", out, err)
	}
	if got := out.Attachments[0].Path; got != filepath.Join(dest, "evil.log") {
		t.Fatalf("wrote to %q, want it kept inside %q", got, dest)
	}
	if _, err := os.Stat(out.Attachments[0].Path); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	if _, _, err := downloadAttachment(c)(context.Background(), nil, downloadAttachmentInput{
		TaskID: "t1", DestDir: dest, Filename: "nope.log",
	}); err == nil {
		t.Fatal("expected error for an unknown filename")
	}
}

func TestAttachmentMIME(t *testing.T) {
	cases := map[string]string{
		"a.png": "image/png", "b.PDF": "application/pdf",
		"c.log": "text/plain; charset=utf-8", "d.bin": "application/octet-stream",
	}
	for name, want := range cases {
		if got := attachmentMIME(name); got != want {
			t.Errorf("attachmentMIME(%q) = %q, want %q", name, got, want)
		}
	}
}
