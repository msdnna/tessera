package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The happy path: a directory that does not exist yet is created and probed.
func TestCheckUploadDir_CreatesAndWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")
	if err := checkUploadDir(dir); err != nil {
		t.Fatalf("checkUploadDir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory was not created: %v", err)
	}
	// The probe must not leave anything behind — the media route lists this
	// directory by name, and a stray file would be served as an upload.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("probe left %d file(s) behind: %v", len(entries), entries)
	}
}

// The #2820 shape: the directory exists but belongs to somebody else, so it is
// writable to them and not to us. An existing-but-unwritable directory has to be
// an error, not a pass — that is precisely what the old code missed.
func TestCheckUploadDir_ReportsUnwritableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "uploads")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := checkUploadDir(dir); err == nil {
		t.Fatal("checkUploadDir accepted a read-only directory")
	}
}

// A path whose parent is read-only cannot be created at all — the error must
// come back as a value rather than a panic.
func TestCheckUploadDir_ReportsUncreatableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(parent, 0o555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := checkUploadDir(filepath.Join(parent, "uploads")); err == nil {
		t.Fatal("checkUploadDir accepted an uncreatable directory")
	}
}
