package middleware

import "testing"

func TestRedactedQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"tab=board&col=todo", "tab=board&col=todo"}, // no secrets: verbatim
		{"code=abc&state=xyz", "code=***&state=***"}, // OAuth callback
		{"code=abc&state=xyz&tab=board", "code=***&state=***&tab=board"}, // mix, order kept
		{"access_token=s&refresh_token=r", "access_token=***&refresh_token=***"},
		{"token=t&secret=s", "token=***&secret=***"},
		{"code", "code"},           // sensitive key, no value -> left as-is
		{"code=", "code=***"},      // sensitive key, empty value -> still redacted
		{"q=a%20b&code=leak", "q=a%20b&code=***"}, // non-secret passed through verbatim
		{"code=a=b", "code=***"},   // value with '=' is discarded
		{"CODE=abc", "CODE=abc"},   // case-sensitive: real OAuth/OIDC params are lowercase
	}
	for _, c := range cases {
		if got := redactedQuery(c.in); got != c.want {
			t.Errorf("redactedQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
