package mail

import (
	"fmt"
	"strings"

	"tessera/internal/i18n"
)

// The transactional letter catalog (stage 6 of #2796). Email has no client to
// render it, so this is one of the two places where a translation legitimately
// lives on the server (the other is internal/notify).
//
// The list is deliberately closed: four letters, and they are the only ones the
// server sends. Growing it means adding a Kind here and its arm in every
// language — templates_test.go renders every kind × every i18n.Supported
// language, so a forgotten arm fails the tests rather than silently shipping
// Russian to an English reader.

// Kind identifies one transactional letter.
type Kind string

// The letters the server sends, and the whole of them.
const (
	KindVerify     Kind = "verify"      // confirm the address on sign-up
	KindReset      Kind = "reset"       // user-requested password recovery
	KindAdminReset Kind = "admin_reset" // recovery link created by an admin
	KindInvitation Kind = "invitation"  // workspace invitation
)

// Vars are the values a letter interpolates. Link is the target; TTLHours is the
// token's lifetime, worded per language by ttlPhrase (48 → "48 часов", 168 →
// "7 дней") so the letter can't drift from the constant that issues the token.
type Vars struct {
	Link     string
	TTLHours int
}

// letter is one subject/body pair. Placeholders are {link} and {ttl} — plain
// markers, not text/template: the catalog is fixed at compile time, so there is
// no author to report a parse error to.
type letter struct{ subject, body string }

var catalog = map[string]map[Kind]letter{
	"ru": {
		KindVerify: {
			subject: "Подтверждение почты — Tessera",
			body:    "Подтвердите адрес, перейдя по ссылке:\n\n{link}\n\nСсылка действует {ttl}.",
		},
		KindReset: {
			subject: "Восстановление доступа — Tessera",
			body: "Вы запросили восстановление доступа к аккаунту Tessera. Чтобы продолжить, перейдите по ссылке:\n\n{link}\n\n" +
				"Ссылка действует {ttl}. Если вы не запрашивали восстановление — просто проигнорируйте это письмо.",
		},
		KindAdminReset: {
			subject: "Восстановление доступа — Tessera",
			body:    "Администратор создал ссылку для восстановления доступа. Перейдите по ссылке (действует {ttl}): {link}",
		},
		KindInvitation: {
			subject: "Приглашение в пространство — Tessera",
			body:    "Вас пригласили в рабочее пространство Tessera. Присоединиться:\n\n{link}\n\nСсылка действует {ttl}.",
		},
	},
	"en": {
		KindVerify: {
			subject: "Confirm your email — Tessera",
			body:    "Confirm your address by following this link:\n\n{link}\n\nThe link is valid for {ttl}.",
		},
		KindReset: {
			subject: "Password recovery — Tessera",
			body: "You asked to recover access to your Tessera account. To continue, follow this link:\n\n{link}\n\n" +
				"The link is valid for {ttl}. If you did not request recovery, just ignore this email.",
		},
		KindAdminReset: {
			subject: "Password recovery — Tessera",
			body:    "An administrator created a recovery link for you. Follow it (valid for {ttl}): {link}",
		},
		KindInvitation: {
			subject: "Workspace invitation — Tessera",
			body:    "You have been invited to a Tessera workspace. Join:\n\n{link}\n\nThe link is valid for {ttl}.",
		},
	},
}

// Compose renders a letter in the recipient's language. lang is the recipient's
// stored preference — never the Accept-Language of the request that triggered
// the send, since a letter is often the by-product of *another* user's action.
// An unsupported language, or a language whose catalog lacks the kind, falls
// back to i18n.Default.
func Compose(kind Kind, lang string, v Vars) (subject, body string) {
	lang = i18n.Normalize(lang)
	l, ok := catalog[lang][kind]
	if !ok {
		lang = i18n.Default
		if l, ok = catalog[lang][kind]; !ok {
			// An unknown kind can only be a programming error; still, never send
			// an empty letter — the link is the whole point of every one of them.
			return "Tessera", v.Link
		}
	}
	r := strings.NewReplacer("{link}", v.Link, "{ttl}", ttlPhrase(v.TTLHours, lang))
	return r.Replace(l.subject), r.Replace(l.body)
}

// ttlPhrase words a token lifetime given in hours: whole days read as days
// ("7 дней"), anything else as hours ("48 часов").
func ttlPhrase(hours int, lang string) string {
	if hours <= 0 {
		hours = 1
	}
	n, unit := hours, "hour"
	if hours%24 == 0 {
		n, unit = hours/24, "day"
	}
	if lang == "en" {
		if n == 1 {
			return fmt.Sprintf("1 %s", unit)
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	if unit == "day" {
		return fmt.Sprintf("%d %s", n, pluralRU(n, "день", "дня", "дней"))
	}
	return fmt.Sprintf("%d %s", n, pluralRU(n, "час", "часа", "часов"))
}

// pluralRU picks the Russian plural form for n (1 час / 2 часа / 5 часов).
func pluralRU(n int, one, few, many string) string {
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
