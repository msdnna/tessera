package i18n

import "testing"

func TestPluralRussianCategories(t *testing.T) {
	// one (1, 21, 101) · few (2–4, 22–24) · many (0, 5–20, 25–30, 111–114).
	cases := []struct {
		n    int
		want string
	}{
		{0, "уведомлений"},
		{1, "уведомление"},
		{2, "уведомления"},
		{4, "уведомления"},
		{5, "уведомлений"},
		{11, "уведомлений"},
		{12, "уведомлений"},
		{14, "уведомлений"},
		{20, "уведомлений"},
		{21, "уведомление"},
		{22, "уведомления"},
		{25, "уведомлений"},
		{101, "уведомление"},
		{111, "уведомлений"},
		{114, "уведомлений"},
		{121, "уведомление"},
	}
	for _, c := range cases {
		got := Plural("ru", c.n, "уведомление", "уведомления", "уведомлений")
		if got != c.want {
			t.Errorf("Plural(ru, %d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestPluralEnglishUsesTwoForms(t *testing.T) {
	// English has no "few": the caller passes the plural twice, and the rule must
	// never reach for the middle form regardless of where n lands in the decades.
	for _, c := range []struct {
		n    int
		want string
	}{{0, "notifications"}, {1, "notification"}, {2, "notifications"}, {21, "notifications"}} {
		got := Plural("en", c.n, "notification", "notifications", "notifications")
		if got != c.want {
			t.Errorf("Plural(en, %d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestPluralFallsBackToDefaultLanguage(t *testing.T) {
	// Same normalisation as every other lookup: a tag we don't ship is worded in
	// the server's own language, agreement included.
	if got, want := Plural("de", 3, "час", "часа", "часов"), "часа"; got != want {
		t.Errorf("unsupported language: got %q, want %q", got, want)
	}
	if got, want := Plural("en-GB", 3, "hour", "hours", "hours"), "hours"; got != want {
		t.Errorf("region subtag: got %q, want %q", got, want)
	}
}

func TestPluralIgnoresSign(t *testing.T) {
	// A count is never negative in the product, but a rule that indexes on a raw
	// modulo would pick a form out of thin air if one ever were.
	if got, want := Plural("ru", -2, "день", "дня", "дней"), "дня"; got != want {
		t.Errorf("negative count: got %q, want %q", got, want)
	}
}
