package handlers

import (
	"testing"
	"time"
)

func TestParseImportCSVSemicolonAndBOM(t *testing.T) {
	data := []byte("\uFEFFName;Tags;Name\n\"A; quoted\";x, y;dup\n;;\nB;;\n")
	tb, err := parseImportFile(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Columns) != 3 || tb.Columns[0] != "Name" || tb.Columns[2] != "Name (2)" {
		t.Fatalf("columns = %q", tb.Columns)
	}
	if len(tb.Rows) != 2 {
		t.Fatalf("blank rows should be skipped, got %d rows", len(tb.Rows))
	}
	if tb.Rows[0]["Name"] != "A; quoted" || tb.Rows[0]["Name (2)"] != "dup" {
		t.Fatalf("row 0 = %v", tb.Rows[0])
	}
	// The log names the file's own line, past the skipped blank one.
	if len(tb.Lines) != 2 || tb.Lines[0] != 2 || tb.Lines[1] != 4 {
		t.Fatalf("lines = %v, want [2 4]", tb.Lines)
	}
}

func TestParseImportJSONShapes(t *testing.T) {
	for _, src := range []string{
		`[{"title":"a","tags":["x","y"],"n":3}]`,
		`{"board":{},"tasks":[{"title":"a","tags":["x","y"],"n":3}]}`,
	} {
		tb, err := parseImportFile([]byte(src), "")
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if len(tb.Rows) != 1 || tb.Rows[0]["tags"] != "x, y" || tb.Rows[0]["n"] != "3" {
			t.Fatalf("%s: %v", src, tb.Rows)
		}
	}
	if _, err := parseImportFile([]byte(`{"x":1}`), "json"); err == nil {
		t.Fatal("an object without tasks should be refused")
	}
}

func TestParseImportValues(t *testing.T) {
	for in, want := range map[string]int32{"high": 3, "Срочный": 4, "2": 2, "": 0} {
		if got, ok := parseImportPriority(in); !ok || got != want {
			t.Errorf("priority %q = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := parseImportPriority("whatever"); ok {
		t.Error("unknown priority should not parse")
	}
	d, ok := parseImportDate("05.11.2026")
	if !ok || !d.Equal(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dotted date = %v %v", d, ok)
	}
	d, ok = parseImportDate("2026-11-05T10:30:00+03:00")
	if !ok || d.UTC().Hour() != 7 {
		t.Errorf("rfc3339 = %v %v", d, ok)
	}
	if got := splitImportList("a, b;A ,, c"); len(got) != 3 {
		t.Errorf("splitList dedupes case-insensitively: %q", got)
	}
}
