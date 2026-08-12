package middleware

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The throttle is what keeps a batching PAT client (MCP, CI) from spending a
// pool connection per request on a last_used_at write nobody reads at second
// precision — so the window boundary is the behaviour worth pinning.
func TestPATToucherThrottlesWithinWindow(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	now := base
	p := newPATToucher(5 * time.Minute)
	p.now = func() time.Time { return now }

	id := uuid.New()
	other := uuid.New()

	cases := []struct {
		name    string
		advance time.Duration
		id      uuid.UUID
		want    bool
	}{
		{"first use writes", 0, id, true},
		{"second use in window is skipped", time.Second, id, false},
		{"still inside the window", 4 * time.Minute, id, false},
		{"a different token is tracked separately", 0, other, true},
		{"past the window writes again", time.Minute + time.Second, id, true},
		{"and throttles again from the new mark", time.Second, id, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now = now.Add(tc.advance)
			if got := p.shouldTouch(tc.id); got != tc.want {
				t.Fatalf("shouldTouch = %v, want %v (at %s)", got, tc.want, now.Sub(base))
			}
		})
	}
}

// interval<=0 must keep the historical write-on-every-request behaviour, so the
// knob can be turned off without a code change.
func TestPATToucherDisabled(t *testing.T) {
	p := newPATToucher(0)
	id := uuid.New()
	for i := range 3 {
		if !p.shouldTouch(id) {
			t.Fatalf("call %d: throttle engaged with interval=0", i)
		}
	}
	if len(p.last) != 0 {
		t.Fatalf("disabled toucher should not retain state, got %d entries", len(p.last))
	}
}

// Eviction keeps the map from becoming a leak on an install that mints many
// short-lived tokens.
func TestPATToucherEvictsStaleEntries(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	p := newPATToucher(time.Minute)
	p.now = func() time.Time { return now }

	for range patTouchEvictAt + 1 {
		p.shouldTouch(uuid.New())
	}
	if len(p.last) <= patTouchEvictAt {
		t.Fatalf("precondition: want map over the sweep threshold, got %d", len(p.last))
	}

	now = now.Add(10 * time.Minute) // every entry is now well past 2*interval
	p.shouldTouch(uuid.New())
	if len(p.last) != 1 {
		t.Fatalf("stale entries not swept: %d remain", len(p.last))
	}
}

// The middleware calls shouldTouch from concurrent request goroutines.
func TestPATToucherConcurrent(t *testing.T) {
	p := newPATToucher(time.Hour)
	id := uuid.New()

	var wg sync.WaitGroup
	writes := make(chan bool, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writes <- p.shouldTouch(id)
		}()
	}
	wg.Wait()
	close(writes)

	n := 0
	for w := range writes {
		if w {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one write to win the window, got %d", n)
	}
}
