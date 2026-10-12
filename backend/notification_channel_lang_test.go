// A notification channel is configured through two rehearsals: the editor's live
// preview and the "send a test" probe. Both stand in for a real delivery — and a
// real delivery leaves the app written in the *recipient's* language
// (handlers.templateData → notify.Title/Sentence). The rehearsals used a hardcoded
// Russian sample instead, so an English reader previewed a Russian message,
// received a Russian test, and only then started getting English notifications.
// Its refusals had the same problem from the other side: hardcoded Russian
// sentences where the rest of the API sends English sentinels for the client to
// translate (#2932).
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode"
)

func containsCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// setLang rewrites the user's preferences (a full-replace endpoint, so every
// field has to travel) to the given UI language.
func setLang(t *testing.T, c *client, lang string) {
	t.Helper()
	c.expect(t, c.put("/users/me/preferences", map[string]any{
		"language": lang, "time_format": "24h", "date_format": "dd.MM.yyyy",
		"week_start": 1, "theme": "light", "accent": "default",
	}), http.StatusOK)
}

// webhookSink captures the JSON envelope a webhook channel receives, which is
// exactly what the probe delivered: kind, title and the rendered body.
type webhookSink struct {
	mu   sync.Mutex
	last map[string]any
	srv  *httptest.Server
}

func newWebhookSink(t *testing.T) *webhookSink {
	t.Helper()
	s := &webhookSink{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.last = body
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *webhookSink) delivered(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		t.Fatal("the probe delivered nothing to the webhook")
	}
	return s.last
}

func TestChannelProbeSpeaksTheReadersLanguage(t *testing.T) {
	t.Parallel()

	probe := func(t *testing.T, lang string) map[string]any {
		t.Helper()
		c := signup(t)
		setLang(t, c, lang)
		sink := newWebhookSink(t)
		ch := c.expect(t, c.post("/notification-channels", map[string]any{
			"type": "webhook", "label": "sink", "config": map[string]any{"url": sink.srv.URL},
		}), http.StatusCreated)
		res := c.expect(t, c.post("/notification-channels/"+ch["id"].(string)+"/test", nil), http.StatusOK)
		if res["ok"] != true {
			t.Fatalf("probe refused: %v", res)
		}
		return sink.delivered(t)
	}

	t.Run("english reader gets an english test", func(t *testing.T) {
		got := probe(t, "en")
		if title, _ := got["title"].(string); title != "Channel test" {
			t.Errorf("title = %q, want %q", title, "Channel test")
		}
		text, _ := got["text"].(string)
		if containsCyrillic(text) {
			t.Errorf("body = %q — Russian delivered to an English reader", text)
		}
		if !strings.Contains(text, "assigned task #42") {
			t.Errorf("body = %q, want the catalog's assigned sentence", text)
		}
	})

	// Negative control: the default language is untouched by the fix.
	t.Run("russian reader still gets the russian test", func(t *testing.T) {
		got := probe(t, "ru")
		if title, _ := got["title"].(string); title != "Проверка канала" {
			t.Errorf("title = %q, want %q", title, "Проверка канала")
		}
		text, _ := got["text"].(string)
		if !strings.Contains(text, "назначил вам задачу #42") {
			t.Errorf("body = %q, want the russian assigned sentence", text)
		}
	})
}

func TestTemplatePreviewSpeaksTheReadersLanguage(t *testing.T) {
	t.Parallel()

	preview := func(t *testing.T, lang, tmpl string) map[string]any {
		t.Helper()
		c := signup(t)
		setLang(t, c, lang)
		return c.expect(t, c.post("/notification-template-preview", map[string]any{"template": tmpl}), http.StatusOK)
	}

	// The empty template is what the editor shows first: the built-in default,
	// rendered against the sample.
	en := preview(t, "en", "")
	if en["ok"] != true {
		t.Fatalf("en preview: %v", en)
	}
	text, _ := en["text"].(string)
	if containsCyrillic(text) {
		t.Errorf("en preview = %q — Russian sample in an English editor", text)
	}
	if !strings.Contains(text, "assigned task #42") {
		t.Errorf("en preview = %q, want the catalog's assigned sentence", text)
	}

	// Every documented field follows the reader, not just the sentence: the
	// workspace name is a server-seeded one captioned for the reader (#2800).
	fields := preview(t, "en", "{{.Title}}|{{.TaskTitle}}|{{.Actor}}|{{.Workspace}}")
	ftext, _ := fields["text"].(string)
	if containsCyrillic(ftext) {
		t.Errorf("en fields = %q — Russian leaked through a custom template", ftext)
	}
	if !strings.Contains(ftext, "Personal space") {
		t.Errorf("en fields = %q, want the captioned personal workspace", ftext)
	}

	// Negative control: the Russian preview is unchanged.
	ru := preview(t, "ru", "")
	rtext, _ := ru["text"].(string)
	if !strings.Contains(rtext, "назначил вам задачу #42") {
		t.Errorf("ru preview = %q, want the russian sample", rtext)
	}
}

func TestChannelRefusalsTravelAsSentinels(t *testing.T) {
	t.Parallel()
	c := signup(t)

	errOf := func(t *testing.T, r resp, want int) string {
		t.Helper()
		if r.Status != want {
			t.Fatalf("status %d, want %d\n%s", r.Status, want, r.Body)
		}
		var out struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(r.Body, &out); err != nil {
			t.Fatalf("decode: %v\n%s", err, r.Body)
		}
		if containsCyrillic(out.Error) {
			t.Errorf("refusal %q is Russian on the wire — the client cannot translate it", out.Error)
		}
		return out.Error
	}

	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{
			name: "a channel type this build doesn't have",
			body: map[string]any{"type": "carrier-pigeon", "config": map[string]any{}},
			want: `unknown channel type: "carrier-pigeon"`,
		},
		{
			name: "email without an address",
			body: map[string]any{"type": "email", "config": map[string]any{}},
			want: "email channel requires address",
		},
		{
			name: "telegram without its bot token",
			body: map[string]any{"type": "telegram", "config": map[string]any{"chat_id": "42"}},
			want: "telegram channel requires bot_token",
		},
		{
			name: "a template longer than the ceiling",
			body: map[string]any{
				"type": "webhook", "config": map[string]any{"url": "https://example.test/hook"},
				"template": strings.Repeat("x", 8193),
			},
			want: "template exceeds 8192 characters",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errOf(t, c.post("/notification-channels", tc.body), http.StatusBadRequest); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}

	// The preview route guards the same ceiling and must word it the same way.
	long := errOf(t, c.post("/notification-template-preview",
		map[string]any{"template": strings.Repeat("x", 8193)}), http.StatusBadRequest)
	if want := "template exceeds 8192 characters"; long != want {
		t.Errorf("preview ceiling: got %q, want %q", long, want)
	}

	// A broken template is reported inline (200 + ok:false) so the editor can show
	// it under the field; the Go parser's own words stay, the wrapper is a sentinel.
	bad := c.expect(t, c.post("/notification-template-preview",
		map[string]any{"template": "{{.Nope"}), http.StatusOK)
	if bad["ok"] != false {
		t.Fatalf("unparseable template previewed as ok: %v", bad)
	}
	if msg, _ := bad["error"].(string); containsCyrillic(msg) {
		t.Errorf("preview error %q is Russian on the wire", msg)
	}

	// Negative control: a complete channel is still accepted.
	c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": "ok", "config": map[string]any{"url": "https://example.test/hook"},
	}), http.StatusCreated)
}

// An email probe on a server without SMTP answers with a warning instead of a
// silent success — that warning is shown verbatim by both clients (the web puts
// it in the channel row, Android wraps it in UiText.Raw), so it travels as a
// sentinel like every other message the clients have to word themselves.
func TestSMTPWarningIsASentinel(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "email", "label": "mail", "config": map[string]any{"address": c.Email},
	}), http.StatusCreated)
	res := c.expect(t, c.post("/notification-channels/"+ch["id"].(string)+"/test", nil), http.StatusOK)
	if res["ok"] != true {
		t.Fatalf("email probe refused: %v", res)
	}
	warning, _ := res["warning"].(string)
	if warning == "" {
		t.Skip("this build has SMTP configured — no warning to check")
	}
	if containsCyrillic(warning) {
		t.Errorf("warning %q is Russian on the wire", warning)
	}
	if want := "smtp is not configured"; warning != want {
		t.Errorf("warning = %q, want %q", warning, want)
	}
}
