// Package i18n holds the server's language vocabulary: which UI languages exist
// and how a stored language tag is normalised to one of them.
//
// The server localises exactly one class of output — messages that leave the app
// and have no client to render them (transactional email, FCM push, external
// channel deliveries). Everything shown inside a client is translated by that
// client from the structured payload (#2801). Keep it that way: this package is
// not a general-purpose translation layer for the backend.
package i18n

import "strings"

// Default is the fallback language: the project's own language, and what every
// pre-i18n row and unconfigured account is treated as.
const Default = "ru"

// Supported lists the languages the catalogs actually carry. Adding one means
// adding its arm to every catalog (internal/mail, internal/notify) — the tests
// there iterate this list, so a missing arm fails the build's test run.
var Supported = []string{"ru", "en"}

// Normalize maps a stored preference (user_preferences.language) or any language
// tag to a supported language, falling back to Default. It accepts region
// subtags ("en-US" → "en") and is case/space tolerant, because the value is a
// free-form text column, not an enum.
func Normalize(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	for _, s := range Supported {
		if l == s {
			return l
		}
	}
	return Default
}
