package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// clock is a manually advanced time source, so the throttle tests assert on the
// bucket arithmetic instead of on sleeps (a rate-limit test built on real time
// is either slow or flaky, usually both).
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestStore(start time.Time) (*rateStore, *clock) {
	c := &clock{t: start}
	s := newRateStore()
	s.now = c.now
	return s, c
}

func TestRateStoreBurstThenRefill(t *testing.T) {
	rule := RateRule{Every: 6 * time.Second, Burst: 10}
	s, c := newTestStore(time.Unix(1_700_000_000, 0))

	for i := range rule.Burst {
		if ok, _ := s.allow("k", rule); !ok {
			t.Fatalf("request %d within the burst was refused", i+1)
		}
	}

	ok, wait := s.allow("k", rule)
	if ok {
		t.Fatal("request past the burst was allowed")
	}
	if wait <= 0 || wait > rule.Every {
		t.Fatalf("Retry-After hint %v outside (0, %v]", wait, rule.Every)
	}

	// A refused request must not spend a token: after one refill interval
	// exactly one more request fits, no matter how many were refused meanwhile.
	for range 5 {
		if ok, _ := s.allow("k", rule); ok {
			t.Fatal("a refused retry was allowed without waiting")
		}
	}
	c.t = c.t.Add(rule.Every)
	if ok, _ := s.allow("k", rule); !ok {
		t.Fatal("no token after a full refill interval")
	}
	if ok, _ := s.allow("k", rule); ok {
		t.Fatal("a refill interval yielded more than one token")
	}
}

func TestRateStoreKeysAreIndependent(t *testing.T) {
	rule := RateRule{Every: 6 * time.Second, Burst: 2}
	s, _ := newTestStore(time.Unix(1_700_000_000, 0))

	for range rule.Burst {
		if ok, _ := s.allow("a", rule); !ok {
			t.Fatal("burst refused for key a")
		}
	}
	if ok, _ := s.allow("a", rule); ok {
		t.Fatal("key a not throttled")
	}
	if ok, _ := s.allow("b", rule); !ok {
		t.Fatal("key b throttled by key a's traffic")
	}
}

func TestRateStoreEvictsIdleBuckets(t *testing.T) {
	rule := RateRule{Every: time.Second, Burst: 1}
	s, c := newTestStore(time.Unix(1_700_000_000, 0))

	// Fill past the sweep threshold, then jump far enough forward that every
	// existing bucket is idle, and touch one more key to trigger the sweep.
	for i := range rateEvictAt {
		s.allow(string(rune(i%256))+string(rune(i/256)), rule)
	}
	if len(s.buckets) < rateEvictAt {
		t.Fatalf("expected at least %d buckets, got %d", rateEvictAt, len(s.buckets))
	}
	c.t = c.t.Add(time.Hour)
	s.allow("fresh", rule)

	if len(s.buckets) != 1 {
		t.Fatalf("idle buckets survived the sweep: %d left", len(s.buckets))
	}
}

// routerWithLimiter mounts the limiter on a route that echoes 200, which is all
// the middleware tests need from a handler.
func routerWithLimiter(rules map[string]RateRule) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rules))
	r.POST("/api/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/other", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func postLogin(r *gin.Engine, path, body, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":40000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRateLimitThrottlesAndAnswers429(t *testing.T) {
	rules := map[string]RateRule{"/api/auth/login": {Every: 6 * time.Second, Burst: 2}}
	r := routerWithLimiter(rules)

	for i := range 2 {
		if got := postLogin(r, "/api/auth/login", `{"email":"a@b.c"}`, "10.0.0.1").Code; got != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, got)
		}
	}
	w := postLogin(r, "/api/auth/login", `{"email":"a@b.c"}`, "10.0.0.1")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 without a Retry-After header")
	}
}

func TestRateLimitIgnoresUnlistedRoutes(t *testing.T) {
	rules := map[string]RateRule{"/api/auth/login": {Every: time.Hour, Burst: 1}}
	r := routerWithLimiter(rules)

	for i := range 20 {
		if got := postLogin(r, "/api/other", `{}`, "10.0.0.1").Code; got != http.StatusOK {
			t.Fatalf("request %d on an unthrottled route: got %d, want 200", i+1, got)
		}
	}
}

func TestRateLimitKeysByEmailAcrossAddresses(t *testing.T) {
	rules := map[string]RateRule{"/api/auth/login": {Every: 6 * time.Second, Burst: 2, ByEmail: true}}
	r := routerWithLimiter(rules)

	// Same account, a fresh source address every time: the IP bucket never fills,
	// so only the email key can stop this.
	for i := range 2 {
		body := `{"email":"Victim@Example.COM"}`
		if got := postLogin(r, "/api/auth/login", body, "10.0.0."+string(rune('1'+i))).Code; got != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, got)
		}
	}
	if got := postLogin(r, "/api/auth/login", `{"email":"victim@example.com"}`, "10.0.0.9").Code; got != http.StatusTooManyRequests {
		t.Fatalf("spraying one account across addresses: got %d, want 429", got)
	}
	// A different account from a fresh address is unaffected.
	if got := postLogin(r, "/api/auth/login", `{"email":"someone@else.test"}`, "10.0.0.9").Code; got != http.StatusOK {
		t.Fatalf("unrelated account: got %d, want 200", got)
	}
}

func TestRateLimitLeavesBodyReadable(t *testing.T) {
	rules := map[string]RateRule{"/api/auth/login": {Every: time.Second, Burst: 5, ByEmail: true}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rules))
	r.POST("/api/auth/login", func(c *gin.Context) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		c.String(http.StatusOK, body.Email+"/"+body.Password)
	})

	w := postLogin(r, "/api/auth/login", `{"email":"a@b.c","password":"hunter2"}`, "10.0.0.1")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d (%s), want 200", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "a@b.c/hunter2" {
		t.Fatalf("handler saw %q — the limiter consumed the body", got)
	}
}

func TestPeekEmailToleratesJunk(t *testing.T) {
	cases := map[string]string{
		"not json":     "<<<",
		"no email":     `{"password":"x"}`,
		"empty body":   ``,
		"wrong type":   `{"email":123}`,
		"array body":   `[{"email":"a@b.c"}]`,
		"blank string": `{"email":"   "}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
			if got := peekEmail(c); got != "" {
				t.Fatalf("peekEmail(%q) = %q, want empty", body, got)
			}
		})
	}
}
