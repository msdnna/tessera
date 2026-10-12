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
// The list is deliberately closed: these letters are the only ones the server
// sends. Growing it means adding a Kind here and its arm in every
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
	// Changing the account's address (#3058): the code goes to the new address,
	// the notice to the old one, and an address already taken by another account
	// gets a warning instead of a code — the requester is told the same thing
	// either way, so the form cannot be used to probe who is registered.
	KindEmailChange  Kind = "email_change"
	KindEmailChanged Kind = "email_changed"
	KindEmailInUse   Kind = "email_in_use"
)

// Vars are the values a letter interpolates. Link is the target; TTLHours is the
// token's lifetime, worded per language by ttlPhrase (48 → "48 часов", 168 →
// "7 дней") so the letter can't drift from the constant that issues the token.
//
// Code is a one-time confirmation code and CodeMinutes its lifetime (#3057);
// Email is an address the letter names (the new one, in the notice to the old).
type Vars struct {
	Link        string
	TTLHours    int
	Code        string
	CodeMinutes int
	Email       string
}

// letter is one subject/body pair. Placeholders are {link}, {ttl}, {code},
// {codettl} and {email} — plain
// markers, not text/template: the catalog is fixed at compile time, so there is
// no author to report a parse error to.
type letter struct{ subject, body string }

var catalog = map[string]map[Kind]letter{
	"ru": {
		KindVerify: {
			subject: "Код подтверждения {code} — Tessera",
			body: "Код подтверждения почты: {code}\nКод действует {codettl}.\n\n" +
				"Или подтвердите адрес, перейдя по ссылке:\n\n{link}\n\nСсылка действует {ttl}.",
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
		KindEmailChange: {
			subject: "Код для смены почты {code} — Tessera",
			body: "Чтобы привязать этот адрес к аккаунту Tessera, введите код: {code}\nКод действует {codettl}.\n\n" +
				"Страница для ввода кода: {link}\n\nЕсли вы не меняли адрес — просто проигнорируйте это письмо.",
		},
		KindEmailChanged: {
			subject: "Почта аккаунта изменена — Tessera",
			body: "Адрес вашего аккаунта Tessera изменён на {email}. Письма теперь приходят туда.\n\n" +
				"Если это были не вы — срочно восстановите доступ: {link}",
		},
		KindEmailInUse: {
			subject: "Попытка привязать ваш адрес — Tessera",
			body: "Кто-то пытался привязать этот адрес к другому аккаунту Tessera. Адрес уже принадлежит вашему " +
				"аккаунту, поэтому ничего не изменилось.\n\nВойти: {link}\n\nЕсли это были не вы — просто проигнорируйте это письмо.",
		},
	},
	"en": {
		KindVerify: {
			subject: "Confirmation code {code} — Tessera",
			body: "Your email confirmation code: {code}\nThe code is valid for {codettl}.\n\n" +
				"Or confirm your address by following this link:\n\n{link}\n\nThe link is valid for {ttl}.",
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
		KindEmailChange: {
			subject: "Email change code {code} — Tessera",
			body: "To attach this address to a Tessera account, enter the code: {code}\nThe code is valid for {codettl}.\n\n" +
				"Where to enter it: {link}\n\nIf you did not change your address, just ignore this email.",
		},
		KindEmailChanged: {
			subject: "Your account email was changed — Tessera",
			body: "The address of your Tessera account was changed to {email}. Emails now go there.\n\n" +
				"If it wasn't you, recover access right away: {link}",
		},
		KindEmailInUse: {
			subject: "Someone tried to use your address — Tessera",
			body: "Someone tried to attach this address to another Tessera account. The address already belongs to " +
				"your account, so nothing changed.\n\nSign in: {link}\n\nIf it wasn't you, just ignore this email.",
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
	r := strings.NewReplacer(
		"{link}", v.Link, "{ttl}", ttlPhrase(v.TTLHours, lang),
		"{code}", v.Code, "{codettl}", minutesPhrase(v.CodeMinutes, lang), "{email}", v.Email,
	)
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
		return fmt.Sprintf("%d %s", n, i18n.Plural(lang, n, "день", "дня", "дней"))
	}
	return fmt.Sprintf("%d %s", n, i18n.Plural(lang, n, "час", "часа", "часов"))
}

// minutesPhrase words a code lifetime given in minutes ("15 минут", "15 minutes").
func minutesPhrase(minutes int, lang string) string {
	if minutes <= 0 {
		minutes = 1
	}
	if lang == "en" {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	return fmt.Sprintf("%d %s", minutes, i18n.Plural(lang, minutes, "минуту", "минуты", "минут"))
}
