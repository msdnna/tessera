package slug

import "testing"

func TestMake(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Общие задачи", "obshchie-zadachi"},
		{"Hello, World!", "hello-world"},
		{"  Проект №1  ", "proekt-1"},
		{"щётка-ёж", "shchetka-ezh"},
		{"объём", "obem"}, // hard/soft signs drop
		{"---", ""},
		{"", ""},
		{"MiXeD Кейс 42", "mixed-keys-42"},
	}
	for _, c := range cases {
		if got := Make(c.in); got != c.want {
			t.Errorf("Make(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMakeUnique(t *testing.T) {
	taken := map[string]bool{}
	if got := MakeUnique("Доска", "board", taken); got != "doska" {
		t.Fatalf("first: %q", got)
	}
	if got := MakeUnique("Доска", "board", taken); got != "doska-2" {
		t.Fatalf("second: %q", got)
	}
	if got := MakeUnique("Доска", "board", taken); got != "doska-3" {
		t.Fatalf("third: %q", got)
	}
	// Empty name falls back.
	if got := MakeUnique("!!!", "board", taken); got != "board" {
		t.Fatalf("fallback: %q", got)
	}
	if !taken["doska-3"] || !taken["board"] {
		t.Fatalf("taken not updated: %v", taken)
	}
}
