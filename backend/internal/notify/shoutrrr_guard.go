package notify

import (
	"context"
	"fmt"
	"net/url"

	"tessera/internal/netguard"
)

// guardShoutrrrTarget applies the outbound-URL policy to a shoutrrr service URL.
//
// The "shoutrrr" channel type takes a whole service URL as its secret, so a
// member can express the same request WebhookSender would make — except
// shoutrrr sends it through http.DefaultClient, which our guarded dialer never
// touches, and the generic/smtp services will happily talk to whatever host the
// DSL names. Without this check, picking the channel type instead of the URL
// scheme is enough to aim the server at a sibling container or the cloud
// metadata endpoint.
//
// Only the host is judged, not the scheme: shoutrrr's schemes are service names
// ("generic+http", "telegram", "ntfy"), not transports, and a DSL with a
// placeholder host carries no target to protect — see netguard.ValidateHost.
func guardShoutrrrTarget(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("shoutrrr url: %w", err)
	}
	if err := netguard.ValidateHost(ctx, u.Hostname(), notifyAllowPrivateURLs()); err != nil {
		return fmt.Errorf("shoutrrr url: %w", err)
	}
	return nil
}
