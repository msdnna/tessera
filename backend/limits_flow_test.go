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
	for _, want := range []string{routeWS, routeUploads, routeAvatar, routeAttachments} {
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
