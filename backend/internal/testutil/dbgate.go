// Package testutil holds helpers shared by the test harnesses in this module.
//
// It exists because the same decision — "the test database is unreachable, now
// what?" — is made from two different packages (the in-process integration
// harness in package main and the black-box suite in e2e), and duplicating the
// wording in both is the reliable way to fix one and forget the other.
package testutil

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// RequireDBEnv turns a skipped tier into a hard failure. Deliberately not named
// CI: that variable is set in plenty of environments and would change the
// meaning of unrelated local runs without anyone asking for it.
const RequireDBEnv = "TESSERA_TEST_REQUIRE_DB"

// SkippedMarker is a single greppable line that identifies a tier which did not
// run, for anything parsing the banner.
const SkippedMarker = "TESSERA_TEST_SKIPPED"

// SkipLogEnv names a file the banner is appended to, on top of stderr.
//
// It has to exist because stderr alone is not enough: `go test` buffers a test
// binary's output and throws it away when the package passes, and a soft skip
// passes by construction (exit 0). Measured on this tree — a skipped run prints
// exactly "ok tessera 0.170s" and nothing else, with or without the banner. So
// the Makefile hands us a temp file and prints it after go test returns.
const SkipLogEnv = "TESSERA_TEST_SKIP_LOG"

// SkipOrFail reports the process exit code for a tier that could not reach its
// database, and prints a banner saying so.
//
// A skip stays the default: on a box with no Postgres the suite has to be a
// no-op. What it must never be is *indistinguishable from success* — `go test`
// prints "ok" for a package where nothing ran, and the old log.Printf was
// invisible without -v. Hence the banner, which is printed either way.
func SkipOrFail(tier string, cause error) int {
	code, banner := decide(tier, cause, os.Getenv(RequireDBEnv))
	// stderr covers the runs where go test does show the output: -v, and any
	// failing package (which includes the strict mode below).
	fmt.Fprint(os.Stderr, banner)
	// The skip log covers the case stderr cannot: a soft skip that exits 0.
	if path := os.Getenv(SkipLogEnv); path != "" {
		appendSkipLog(path, banner)
	}
	return code
}

// appendSkipLog is best-effort on purpose: an unwritable path is not a reason to
// change the outcome of a test run, and the banner is already on stderr.
func appendSkipLog(path, banner string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (could not write %s=%s: %v)\n", SkipLogEnv, path, err)
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(banner)
}

// decide is the pure half of SkipOrFail, so the behaviour can be unit-tested
// without an os.Exit path.
func decide(tier string, cause error, requireDB string) (int, string) {
	strict := isTruthy(requireDB)

	var b strings.Builder
	if strict {
		b.WriteString(box(
			fmt.Sprintf("FAILED: %s tier could not run (no test database)", tier),
			"0 tests executed — "+RequireDBEnv+" makes that an error",
		))
	} else {
		b.WriteString(box(
			fmt.Sprintf("SKIPPED: %s tier did not run (no test database)", tier),
			"0 tests executed — this is NOT a passing run",
			"set "+RequireDBEnv+"=1 ("+strictTarget(tier)+") to fail instead",
		))
	}
	if cause != nil {
		fmt.Fprintf(&b, "  cause: %v\n", cause)
	}
	// The marker goes below the box on its own line: the box is for humans, this
	// is what the Makefile greps for.
	fmt.Fprintf(&b, "%s: %s\n", SkippedMarker, tier)

	if strict {
		return 1, b.String()
	}
	return 0, b.String()
}

// strictTarget names the make target that turns this tier's skip red, so the
// hint points at something that exists rather than at the backend one always.
func strictTarget(tier string) string {
	if tier == "e2e" {
		return "make test-e2e-backend-strict"
	}
	return "make test-backend-strict"
}

// isTruthy treats anything other than an explicit "off" value as strict. A typo
// like REQUIRE_DB=y silently degrading back into a green no-op run is the very
// failure mode this file exists to prevent, so unknown values err loud.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// box draws the lines inside a frame sized to the widest one.
func box(lines ...string) string {
	width := 0
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > width {
			width = n
		}
	}
	var b strings.Builder
	b.WriteString("\n╔═" + strings.Repeat("═", width) + "═╗\n")
	for _, l := range lines {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(l))
		b.WriteString("║ " + l + pad + " ║\n")
	}
	b.WriteString("╚═" + strings.Repeat("═", width) + "═╝\n")
	return b.String()
}
