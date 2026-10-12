// Package netguard constrains outbound HTTP requests so an attacker-controlled
// URL (a webhook channel, a GitLab base URL) cannot turn the server into a proxy
// for the internal network — SSRF defense for self-hosted Tessera.
//
// Two layers, both needed:
//
//   - ValidateURL rejects obviously bad input up front (non-http(s) schemes,
//     embedded userinfo, no host, and — when private targets are disallowed — a
//     literal private/loopback IP). This is for a clear, early error to the user
//     rather than a cryptic dial failure.
//
//   - Dialer installs a DialContext.Control hook that re-checks the *resolved*
//     IP right before the TCP connect. This is the real backstop: it runs after
//     DNS resolution, so it defeats DNS-rebinding and HTTP redirects that land
//     on an internal host — which a URL-string check can never see.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"time"
)

// ErrBlocked is returned by the dial Control hook (and surfaces from any
// request whose dial it cancels) when the destination is not a public address
// and private targets are disallowed.
var ErrBlocked = errors.New("netguard: connection to a non-public address is blocked")

// ControlFunc returns a net.Dialer.Control callback that rejects non-public
// destination IPs, resolving the policy via resolve on every dial. Resolving
// per-dial (rather than capturing a bool) lets a policy kept in an env var take
// effect on the next connection — and lets a test toggle it — without rebuilding
// the HTTP client. The check runs after DNS resolution (Control fires on the
// resolved IP:port), so it also blocks DNS-rebinding and redirects to an
// internal host.
func ControlFunc(resolve func() bool) func(network, address string, c syscall.RawConn) error {
	return func(_ string, address string, _ syscall.RawConn) error {
		if resolve() {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil // not host:port (e.g. a unix socket path) — nothing to inspect
		}
		if ip := net.ParseIP(host); ip != nil && !allowedPublic(ip) {
			return ErrBlocked
		}
		return nil
	}
}

// Control returns a dial Control callback with a fixed policy: nil (no
// restriction) when allowPrivate is true, otherwise the guarded check. Callers
// set the result verbatim on net.Dialer.Control.
func Control(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	if allowPrivate {
		return nil
	}
	return ControlFunc(func() bool { return false })
}

// Dialer returns a net.Dialer whose Control hook blocks non-public destinations
// when allowPrivate is false. Callers set their own Timeout/KeepAlive on it.
func Dialer(allowPrivate bool) *net.Dialer {
	return &net.Dialer{Control: Control(allowPrivate)}
}

// DialerFunc is Dialer with a per-dial policy resolver — for a policy kept in an
// env var, so toggling it (in a test, or by an operator) affects the next
// connection without rebuilding the client.
func DialerFunc(resolve func() bool) *net.Dialer {
	return &net.Dialer{Control: ControlFunc(resolve)}
}

// ValidateURL parses raw and enforces a safe outbound shape: an http or https
// URL with a host and no embedded userinfo. When allowPrivate is false it also
// rejects a host that is a literal private/loopback IP — for a clear, early
// error instead of a dial-time failure. A *hostname* that resolves to a private
// IP is not caught here; the dial-time Control hook is the backstop for that.
func ValidateURL(raw string, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL scheme %q is not allowed (only http/https)", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("userinfo (user:pass@) is not allowed in the URL")
	}
	if u.Host == "" {
		return nil, errors.New("URL is missing a host")
	}
	if !allowPrivate {
		// Hostname() strips the port and IPv6 brackets, so ParseIP sees a bare IP.
		if h := u.Hostname(); h != "" {
			if ip := net.ParseIP(h); ip != nil && !allowedPublic(ip) {
				return nil, fmt.Errorf("URL host %s is not a public address", h)
			}
		}
	}
	return u, nil
}

// lookupTimeout bounds the name resolution ValidateHost does. A variable rather
// than a constant so a test can shorten it.
var lookupTimeout = 3 * time.Second

// ValidateHost enforces the public-address policy for a host that is about to
// be dialled by a client we cannot attach a Control hook to — a third-party
// sender that builds its own http.Client (shoutrrr) rather than taking ours. A
// literal IP is judged directly; a name is resolved and rejected if any address
// behind it is non-public, which is what catches "localhost" and an internal
// service name on the compose network.
//
// This is strictly weaker than the dial-time hook: a name that resolves
// differently between this check and the connect (DNS rebinding) still gets
// through, and a name that does not resolve at all is allowed — there is no
// internal target to protect, and several service DSLs carry a placeholder host
// ("telegram") that was never meant to be looked up. Prefer Dialer wherever the
// client is ours.
//
// The lookup is capped by lookupTimeout: a placeholder host is a name no
// resolver can answer, and a resolver that takes its time saying so must not
// hold the delivery worker.
func ValidateHost(ctx context.Context, host string, allowPrivate bool) error {
	if allowPrivate || host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if !allowedPublic(ip) {
			return fmt.Errorf("host %s is not a public address", host)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil // unresolvable — the dial cannot reach anything either
	}
	for _, a := range addrs {
		if !allowedPublic(a.IP) {
			return fmt.Errorf("host %s resolves to %s, which is not a public address", host, a.IP)
		}
	}
	return nil
}

// allowedPublic reports whether ip is a routable public address: not
// unspecified, loopback, private (RFC 1918 / IPv6 ULA), link-local, carrier-grade
// NAT, or multicast. The cloud metadata endpoint 169.254.169.254 is link-local,
// so it is blocked here; 100.64.0.0/10 (CGNAT) is blocked explicitly because Go's
// IsPrivate does not cover it and it is a classic SSRF bypass.
func allowedPublic(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if isCGNAT(ip) {
		return false
	}
	return ip.IsGlobalUnicast()
}

// isCGNAT reports whether ip is in the RFC 6598 carrier-grade NAT shared
// address space 100.64.0.0/10.
func isCGNAT(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}
