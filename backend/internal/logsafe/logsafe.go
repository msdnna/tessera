// Package logsafe keeps text that came from outside the process — request
// paths, payload fields, error strings quoting a remote response — from forging
// log entries. Both the stdlib log lines and the slog text handler are read line
// by line (docker logs, journald, grep); a value carrying a newline would start
// what looks like a fresh entry of its own, and a terminal escape could repaint
// the screen of whoever tails the log.
//
// The cure is escaping, not stripping: the escaped form still shows what was
// sent, which is what an operator reading the line wants to know.
package logsafe

import (
	"fmt"
	"log"
	"strings"
)

// Str returns s with line breaks written as the two-character escapes \r and \n,
// and every other control character (C0, DEL) as \xNN. Tabs are kept. A string
// with nothing to escape comes back unchanged.
func Str(s string) string {
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if isControl(r) {
			fmt.Fprintf(&b, `\x%02x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Printf is log.Printf for an entry that carries outside input: the whole
// formatted line goes through Str, so it stays one line whatever the arguments
// hold — including errors, whose text often quotes a remote response.
func Printf(format string, args ...any) {
	// Depth 2: the caller's frame, should the log flags ever ask for a file:line.
	_ = log.Output(2, Str(fmt.Sprintf(format, args...)))
}

// isControl reports a control character other than tab: the C0 range and DEL.
// CR and LF are already escaped by the time it runs.
func isControl(r rune) bool {
	return (r < 0x20 && r != '\t') || r == 0x7f
}
