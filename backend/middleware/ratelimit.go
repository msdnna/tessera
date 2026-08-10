package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateRule is one route's budget: a token refills every Every, up to Burst
// saved up. ByEmail additionally keys the bucket on the email in the request
// body, so spraying one account from many addresses still hits a ceiling.
type RateRule struct {
	Every   time.Duration
	Burst   int
	ByEmail bool
}

// rateEvictAt is the bucket count past which allow sweeps idle keys. Without a
// sweep the map is itself a slow memory leak — every distinct client IP that
// ever hit a throttled route would be remembered forever.
const rateEvictAt = 4096

type rateEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

// rateStore holds one token bucket per key. Buckets live in this process only:
// a second instance would throttle independently (see #2633).
type rateStore struct {
	mu      sync.Mutex
	buckets map[string]*rateEntry
	now     func() time.Time // injectable for tests
}

func newRateStore() *rateStore {
	return &rateStore{buckets: make(map[string]*rateEntry), now: time.Now}
}

// allow reports whether the request fits the key's budget and, when it doesn't,
// how long the caller must wait for the next token.
func (s *rateStore) allow(key string, r RateRule) (bool, time.Duration) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.buckets[key]
	if !ok {
		if len(s.buckets) >= rateEvictAt {
			s.evict(now, r)
		}
		e = &rateEntry{lim: rate.NewLimiter(rate.Every(r.Every), r.Burst)}
		s.buckets[key] = e
	}
	e.seen = now

	res := e.lim.ReserveN(now, 1)
	if !res.OK() {
		return false, r.Every
	}
	// A refused request must not spend the token it reserved — we reject it, we
	// don't queue it. Cancelling immediately puts the token straight back, so a
	// client hammering the endpoint recovers exactly at the refill rate instead
	// of pushing its own recovery further away with every retry.
	if wait := res.DelayFrom(now); wait > 0 {
		res.CancelAt(now)
		return false, wait
	}
	return true, 0
}

// evict drops buckets idle for longer than it takes one to refill completely —
// past that point the entry is indistinguishable from a fresh one.
func (s *rateStore) evict(now time.Time, r RateRule) {
	idle := time.Duration(r.Burst) * r.Every
	if idle < time.Minute {
		idle = time.Minute
	}
	for k, e := range s.buckets {
		if now.Sub(e.seen) > idle {
			delete(s.buckets, k)
		}
	}
}

// RateLimit throttles the routes named in rules, keyed by gin's FullPath.
// Routes without a rule pass through untouched, so this is safe to mount
// globally.
//
// It must run *after* BodyLimit: the ByEmail rules read the request body, and
// that read is only bounded because BodyLimit capped it first.
func RateLimit(rules map[string]RateRule) gin.HandlerFunc {
	store := newRateStore()
	return func(c *gin.Context) {
		rule, ok := rules[c.FullPath()]
		if !ok {
			c.Next()
			return
		}

		route := c.FullPath()
		keys := []string{route + "|ip|" + c.ClientIP()}
		if rule.ByEmail {
			if email := peekEmail(c); email != "" {
				keys = append(keys, route+"|email|"+email)
			}
		}

		for _, k := range keys {
			allowed, wait := store.allow(k, rule)
			if allowed {
				continue
			}
			retry := int(math.Ceil(wait.Seconds()))
			if retry < 1 {
				retry = 1
			}
			c.Header("Retry-After", strconv.Itoa(retry))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}

// peekEmail pulls the "email" field out of a JSON body and puts the body back
// for the handler. Returns "" when there is nothing usable — a missing or
// malformed body is the handler's problem to report, not the limiter's.
func peekEmail(c *gin.Context) string {
	if c.Request == nil || c.Request.Body == nil {
		return ""
	}
	buf, err := io.ReadAll(c.Request.Body)
	// Restore whatever was read either way, so the handler still sees a body
	// (and still produces its own 400) when the read failed.
	c.Request.Body = io.NopCloser(bytes.NewReader(buf))
	if err != nil {
		return ""
	}
	var body struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(buf, &body) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(body.Email))
}
