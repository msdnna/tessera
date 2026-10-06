package notify

import (
	"net/http"
	"time"

	"tessera/internal/netguard"
)

// hardenedDefaultTimeout bounds a request made through the process-wide default
// client. shoutrrr has no timeout of its own and no context — without one, a
// service that accepts the connection and then stalls holds a delivery worker
// for as long as the peer keeps the socket open.
const hardenedDefaultTimeout = 15 * time.Second

// HardenDefaultClient points http.DefaultClient at an SSRF-guarded transport.
//
// Why a global: the shoutrrr channel type hands a member-supplied service URL to
// a third-party library that dials through http.DefaultClient for every one of
// its services (generic.Send uses DefaultClient.Do, the rest go through
// http.Post, which is DefaultClient.Post). We cannot pass it a client of ours,
// so the only place left to attach the guard is the client it reaches for.
//
// guardShoutrrrTarget already rejects a first hop that names an internal host,
// but that is a check-then-use: the library follows up to 10 redirects with the
// default policy, and every hop after the first was dialled with no check at
// all. A member could therefore point a channel at a public host they control
// and have it answer 302 → 169.254.169.254 (or a sibling container on the
// compose network) — exactly the request the guard exists to refuse. A name that
// resolves differently between the check and the connect (DNS rebinding) got
// through the same way. netguard's Control hook runs on the *resolved* address
// of every dial, redirects included, so installing it here closes both.
//
// The transport deliberately mirrors newWebhookClient rather than cloning
// http.DefaultTransport: guarded outbound to a member-supplied target does not
// go through HTTP_PROXY in this codebase, because a proxy resolves the target
// itself and an IP check on the proxy's own address protects nothing. Callers
// that legitimately need the proxy carry their own client (see fcmClient) — that
// is why NewFCMSender hands its token source an explicit one instead of letting
// x/oauth2 fall back to this client.
//
// Called once from main() before any delivery runs. Tests call it themselves;
// leaving it out of an init() keeps httptest-based tests in other packages on
// the stock client.
func HardenDefaultClient() {
	http.DefaultClient = &http.Client{
		Timeout:   hardenedDefaultTimeout,
		Transport: &http.Transport{DialContext: netguard.DialerFunc(notifyAllowPrivateURLs).DialContext},
	}
}
