package handlers

import (
	"strings"
	"testing"
	"unicode"
)

// Refusals from the channel editor are read by a client that translates them —
// the web keeps the catalog in utils/errors.js, Android in Errors.kt — so they
// travel the wire as English sentinels like every other refusal on this API. A
// Russian sentence here is printed verbatim: the settings modal shows it under
// the form (chErr) and Android wraps it in UiText.Raw, so an English UI ends up
// with a Russian line it cannot translate (#2932).

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func TestValidateChannelSpeaksSentinels(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		cfg  map[string]any
		want string
	}{
		{"email without an address", "email", map[string]any{}, "email channel requires address"},
		{"telegram without a chat", "telegram", map[string]any{}, "telegram channel requires chat_id"},
		{"webhook without a url", "webhook", map[string]any{}, "webhook channel requires url"},
		{"device without an id", "device", map[string]any{}, "device channel requires device_id"},
		{"a type this build doesn't have", "carrier-pigeon", map[string]any{}, `unknown channel type: "carrier-pigeon"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateChannel(c.typ, c.cfg)
			if err == nil {
				t.Fatalf("validateChannel(%q) accepted an incomplete config", c.typ)
			}
			if err.Error() != c.want {
				t.Errorf("got  %q\nwant %q", err, c.want)
			}
		})
	}

	// Negative controls: a complete config passes, and shoutrrr carries its target
	// in the secret so an empty config is legitimate.
	for _, ok := range []struct {
		typ string
		cfg map[string]any
	}{
		{"email", map[string]any{"address": "a@b.c"}},
		{"telegram", map[string]any{"chat_id": float64(42)}},
		{"webhook", map[string]any{"url": "https://example.test/hook"}},
		{"device", map[string]any{"device_id": "dev-1"}},
		{"shoutrrr", map[string]any{}},
	} {
		if err := validateChannel(ok.typ, ok.cfg); err != nil {
			t.Errorf("validateChannel(%q, %v) = %v, want accepted", ok.typ, ok.cfg, err)
		}
	}
}

func TestValidateTemplateSpeaksSentinels(t *testing.T) {
	// The ceiling is named in the refusal so every client can word it in the
	// reader's own units — the same shape as the document and attachment caps.
	long := strings.Repeat("x", maxTemplateLen+1)
	err := validateTemplate(long)
	if err == nil {
		t.Fatal("an over-long template was accepted")
	}
	if want := "template exceeds 8192 characters"; err.Error() != want {
		t.Errorf("got  %q\nwant %q", err, want)
	}

	// A broken template keeps the Go parser's own words (they name the offending
	// action and have no translation), but the wrapper around them is a sentinel.
	err = validateTemplate("{{.Nope")
	if err == nil {
		t.Fatal("an unparseable template was accepted")
	}
	if !strings.HasPrefix(err.Error(), "template error: ") {
		t.Errorf("got %q, want a \"template error: \" prefix", err)
	}
	if hasCyrillic(err.Error()) {
		t.Errorf("got %q — Russian on the wire", err)
	}

	// Negative controls: empty keeps the built-in default, and a template using
	// the documented fields passes.
	if err := validateTemplate(""); err != nil {
		t.Errorf("empty template: %v", err)
	}
	if err := validateTemplate("#{{.TaskNumber}} {{.TaskTitle}}"); err != nil {
		t.Errorf("valid template: %v", err)
	}
}
