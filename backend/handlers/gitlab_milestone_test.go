package handlers

import "testing"

func TestGidNumericID(t *testing.T) {
	cases := map[string]int64{
		"gid://gitlab/Milestone/42": 42,
		"gid://gitlab/Milestone/1":  1,
		"":                          0,
		"gid://gitlab/Milestone/":   0,
		"nonsense":                  0,
	}
	for in, want := range cases {
		if got := gidNumericID(in); got != want {
			t.Errorf("gidNumericID(%q) = %d, want %d", in, got, want)
		}
	}
}
