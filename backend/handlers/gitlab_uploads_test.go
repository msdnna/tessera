package handlers

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestOutboundRefs covers what counts as a Tessera-hosted asset reference, and —
// the part that can silently corrupt user text — what must be left alone because
// markdown renders it as code.
func TestOutboundRefs(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"nothing", "just some text", nil},
		{"markdown image", "![shot](/api/uploads/abc-1.png)", []string{"/api/uploads/abc-1.png"}},
		{"html img", `<img src="/api/uploads/abc-1.png" width="200">`, []string{"/api/uploads/abc-1.png"}},
		{"file link", "[спека](/api/uploads/deadbeef.pdf) внизу", []string{"/api/uploads/deadbeef.pdf"}},
		{
			"proxy link",
			"![from gitlab](/api/gitlab/asset?ws=1&p=abc&sig=def)",
			[]string{"/api/gitlab/asset?ws=1&p=abc&sig=def"},
		},
		{
			"same asset twice is one ref",
			"![a](/api/uploads/x.png) и снова ![a](/api/uploads/x.png)",
			[]string{"/api/uploads/x.png"},
		},
		{
			"inside a fence is left alone",
			"текст\n```\n![a](/api/uploads/x.png)\n```\n",
			nil,
		},
		{
			"inside an inline span is left alone",
			"пиши `![a](/api/uploads/x.png)` вот так",
			nil,
		},
		{
			"outside the fence still counts",
			"```\n/api/uploads/in-code.png\n```\n![real](/api/uploads/real.png)",
			[]string{"/api/uploads/real.png"},
		},
		{
			"tilde fence",
			"~~~md\n![a](/api/uploads/x.png)\n~~~\n![b](/api/uploads/y.png)",
			[]string{"/api/uploads/y.png"},
		},
		{
			"unterminated fence swallows the rest",
			"![before](/api/uploads/a.png)\n```\n![after](/api/uploads/b.png)\n",
			[]string{"/api/uploads/a.png"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := outboundRefs(tc.body)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("outboundRefs(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// TestRewriteOutbound checks the substitution itself: a mirrored asset becomes the
// GitLab URL, one that couldn't be mirrored degrades to text instead of leaving a
// broken image, and code stays untouched.
func TestRewriteOutbound(t *testing.T) {
	resolved := map[string]string{
		"/api/uploads/ok.png":   "/uploads/sha1/ok.png",
		"/api/uploads/fail.png": "",
	}
	cases := []struct {
		name, body, want string
	}{
		{
			"image rewritten",
			"![схема](/api/uploads/ok.png)",
			"![схема](/uploads/sha1/ok.png)",
		},
		{
			"link rewritten, bang preserved as absent",
			"[спека](/api/uploads/ok.png)",
			"[спека](/uploads/sha1/ok.png)",
		},
		{
			"html src rewritten",
			`<img src="/api/uploads/ok.png">`,
			`<img src="/uploads/sha1/ok.png">`,
		},
		{
			"failed upload degrades to text",
			"![схема](/api/uploads/fail.png)",
			"_(схема — файл доступен в Tessera)_",
		},
		{
			"failed upload with no alt text",
			"![](/api/uploads/fail.png)",
			"_(вложение — файл доступен в Tessera)_",
		},
		{
			"code fence untouched",
			"```\n![a](/api/uploads/ok.png)\n```",
			"```\n![a](/api/uploads/ok.png)\n```",
		},
		{
			"both, in one body",
			"![a](/api/uploads/ok.png)\n\n![b](/api/uploads/fail.png)",
			"![a](/uploads/sha1/ok.png)\n\n_(b — файл доступен в Tessera)_",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteOutbound(tc.body, codeRanges(tc.body), resolved)
			if got != tc.want {
				t.Errorf("rewriteOutbound(%q)\n got %q\nwant %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestAssetRoundTrip is the regression this whole design exists for.
//
// title_desc conflict detection compares `tasks.description` against the issue body
// pulled back from GitLab. If mirroring assets on push were one-way, those strings
// would differ forever: every push would look like a divergence, re-upload every
// image, and pile duplicates into GitLab's store. Push-then-pull must therefore be
// the identity on the description.
func TestAssetRoundTrip(t *testing.T) {
	const desc = "Смотри ![схема](/api/uploads/abc-1.png) и файл [спека](/api/uploads/def-2.pdf).\n\n" +
		"```\n![не трогать](/api/uploads/in-code.png)\n```\n"

	outMap := map[string]string{
		"/api/uploads/abc-1.png": "/uploads/sha-a/abc-1.png",
		"/api/uploads/def-2.pdf": "/uploads/sha-b/def-2.pdf",
	}
	pushed := rewriteOutbound(desc, codeRanges(desc), outMap)
	if strings.Contains(pushed, "/api/uploads/abc-1.png") {
		t.Fatalf("pushed body still carries a Tessera URL: %q", pushed)
	}

	// The inverse map is what gitlab_uploads stores; anything else is a foreign
	// GitLab upload and gets a proxy URL.
	inMap := map[string]string{}
	for src, gl := range outMap {
		inMap[gl] = src
	}
	pulled := rewriteInbound(pushed,
		func(p string) (string, bool) { s, ok := inMap[p]; return s, ok },
		func(p string) string { return "/api/gitlab/asset?p=" + p },
	)
	if pulled != desc {
		t.Errorf("description did not round-trip:\n got %q\nwant %q", pulled, desc)
	}
}

// TestRewriteInboundForeignUpload guards the other half: an upload Tessera did NOT
// mirror still has to go through the signed proxy, or images authored in GitLab stop
// loading in Tessera.
func TestRewriteInboundForeignUpload(t *testing.T) {
	body := "![gl](/uploads/other/pic.png)"
	got := rewriteInbound(body,
		func(string) (string, bool) { return "", false },
		func(p string) string { return "/api/gitlab/asset?p=" + p },
	)
	want := "![gl](/api/gitlab/asset?p=/uploads/other/pic.png)"
	if got != want {
		t.Errorf("rewriteInbound = %q, want %q", got, want)
	}
}

// TestRewriteInboundLeavesTesseraURLs pins the guard alternative in uploadRe.
// "/api/uploads/x.png" ends in the literal "/uploads/x.png", so the plain pattern
// matched the tail and produced "/api/" + a proxy link — corrupting any Tessera URL
// that reached the inbound rewrite.
func TestRewriteInboundLeavesTesseraURLs(t *testing.T) {
	body := "![ours](/api/uploads/x.png) и ![theirs](/uploads/gl/y.png)"
	got := rewriteInbound(body,
		func(string) (string, bool) { return "", false },
		func(p string) string { return "PROXY(" + p + ")" },
	)
	want := "![ours](/api/uploads/x.png) и ![theirs](PROXY(/uploads/gl/y.png))"
	if got != want {
		t.Errorf("rewriteInbound = %q, want %q", got, want)
	}
}

// TestCodeRangesInlineSpans pins the inline-code scanner: a run of N backticks is
// closed only by a run of exactly N, and an unmatched opener is literal text.
func TestCodeRangesInlineSpans(t *testing.T) {
	cases := []struct {
		name, body string
		wantCode   bool // is the asset ref inside code?
	}{
		{"single backticks", "a `/api/uploads/x.png` b", true},
		{"double backticks", "a ``/api/uploads/x.png`` b", true},
		{"unmatched opener", "a ` /api/uploads/x.png b", false},
		{"closed before the ref", "a `code` /api/uploads/x.png", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refs := outboundRefs(tc.body)
			if got := len(refs) == 0; got != tc.wantCode {
				t.Errorf("body %q: inCode=%v, want %v (refs=%v)", tc.body, got, tc.wantCode, refs)
			}
		})
	}
}

// TestUnwrapAssetProxy: a proxy link Tessera minted unwraps back to the
// project-relative path (so a pushed body reuses the asset already in GitLab rather
// than duplicating it), and a forged or tampered one is refused.
func TestUnwrapAssetProxy(t *testing.T) {
	h := &API{assetKey: []byte("test-asset-key")}
	wsID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	const rel = "/uploads/secret42/pic.png"

	ref := h.assetProxyURL(wsID, rel)
	got, ok := h.unwrapAssetProxy(ref)
	if !ok || got != rel {
		t.Fatalf("unwrapAssetProxy(%q) = %q, %v; want %q, true", ref, got, ok, rel)
	}

	if _, ok := h.unwrapAssetProxy(ref + "x"); ok {
		t.Error("a tampered signature was accepted")
	}
	if _, ok := h.unwrapAssetProxy("/api/gitlab/asset?ws=" + wsID.String() + "&p=L2V0Yy9wYXNzd2Q&sig=deadbeef"); ok {
		t.Error("a forged link was accepted")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:            "512 Б",
		2048:           "2 КБ",
		3 * 1024 * 1024: "3.0 МБ",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
