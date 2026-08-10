package middleware

import (
	"testing"
	"time"
)

// The collector must count by status class and derive sane percentiles from the
// recorded latencies.
func TestCollectorSnapshot(t *testing.T) {
	col := NewCollector()
	col.record(200, 10*time.Millisecond)
	col.record(201, 20*time.Millisecond)
	col.record(404, 5*time.Millisecond)
	col.record(500, 30*time.Millisecond)

	s := col.Snapshot()
	if s.Requests != 4 {
		t.Fatalf("requests = %d, want 4", s.Requests)
	}
	if s.ByClass["2xx"] != 2 || s.ByClass["4xx"] != 1 || s.ByClass["5xx"] != 1 {
		t.Fatalf("by_class = %v, want 2xx:2 4xx:1 5xx:1", s.ByClass)
	}
	if s.P50MS <= 0 {
		t.Fatalf("p50 = %v, want > 0", s.P50MS)
	}
	if s.P95MS < s.P50MS {
		t.Fatalf("p95 (%v) < p50 (%v)", s.P95MS, s.P50MS)
	}
}

// An empty collector must not panic and reports zeroes.
func TestCollectorEmpty(t *testing.T) {
	s := NewCollector().Snapshot()
	if s.Requests != 0 || s.P50MS != 0 || s.P95MS != 0 {
		t.Fatalf("empty snapshot not zero: %+v", s)
	}
}
