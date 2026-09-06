package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecideSkipsByDefaultButSaysSo(t *testing.T) {
	code, banner := decide("integration", errors.New("connection refused"), "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (a box with no Postgres must still be a no-op)", code)
	}
	// The whole point of the task: a skip may not read as a pass.
	if !strings.Contains(banner, "SKIPPED") || !strings.Contains(banner, "NOT a passing run") {
		t.Fatalf("banner does not say the tier was skipped:\n%s", banner)
	}
	if !strings.Contains(banner, "connection refused") {
		t.Fatalf("banner drops the cause:\n%s", banner)
	}
	if !strings.Contains(banner, SkippedMarker+": integration") {
		t.Fatalf("banner lacks the marker the Makefile greps for:\n%s", banner)
	}
}

func TestDecideHintNamesTheTiersOwnStrictTarget(t *testing.T) {
	// A hint pointing at the wrong target is worse than none: it sends the
	// reader to a command that leaves this tier skipped anyway.
	if _, banner := decide("e2e", nil, ""); !strings.Contains(banner, "make test-e2e-backend-strict") {
		t.Fatalf("e2e banner points elsewhere:\n%s", banner)
	}
	if _, banner := decide("integration", nil, ""); !strings.Contains(banner, "make test-backend-strict") {
		t.Fatalf("integration banner points elsewhere:\n%s", banner)
	}
}

func TestDecideFailsWhenDBRequired(t *testing.T) {
	code, banner := decide("e2e", errors.New("connection refused"), "1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 when %s is set", code, RequireDBEnv)
	}
	if !strings.Contains(banner, "FAILED") {
		t.Fatalf("banner still reads as a skip:\n%s", banner)
	}
}

func TestDecideTreatsUnknownValuesAsStrict(t *testing.T) {
	// A typo must not silently restore the false-green this file prevents.
	for _, v := range []string{"1", "true", "yes", "on", "y", "please"} {
		if code, _ := decide("integration", nil, v); code != 1 {
			t.Errorf("%s=%q → exit %d, want 1", RequireDBEnv, v, code)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "off", " OFF "} {
		if code, _ := decide("integration", nil, v); code != 0 {
			t.Errorf("%s=%q → exit %d, want 0", RequireDBEnv, v, code)
		}
	}
}

func TestDecideBannerIsRectangular(t *testing.T) {
	// The frame is padded by rune count, not bytes — an em dash or a Cyrillic
	// letter in a line would otherwise ragged the right edge.
	_, banner := decide("integration", nil, "")
	var width int
	for _, line := range strings.Split(strings.TrimSpace(banner), "\n") {
		if !strings.HasPrefix(line, "╔") && !strings.HasPrefix(line, "║") && !strings.HasPrefix(line, "╚") {
			continue
		}
		n := utf8Len(line)
		if width == 0 {
			width = n
		} else if n != width {
			t.Fatalf("line %q is %d runes, want %d:\n%s", line, n, width, banner)
		}
	}
	if width == 0 {
		t.Fatalf("no framed lines in banner:\n%s", banner)
	}
}

func utf8Len(s string) int { return len([]rune(s)) }

func TestSkipOrFailWritesTheSkipLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skip.log")
	t.Setenv(RequireDBEnv, "")
	t.Setenv(SkipLogEnv, path)

	if code := SkipOrFail("integration", errors.New("connection refused")); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("skip log not written: %v", err)
	}
	// This file is the only channel that survives a passing package, so the
	// banner has to land here in full, not just the marker.
	if !strings.Contains(string(got), "SKIPPED") || !strings.Contains(string(got), SkippedMarker) {
		t.Fatalf("skip log lacks the banner:\n%s", got)
	}
}

func TestSkipOrFailWithoutSkipLogLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(RequireDBEnv, "")
	t.Setenv(SkipLogEnv, "")

	SkipOrFail("integration", errors.New("connection refused"))

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unset %s still produced files: %v", SkipLogEnv, entries)
	}
}
