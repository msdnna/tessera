package observability

import (
	"regexp"
	"sort"
	"strings"

	"github.com/getsentry/sentry-go"
)

// sensitiveQueryParams carry secrets in the URL and must never leave the process
// in the clear — not in the access log, and not in a Sentry event. The OAuth
// callback (?code=…&state=…) is the reason this list exists; the signed-asset
// routes add ?sig=… (a long-lived HMAC that grants the download), and ?token=…
// covers the email verification / recovery / invitation links. Listed by the
// literal name OAuth/OIDC and our own token flows actually use.
var sensitiveQueryParams = map[string]struct{}{
	"code":          {},
	"state":         {},
	"token":         {},
	"secret":        {},
	"sig":           {},
	"access_token":  {},
	"refresh_token": {},
	// GitLab's own name for a personal access token passed in the URL. We send
	// it that way only as the last resort of the avatar proxy (older web
	// routes accept nothing else), but once it is in a URL it can reach a log
	// through any error value that quotes the request.
	"private_token": {},
}

// redactURLParams matches a sensitive parameter and its value inside a larger
// string — an error message, a log line — where the URL is embedded rather than
// handed over as a bare query. The value runs to the next separator: '&' ends a
// parameter, and whitespace, a quote or a backslash ends the URL itself (Go
// quotes URLs in error messages as \"…\").
var redactURLParams = buildRedactURLParams()

func buildRedactURLParams() *regexp.Regexp {
	keys := make([]string, 0, len(sensitiveQueryParams))
	for k := range sensitiveQueryParams {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	// Longest first: alternation is leftmost-first in Go's engine, and a short
	// name must not win over a longer one that starts with it.
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return regexp.MustCompile(`([?&](?:` + strings.Join(keys, "|") + `)=)[^&\s"'\\]*`)
}

// RedactURLs replaces the value of every sensitive query parameter found
// anywhere in s. Use it on text that may quote a URL we built — most of all on
// an error from an HTTP client, since *url.Error stringifies the full request
// URL. The parameter name survives so the message stays diagnosable.
func RedactURLs(s string) string {
	if s == "" {
		return ""
	}
	return redactURLParams.ReplaceAllString(s, "${1}***")
}

// RedactQuery returns rawQuery with every sensitive parameter's value replaced
// by "***". Unrelated parameters are passed through untouched and in their
// original order so the log stays useful for debugging routing. Only the value
// is touched; a sensitive key with no '=' keeps its (value-less) form.
func RedactQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	var b strings.Builder
	for i, pair := range strings.Split(rawQuery, "&") {
		if i > 0 {
			b.WriteByte('&')
		}
		key, _, hasEq := strings.Cut(pair, "=")
		if _, secret := sensitiveQueryParams[key]; secret {
			b.WriteString(key)
			if hasEq {
				b.WriteString("=***")
			}
			continue
		}
		b.WriteString(pair)
	}
	return b.String()
}

// scrubEvent redacts secrets from an outbound Sentry event. It is wired as both
// BeforeSend and BeforeSendTransaction because the SDK fills Request.QueryString
// with the *raw* query unconditionally — SendDefaultPII (off here) gates only
// headers, cookies and the peer address, not the URL. With tracing on (the
// default sample rate is 1.0) that means every single request would otherwise
// ship its query string, so the transaction hook matters more than the error one.
//
// Request.URL is left alone: the SDK builds it from scheme+host+path only.
func scrubEvent(event *sentry.Event) *sentry.Event {
	if event != nil && event.Request != nil {
		event.Request.QueryString = RedactQuery(event.Request.QueryString)
	}
	return event
}
