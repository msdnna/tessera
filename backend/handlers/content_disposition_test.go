package handlers

import (
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every download route names its file in the Content-Disposition header, and the
// only thing that makes that header useful is a client being able to read the
// name back out of it. So the assertion here is a round-trip through a real
// parser — Go's own mime.ParseMediaType — not a substring check on how the
// header happens to be spelled. The substring style is what let #2932 through:
// TestContentDispositionSurvivesCyrillic asserted "filename*=UTF-8''" was in
// there and never asked whether the result parses.

// dispositionCases are titles a Russian-speaking user actually types. The
// punctuation ones are the point: ":" in "Смета: Q4" is ordinary, and it is not
// an attr-char, so it has to be percent-encoded like any other non-attr-char.
var dispositionCases = []string{
	"Отчёт за квартал.pdf",
	"Смета: Q4.xlsx",
	"отчёт@июль.pdf",
	"a=b.txt",
	"План (v2).docx",
	`ev"il.pdf`,
	"back\\slash.txt",
	"перенос\nстроки.txt",
	"plain.pdf",
}

// TestContentDispositionRoundTrips states the whole contract. Two halves, and
// both matter:
//
//   - the header parses and gives the name back unchanged;
//   - the header is pure ASCII. An HTTP field value is ASCII (RFC 9110 §5.5)
//     and a recipient reads a Content-Disposition quoted-string as ISO-8859-1
//     (RFC 6266 §4.3), so raw UTF-8 bytes there do not come out as Cyrillic,
//     they come out as mojibake. Go's parser is lenient enough to hand those
//     bytes straight back, which is exactly why the round-trip alone would not
//     catch it — the name has to travel in filename* instead.
func TestContentDispositionRoundTrips(t *testing.T) {
	for _, name := range dispositionCases {
		hdr := contentDisposition(name)
		for i := 0; i < len(hdr); i++ {
			if hdr[i] > 127 {
				t.Errorf("%q: non-ASCII byte in header (%q)", name, hdr)
				break
			}
		}
		_, params, err := mime.ParseMediaType(hdr)
		if err != nil {
			t.Errorf("%q: header does not parse: %v (%q)", name, err, hdr)
			continue
		}
		if params["filename"] != name {
			t.Errorf("%q: got filename %q, header %q", name, params["filename"], hdr)
		}
	}
}

// A name that survives nothing still has to leave a usable header: the route has
// bytes to hand over either way.
func TestContentDispositionWithoutAName(t *testing.T) {
	for _, name := range []string{"", "   ", "\x00\x01"} {
		hdr := contentDisposition(name)
		typ, _, err := mime.ParseMediaType(hdr)
		if err != nil {
			t.Errorf("%q: header does not parse: %v (%q)", name, err, hdr)
			continue
		}
		if typ != "attachment" {
			t.Errorf("%q: got disposition %q, want attachment (%q)", name, typ, hdr)
		}
	}
}

// The ASCII fallback exists for clients that never learned filename*. It must
// stay a well-formed quoted-string, and it must not be the channel through which
// a title escapes into the header grammar.
func TestContentDispositionASCIIFallbackIsSafe(t *testing.T) {
	for _, name := range dispositionCases {
		hdr := contentDisposition(name)
		if n := strings.Count(hdr, `"`); n != 2 {
			t.Errorf("%q: %d quotes in header, want 2 (%q)", name, n, hdr)
		}
		if strings.ContainsAny(hdr, "\r\n") {
			t.Errorf("%q: a newline reached the header (%q)", name, hdr)
		}
	}
}

// setsDisposition matches a handler writing the header. The value is captured so
// the ratchet below can see what it was built from.
var setsDisposition = regexp.MustCompile(`(?s)Header\(\s*"Content-Disposition"\s*,\s*(.*?)\)\s*$`)

// TestEveryRouteNamesFilesThroughTheSharedBuilder is the ratchet. Three of the
// four download routes each grew their own fmt.Sprintf("…filename=%q…"), which
// is why three of them were wrong and the fourth was not — nothing tied them
// together, so fixing one fixed one. A unit test on contentDisposition cannot
// see a route that does not call it, so this reads the package source instead.
//
// A bare "attachment" (no name at all) stays allowed: ServeUpload and the
// document-asset and GitLab-asset routes use it deliberately, to stop a browser
// rendering a file inline, and they have no filename to offer.
func TestEveryRouteNamesFilesThroughTheSharedBuilder(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			m := setsDisposition.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			value := strings.TrimSpace(m[1])
			if value == `"attachment"` || strings.HasPrefix(value, "contentDisposition(") {
				continue
			}
			t.Errorf("%s: Content-Disposition built by hand: %s\n"+
				"\tuse contentDisposition(name) — see content_disposition.go", f, value)
		}
	}
}
