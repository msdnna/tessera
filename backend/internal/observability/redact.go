package observability

import (
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
