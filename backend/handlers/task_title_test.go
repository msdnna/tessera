package handlers

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// The two titles the bug actually produced (#2813): Enter pressed with
		// the caret in the middle of the string.
		{"newline mid-string", "Окно Ч\nто нового при клике на версию в футере", "Окно Ч то нового при клике на версию в футере"},
		{"newline after trim-safe tail", "Перевод документации в С\nправочном центре", "Перевод документации в С правочном центре"},
		{"trailing newline", "Задача\n", "Задача"},
		{"pasted multi-line", "первая\nвторая\r\nтретья", "первая вторая третья"},
		{"tabs and doubled spaces", "а\tб  в", "а б в"},
		{"whitespace only", " \n\t ", ""},
		{"already clean", "Обычный заголовок", "Обычный заголовок"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTitle(tc.in); got != tc.want {
				t.Errorf("normalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
