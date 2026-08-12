package handlers

import (
	"encoding/json"

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

// truncErr renders an error for the last_error column, capped so a verbose
// upstream body doesn't bloat the row.
func truncErr(err error) string {
	s := err.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
