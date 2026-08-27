package mail

import (
	"strings"
	"testing"

	"tessera/internal/i18n"
)

// allKinds is every letter the server sends. Adding a Kind without adding it
// here is the one way this file stops covering the catalog — keep them together.
var allKinds = []Kind{KindVerify, KindReset, KindAdminReset, KindInvitation}

func TestComposeCoversEveryKindInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Supported {
		for _, kind := range allKinds {
			subject, body := Compose(kind, lang, Vars{Link: "https://tessera.example/x?token=abc", TTLHours: 48})
			if subject == "" || body == "" {
				t.Errorf("%s/%s: empty letter", lang, kind)
				continue
			}
			if !strings.Contains(body, "https://tessera.example/x?token=abc") {
				t.Errorf("%s/%s: body drops the link:\n%s", lang, kind, body)
			}
			for _, ph := range []string{"{link}", "{ttl}"} {
				if strings.Contains(subject+body, ph) {
					t.Errorf("%s/%s: placeholder %s left unrendered:\n%s", lang, kind, ph, body)
				}
			}
		}
	}
}

func TestComposeLanguagesDiffer(t *testing.T) {
	// A translation that silently falls through to Russian is the exact bug this
	// stage exists to prevent, so assert the two really are different letters.
	for _, kind := range allKinds {
		ruSubj, ruBody := Compose(kind, "ru", Vars{Link: "L", TTLHours: 1})
		enSubj, enBody := Compose(kind, "en", Vars{Link: "L", TTLHours: 1})
		if ruSubj == enSubj || ruBody == enBody {
			t.Errorf("%s: en letter identical to ru (subject %q/%q)", kind, ruSubj, enSubj)
		}
	}
}

func TestComposeFallsBackToDefaultLanguage(t *testing.T) {
	want, wantBody := Compose(KindVerify, i18n.Default, Vars{Link: "L", TTLHours: 48})
	for _, lang := range []string{"", "de", "klingon", "  "} {
		got, gotBody := Compose(KindVerify, lang, Vars{Link: "L", TTLHours: 48})
		if got != want || gotBody != wantBody {
			t.Errorf("lang %q: got %q, want the %s letter %q", lang, got, i18n.Default, want)
		}
	}
}

func TestComposeUnknownKindStillCarriesTheLink(t *testing.T) {
	subject, body := Compose(Kind("no-such-letter"), "en", Vars{Link: "https://tessera.example/x"})
	if subject == "" || !strings.Contains(body, "https://tessera.example/x") {
		t.Fatalf("unknown kind must still deliver the link, got %q / %q", subject, body)
	}
}

func TestTTLPhrase(t *testing.T) {
	cases := []struct {
		hours int
		lang  string
		want  string
	}{
		{1, "ru", "1 час"},
		{2, "ru", "2 часа"},
		{48, "ru", "2 дня"},
		{5, "ru", "5 часов"},
		{11, "ru", "11 часов"},
		{21, "ru", "21 час"},
		{168, "ru", "7 дней"},
		{1, "en", "1 hour"},
		{48, "en", "2 days"},
		{5, "en", "5 hours"},
		{168, "en", "7 days"},
		{0, "ru", "1 час"}, // never word a link as already expired
	}
	for _, c := range cases {
		if got := ttlPhrase(c.hours, c.lang); got != c.want {
			t.Errorf("ttlPhrase(%d, %q) = %q, want %q", c.hours, c.lang, got, c.want)
		}
	}
}

func TestComposeWordsTheLifetime(t *testing.T) {
	_, ru := Compose(KindInvitation, "ru", Vars{Link: "L", TTLHours: 168})
	if !strings.Contains(ru, "7 дней") {
		t.Errorf("invitation must say 7 дней, got:\n%s", ru)
	}
	_, en := Compose(KindVerify, "en", Vars{Link: "L", TTLHours: 48})
	if !strings.Contains(en, "2 days") {
		t.Errorf("verification must say 2 days, got:\n%s", en)
	}
}
