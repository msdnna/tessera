package handlers

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/i18n"
)

// Which language an outbound message is written in (stage 6 of #2796).
//
// The rule is always the same: it's the *recipient's* stored preference
// (user_preferences.language), never the Accept-Language of the request that
// triggered the send — a verification letter is the sender's own action, but an
// invitation, a notification or an admin-issued reset link is written by one
// person and read by another.

// userLang resolves the language of a known user. Any miss (no preferences row,
// empty column, unknown tag) lands on i18n.Default.
func userLang(ctx context.Context, q *db.Queries, userID uuid.UUID) string {
	p, err := q.GetUserPreferences(ctx, userID)
	if err != nil {
		return i18n.Default
	}
	return i18n.Normalize(p.Language)
}

// emailLang resolves the language for an address that may not belong to an
// account yet — a workspace invitation is the only such case. An invitee we
// already know reads it in their own language; a stranger gets i18n.Default,
// since the alternative (the inviter's language) is a guess about a third party.
func emailLang(ctx context.Context, q *db.Queries, email string) string {
	u, err := q.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return i18n.Default
	}
	return userLang(ctx, q, u.ID)
}

func (h *API) userLang(ctx context.Context, userID uuid.UUID) string {
	return userLang(ctx, h.q, userID)
}

func (h *AuthHandler) userLang(ctx context.Context, userID uuid.UUID) string {
	return userLang(ctx, h.q, userID)
}
