package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tessera/config"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// TestBodyLimitRoutesExist guards the one failure mode the limits table can't
// report on its own: a route constant that no longer matches a registered
// route. The limit would then quietly fall back to the blanket 1 MiB, and the
// only symptom in production would be attachment uploads failing with 413.
func TestBodyLimitRoutesExist(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:     "integration-test-secret-min32chars!!",
		UploadDir:     testUploadDir,
		EncryptionKey: "integration-test-encryption-key",
		CORSOrigin:    "*",
	}
	hub := realtime.NewHub()
	go hub.Run()
	r, _ := newRouter(cfg, testQueries, testPool, hub, mail.New(mail.Config{}))

	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Path] = true
	}
	for _, want := range []string{
		routeWS, routeUploads, routeAvatar, routeAttachments,
		routeDocAssets, routeDocImport, routeDocPdf, routeDocContent, routeSentryTunnel,
	} {
		if !registered[want] {
			t.Errorf("body-limit table names %q, which is not a registered route", want)
		}
	}
	for path := range authRateRules() {
		if !registered[path] {
			t.Errorf("rate-limit table names %q, which is not a registered route", path)
		}
	}
}

func TestBodyLimitRejectsOversizedJSON(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача")

	// Well past the 1 MiB blanket ceiling, but a perfectly valid PATCH shape —
	// so a 413 can only come from the transport limit, not from validation.
	huge := strings.Repeat("я", 700_000) // multi-byte on purpose: ~1.4 MB
	r := c.patch("/tasks/"+task["id"].(string),
		map[string]any{"title": "Задача", "description": huge})
	if r.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized PATCH: status %d, want 413\n%s", r.Status, truncate(r.Body))
	}

	// The same route with a sane body still works — the limit is a ceiling, not
	// a blanket refusal. (Title is mandatory here: UpdateTask is still
	// full-replace, which is #2642's problem, not this batch's.)
	ok := c.patch("/tasks/"+task["id"].(string),
		map[string]any{"title": "Задача", "description": "коротко"})
	if ok.Status != http.StatusOK {
		t.Fatalf("normal PATCH: status %d, want 200\n%s", ok.Status, truncate(ok.Body))
	}
}

// The attachment route gets a budget of its own; without it the blanket 1 MiB
// would break every real upload.
func TestAttachmentBudgetSurvivesLargeUpload(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача с файлом")

	big := make([]byte, 5<<20) // 5 MiB — over the blanket limit, under the route's
	for i := range big {
		big[i] = byte(i)
	}
	r := uploadFile(t, c, "/tasks/"+task["id"].(string)+"/attachments", "file", "big.bin", big)
	if r.Status != http.StatusCreated {
		t.Fatalf("5 MiB attachment: status %d, want 201\n%s", r.Status, truncate(r.Body))
	}
}

// Inline media has a smaller budget than attachments; a file over it must be
// refused rather than buffered.
func TestMediaBudgetRefusesOversizedUpload(t *testing.T) {
	c := signup(t)

	over := make([]byte, 10<<20) // past the 9 MiB media ceiling
	r := uploadFile(t, c, "/uploads", "file", "huge.png", over)
	if r.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized media upload: status %d, want 413\n%s", r.Status, truncate(r.Body))
	}
}

// A PDF dropped into an open page goes to a route of its own, and that route's
// handler allows 20 MiB. Without an entry in the limits table it sits on the
// blanket 1 MiB JSON budget instead, so the transport refuses the request before
// the handler ever sees it — and refuses it with "request body too large", which
// is neither the handler's sentence nor a true one at 2 MiB. Every real PDF is
// over a megabyte, so the route was unusable for its entire purpose.
//
// The pair is the point: one upload the route must carry, one it must refuse
// with its *own* answer. A budget that merely refuses everything would pass the
// first half of this on its own.
func TestDocumentPdfBudgetSurvivesRealPdf(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.post("/workspaces/"+ws+"/documents", map[string]any{"title": "Страница"}).mapBody(t)
	id, _ := doc["id"].(string)

	// Over the blanket ceiling, well under the route's own — an ordinary scan.
	big := append(append([]byte{}, pdfFixture...), make([]byte, 2<<20)...)
	r := uploadFile(t, c, "/documents/"+id+"/pdf", "file", "Смета.pdf", big)
	if r.Status != http.StatusCreated {
		t.Fatalf("2 MiB PDF: status %d, want 201\n%s", r.Status, truncate(r.Body))
	}

	// Past maxDocImportBytes: still a 413, but the handler's, phrased in the
	// units it promises. The transport would say "request body too large".
	over := append(append([]byte{}, pdfFixture...), make([]byte, 21<<20)...)
	ref := uploadFile(t, c, "/documents/"+id+"/pdf", "file", "Огромная.pdf", over)
	if ref.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("21 MiB PDF: status %d, want 413\n%s", ref.Status, truncate(ref.Body))
	}
	if body := string(ref.Body); !strings.Contains(body, "20 МБ") {
		t.Fatalf("21 MiB PDF was refused by the transport, not the handler: %s", truncate(ref.Body))
	}
}

// docOfBytes builds a valid ProseMirror document of roughly `size` bytes of
// JSON: `paras` paragraphs, each holding one text node. Node count stays far
// under maxDocNodes so the only ceiling the request can meet is a size one.
func docOfBytes(paras, perPara int) map[string]any {
	line := strings.Repeat("текст ", perPara/len("текст "))
	content := make([]any, 0, paras)
	for i := 0; i < paras; i++ {
		content = append(content, map[string]any{
			"type":    "paragraph",
			"content": []any{map[string]any{"type": "text", "text": line}},
		})
	}
	return map[string]any{"type": "doc", "content": content}
}

// The editor's own route repeats the mistake the constants above are named
// after, and repeats it where it costs the most. validateDocContent allows a
// document of MaxDocContentBytes (4 MiB of JSON), but with no entry in the limits
// table the save runs on the blanket 1 MiB — so a long page is refused by the
// transport, and the handler's ceiling is unreachable code.
//
// What makes this route different from the three above it is that nobody
// retries a refused upload by hand here: useDocAutosave puts the rejected
// content back on the queue and keeps saving, so a document that crossed a
// megabyte simply stops being written while its author keeps typing. The office
// import lands in the same place — it PATCHes the converted body through this
// route, and a 20 MiB file the import route deliberately accepts produces far
// more than a megabyte of JSON, leaving the freshly created document empty.
//
// Both halves are asserted for the same reason as the PDF pair: a budget that
// merely accepted everything would pass the first and lose the handler's own
// answer on the second.
func TestDocumentContentBudgetSurvivesLongDocument(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.post("/workspaces/"+ws+"/documents", map[string]any{"title": "Длинная страница"}).mapBody(t)
	id, _ := doc["id"].(string)
	updatedAt, _ := doc["updated_at"].(string)

	// ~2 MiB of JSON: over the blanket ceiling, well under the handler's. An
	// ordinary long page — a spec, a manual, an imported .docx.
	big := docOfBytes(2000, 1000)
	r := c.patch("/documents/"+id+"/content",
		map[string]any{"content": big, "updated_at": updatedAt})
	if r.Status != http.StatusOK {
		t.Fatalf("~2 MiB document: status %d, want 200\n%s", r.Status, truncate(r.Body))
	}
	saved := r.mapBody(t)
	updatedAt, _ = saved["updated_at"].(string)

	// Past MaxDocContentBytes: still refused, but by the validator, in its own
	// words and with a 400 — the transport would answer 413 "request body too
	// large", which says nothing about documents. This is what the route's slack
	// buys: an ordinary overshoot is explained rather than cut off.
	over := docOfBytes(3500, 1300)
	ref := c.patch("/documents/"+id+"/content",
		map[string]any{"content": over, "updated_at": updatedAt})
	if ref.Status != http.StatusBadRequest {
		t.Fatalf("~4.5 MiB document: status %d, want 400\n%s", ref.Status, truncate(ref.Body))
	}
	if body := string(ref.Body); !strings.Contains(body, "content exceeds") {
		t.Fatalf("oversized document was refused by the transport, not the validator: %s", truncate(ref.Body))
	}
}

// TestRateLimitOnAuthRoutes boots a second server with throttling on, because
// the shared harness deliberately runs without it.
func TestRateLimitOnAuthRoutes(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:        "integration-test-secret-min32chars!!",
		UploadDir:        testUploadDir,
		EncryptionKey:    "integration-test-encryption-key",
		CORSOrigin:       "*",
		RateLimitEnabled: true,
	}
	hub := realtime.NewHub()
	go hub.Run()
	r, _ := newRouter(cfg, testQueries, testPool, hub, mail.New(mail.Config{}))
	srv := httptest.NewServer(r)
	defer srv.Close()

	login := func(email string) (int, string) {
		body, _ := json.Marshal(map[string]any{"email": email, "password": "wrong-password"})
		res, err := http.Post(srv.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode, res.Header.Get("Retry-After")
	}

	// The credential rule allows a burst of 10 failed logins, then refuses. We
	// fire past the burst and assert the throttle engages with a 429 carrying a
	// Retry-After.
	//
	// We deliberately do NOT pin the 429 to exactly the 11th attempt: each login
	// is a real round trip through Postgres, and on a loaded CI box the burst can
	// span more than one 6s refill interval, handing the bucket a fresh token
	// mid-run (so a request that a fast machine refuses with 429 comes back 401).
	// The exact burst-then-refill arithmetic is pinned deterministically, on an
	// injected clock, by middleware.TestRateStoreBurstThenRefill; here we only
	// prove the limiter is wired onto the auth surface. A failed login nets ~0.85
	// tokens spent after refill, so a 429 is reached far inside this bound for any
	// sane per-request latency.
	var retryAfter string
	throttled := false
	for i := 1; i <= 30; i++ {
		code, ra := login("victim@example.test")
		if code == http.StatusTooManyRequests {
			retryAfter, throttled = ra, true
			break
		}
		if code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401 or 429", i, code)
		}
	}
	if !throttled {
		t.Fatal("credential route never returned 429 despite exceeding the burst")
	}
	if retryAfter == "" {
		t.Error("429 came back without a Retry-After header")
	}

	// A protected route on the same server is untouched — the throttle is scoped
	// to the anonymous auth surface, not to the API at large.
	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health after throttling logins: status %d, want 200", res.StatusCode)
	}
}

// truncate keeps failure output readable when the body is a megabyte of noise.
func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "…"
	}
	return string(b)
}
