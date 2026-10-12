package logsafe

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestStr(t *testing.T) {
	cases := map[string]string{
		"plain":                      "plain",
		"tab\tkept":                  "tab\tkept",
		"a\nb":                       `a\nb`,
		"a\r\n2026/10/12 forged":     `a\r\n2026/10/12 forged`,
		"bell\a esc\x1b[31m del\x7f": `bell\x07 esc\x1b[31m del\x7f`,
		"кириллица\nтоже":            `кириллица\nтоже`,
	}
	for in, want := range cases {
		if got := Str(in); got != want {
			t.Errorf("Str(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintfKeepsOneLine(t *testing.T) {
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	Printf("device %s: %v", "x\n[GIN] 200 | forged", 7)
	out := buf.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("want exactly one terminated line, got %q", out)
	}
	if want := `device x\n[GIN] 200 | forged: 7` + "\n"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
