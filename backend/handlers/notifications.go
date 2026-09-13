package handlers

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Shared helpers for the notification subsystem and its neighbours. The subsystem
// itself is split by responsibility: notification_channels.go (channels),
// notification_routes.go (routing rules), notification_delivery.go (senders +
// outbox worker), notification_scanner.go (due/reminder sweep) and
// notification_prefs.go (per-user scheduling prefs).

// orJSONObj returns raw, or "{}" when it's empty/null (JSONB columns never store
// a bare null this way, but request bodies may omit the field).
func orJSONObj(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}")
	}
	return raw
}

// orIDs normalises a nil id slice to empty (for the uuid[] column).
func orIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// maxLastErrorBytes caps what goes into a last_error column, so a verbose
// upstream body doesn't bloat the row. Bytes, because that is what the column
// costs.
const maxLastErrorBytes = 500

// truncErr renders an error for the last_error column, capped to
// maxLastErrorBytes without ever cutting through a character.
//
// The cap counts bytes but the text is Russian as often as not — a transport
// error carries the channel's URL, and an unescaped query string reaches
// url.Error verbatim — so a plain s[:500] can land between the two bytes of a
// letter. Postgres refuses the resulting sequence outright ("invalid byte
// sequence for encoding UTF8"), settleDelivery hands that refusal to soft(),
// and the row keeps the 'sending' status it was claimed with: the outcome is
// never recorded, attempts never retires it, and every later drain re-sends the
// notification through RequeueStaleDeliveries. Same rule as truncateRunes and
// truncForJournal, which say it for display text; here it decides whether the
// write lands at all.
func truncErr(err error) string {
	s := strings.ToValidUTF8(err.Error(), "�") // an upstream body may not be UTF-8 at all
	if len(s) <= maxLastErrorBytes {
		return s
	}
	s = s[:maxLastErrorBytes]
	// Drop the partial character the cut may have left behind (at most 3 bytes).
	for len(s) > 0 {
		if r, size := utf8.DecodeLastRuneInString(s); r != utf8.RuneError || size > 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
