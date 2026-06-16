// Package notify is the provider-neutral notification router: it decides which
// notifications reach which user-configured channels (the rule engine here) and
// carries the channel/message types the concrete senders implement. The DB-bound
// dispatch + outbox worker live in the handlers layer (they need queries and the
// secret sealer); this package stays pure and dependency-light so a second
// integration can reuse it, and so the rule engine is unit-testable in isolation.
//
// Channel transports are kept behind the Sender interface so the native MVP
// senders (telegram/webhook here, email via the server mailer in handlers) can be
// swapped for a unified library (e.g. shoutrrr) when the Phase B long tail of
// providers lands — without touching the router or the storage layer.
package notify

import "github.com/google/uuid"

// Event is the provider-neutral fact the router matches rules against. It is
// derived from a persisted notification; keep it small — adding a dimension here
// (priority, has_due, …) means teaching Matcher and the UI about it too.
type Event struct {
	Kind        string    // assigned | comment | mention | due_soon | …
	WorkspaceID uuid.UUID
}

// Matcher is a routing rule's condition (stored as JSONB on
// notification_routes.matcher). A zero matcher matches everything (catch-all).
type Matcher struct {
	Kinds       []string   `json:"kinds"`        // any-of; empty = any kind
	WorkspaceID *uuid.UUID `json:"workspace_id"` // nil = any workspace
}

// Matches reports whether an event satisfies the matcher.
func (m Matcher) Matches(ev Event) bool {
	if len(m.Kinds) > 0 {
		hit := false
		for _, k := range m.Kinds {
			if k == ev.Kind {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if m.WorkspaceID != nil && *m.WorkspaceID != ev.WorkspaceID {
		return false
	}
	return true
}

// RouteOptions are the per-rule knobs (stored as JSONB on
// notification_routes.options). Mute drops the event entirely when the rule
// matches (useful as an early "never notify me about X" rule).
type RouteOptions struct {
	Mute bool `json:"mute"`
}

// Channel is a decrypted delivery target handed to a Sender. Config holds
// non-secret settings (telegram chat_id, webhook url …); Secret holds the
// decrypted secret blob (telegram bot token …).
type Channel struct {
	Type   string
	Label  string
	Config map[string]any
	Secret map[string]string
}

// configString reads a string field from the channel config (numbers are
// coerced, so a JSON-numeric chat_id still works).
func (ch Channel) configString(key string) string {
	switch v := ch.Config[key].(type) {
	case string:
		return v
	case float64:
		// chat_id may arrive as a JSON number; render without a decimal point.
		return trimFloat(v)
	default:
		return ""
	}
}

// Message is the rendered notification to deliver.
type Message struct {
	Kind  string
	Title string
	Body  string
	Link  string // optional deep link to the originating task
}
