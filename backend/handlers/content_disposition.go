package handlers

import (
	"strings"
)

// Naming a downloaded file is the same job on every route that streams one, and
// it has exactly one correct answer, so it lives here rather than being spelled
// out again per handler. Four routes hand a file over — document export, a task
// attachment, a conference-chat attachment, a call recording — and before #2932
// only the first went through a shared builder; the other three wrote
// `fmt.Sprintf("attachment; filename=%q", name)` inline.
//
// Two separate things were wrong with that, and with the shared builder too.
//
// %q is Go quoting, not HTTP quoting. An HTTP field value is ASCII (RFC 9110
// §5.5) and a recipient reads a Content-Disposition quoted-string as ISO-8859-1
// (RFC 6266 §4.3), so the raw UTF-8 of a Russian filename arrives as mojibake
// rather than as the name — and %q escapes a newline to a literal backslash-n,
// which silently becomes part of the name. Non-ASCII names must travel in the
// filename* parameter instead; that is what it is for.
//
// The shared builder had filename*, but percent-encoded it with
// url.PathEscape, which encodes for a URL path segment and not for an
// RFC 8187 ext-value. The two disagree on exactly three characters — ":", "="
// and "@" — which PathEscape leaves raw and which are not attr-chars. One of
// them anywhere in a document title made the whole header unparseable, so the
// client got no name at all, not even from the ASCII fallback: a document called
// "Смета: Q4" exported as an unnamed file. Colons in titles are ordinary (the
// task this was found under is called "Анализ: качество кода и безопасная
// разработка"), so this was the common case, not the exotic one.

// attrChar reports whether b may appear unencoded in an RFC 8187 ext-value.
// This is the attr-char production verbatim; everything outside it is
// percent-encoded, and encoding a character that did not need it is harmless.
func attrChar(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9':
		return true
	}
	return strings.IndexByte("!#$&+-.^_`|~", b) >= 0
}

// extValue percent-encodes name for the filename* parameter, byte by byte —
// the encoding is defined over the UTF-8 octets, so multi-byte runes need no
// special handling.
func extValue(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(name); i++ {
		c := name[i]
		if attrChar(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// asciiFallback is the plain filename= value, for clients that never learned
// filename*. Anything it cannot carry becomes "_": non-ASCII has no meaning
// here, and a quote, a backslash or a control character would break out of the
// quoted-string rather than sit inside it.
func asciiFallback(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// contentDisposition builds the download header naming name. Both forms are
// emitted: the ASCII filename= for old clients, and filename* for everyone
// else, which wins wherever it is understood.
//
// A name with nothing usable in it yields a bare "attachment" — the route still
// has bytes to hand over, and an empty filename="" is worse than saying nothing
// and letting the client fall back to the URL.
func contentDisposition(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment"
	}
	ascii := asciiFallback(name)
	if strings.Trim(ascii, "_") == "" {
		// Nothing survived the ASCII pass, so there is no fallback worth sending;
		// filename* alone still names the file for anything from this decade.
		return "attachment; filename*=UTF-8''" + extValue(name)
	}
	return `attachment; filename="` + ascii + `"; filename*=UTF-8''` + extValue(name)
}
