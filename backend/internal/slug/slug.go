// Package slug builds short, human-readable, URL-safe identifiers from names —
// e.g. "Общие задачи" → "obshchie-zadachi" — so links read clearly instead of
// exposing UUIDs. Cyrillic is transliterated to Latin.
package slug

import "strings"

// cyrillic maps lowercase Russian letters to their Latin transliteration.
var cyrillic = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

// Make turns a display name into a slug: transliterated, lowercased, with runs
// of non-alphanumerics collapsed to single hyphens. Returns "" when nothing
// usable remains (caller should fall back, e.g. to a generic base).
func Make(name string) string {
	var b strings.Builder
	prevHyphen := false
	emit := func(s string) {
		b.WriteString(s)
		prevHyphen = false
	}
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			emit(string(r))
		case r >= 'а' && r <= 'я', r == 'ё':
			if t := cyrillic[r]; t != "" {
				emit(t)
			}
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// MakeUnique returns Make(name) (or `fallback` when empty) made unique against
// `taken` by appending -2, -3, … as needed; the chosen slug is added to `taken`.
func MakeUnique(name, fallback string, taken map[string]bool) string {
	base := Make(name)
	if base == "" {
		base = fallback
	}
	candidate := base
	for i := 2; taken[candidate]; i++ {
		candidate = base + "-" + itoa(i)
	}
	taken[candidate] = true
	return candidate
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
