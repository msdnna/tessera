package i18n

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"ru":      "ru",
		"en":      "en",
		"EN":      "en",
		" en ":    "en",
		"en-US":   "en",
		"en_GB":   "en",
		"ru-RU":   "ru",
		"":        Default, // account never opened the settings
		"de":      Default, // a language we don't carry
		"english": Default, // not a tag at all
		"-en":     Default, // leading separator: not a subtag we accept
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSupportedContainsDefault(t *testing.T) {
	for _, l := range Supported {
		if l == Default {
			return
		}
	}
	t.Fatalf("Default %q missing from Supported %v — every fallback would break", Default, Supported)
}
