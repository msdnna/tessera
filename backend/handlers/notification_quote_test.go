package handlers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The in-app toast (#3117) quotes the comment under the sentence. The inline
// `excerpt` only exists for very short comments (shortCtx), so the quote rides
// separately — trimmed to one line and a bounded length.
func TestCommentNotificationsCarryQuote(t *testing.T) {
	long := strings.Repeat("слово ", 40)
	for name, m := range map[string]notifyMsg{
		"comment": msgComment("Анна", nil, "Проверила на светлой:\n  чип тега всё ещё бледный"),
		"reply":   msgThreadReply("Анна", nil, long),
		"mention": msgMention("Анна", nil, "@Мария посмотри"),
	} {
		q, _ := m.payload["quote"].(string)
		if q == "" || strings.Contains(q, "\n") || strings.Contains(q, "  ") {
			t.Fatalf("%s: quote %q, want one line", name, q)
		}
		if utf8.RuneCountInString(q) > quoteLimit+1 {
			t.Fatalf("%s: quote of %d runes, want at most %d", name, utf8.RuneCountInString(q), quoteLimit+1)
		}
	}
	if m := msgComment("Анна", nil, "   "); m.payload["quote"] != nil {
		t.Fatalf("an empty comment quotes nothing, got %v", m.payload["quote"])
	}
}
