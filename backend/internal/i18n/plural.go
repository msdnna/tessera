package i18n

// Plural picks the form of a noun that agrees with n, in lang.
//
// It exists because the server composes a handful of sentences that carry a count
// ("Сводка — 3 уведомления", "Ссылка действует 2 часа"), and those sentences leave
// the app — there is no client to do the agreement for them. Every caller passes
// the three CLDR forms of *one* language's noun, already chosen from its own
// catalog; this only decides which of the three to use.
//
// Russian needs all three: one (1, 21, 101) · few (2–4, 22–24) · many (0, 5–20,
// 11–14, 25–30). Two-form languages pass the plural twice and never reach few.
// An unsupported tag is normalised the same way every other lookup normalises it,
// so the fallback language's agreement is the fallback agreement.
func Plural(lang string, n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	if Normalize(lang) != "ru" {
		if n == 1 {
			return one
		}
		return many
	}
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
