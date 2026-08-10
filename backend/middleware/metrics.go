package middleware

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// latRingSize bounds the reservoir of recent request latencies kept for
// percentiles — enough to be representative, small enough to stay cheap.
const latRingSize = 1024

// Collector accumulates lightweight in-process HTTP metrics for the
// /admin/metrics probe: request counts by status class and a bounded ring of
// recent latencies for p50/p95. It is per-process (a second instance would count
// independently) — fine for the self-hosted single-process deployment.
type Collector struct {
	mu      sync.Mutex
	total   int64
	byClass map[int]int64 // status/100 (2,3,4,5) → count
	lat     []time.Duration
	latAt   int
	latFull bool
}

// NewCollector builds an empty Collector.
func NewCollector() *Collector {
	return &Collector{byClass: make(map[int]int64), lat: make([]time.Duration, latRingSize)}
}

func (col *Collector) record(status int, d time.Duration) {
	col.mu.Lock()
	col.total++
	col.byClass[status/100]++
	col.lat[col.latAt] = d
	col.latAt = (col.latAt + 1) % latRingSize
	if col.latAt == 0 {
		col.latFull = true
	}
	col.mu.Unlock()
}

// MetricsSnapshot is the JSON view of the collector at a point in time.
type MetricsSnapshot struct {
	Requests int64            `json:"requests"`
	ByClass  map[string]int64 `json:"by_class"` // "2xx", "4xx", …
	P50MS    float64          `json:"p50_ms"`
	P95MS    float64          `json:"p95_ms"`
}

// Snapshot returns a consistent view of the counters and latency percentiles.
func (col *Collector) Snapshot() MetricsSnapshot {
	col.mu.Lock()
	n := latRingSize
	if !col.latFull {
		n = col.latAt
	}
	sample := make([]time.Duration, n)
	copy(sample, col.lat[:n])
	byClass := make(map[string]int64, len(col.byClass))
	for k, v := range col.byClass {
		byClass[fmt.Sprintf("%dxx", k)] = v
	}
	total := col.total
	col.mu.Unlock()

	sort.Slice(sample, func(i, j int) bool { return sample[i] < sample[j] })
	return MetricsSnapshot{
		Requests: total,
		ByClass:  byClass,
		P50MS:    percentileMS(sample, 0.50),
		P95MS:    percentileMS(sample, 0.95),
	}
}

// percentileMS returns the p-th percentile of a sorted latency slice, in
// milliseconds (nearest-rank).
func percentileMS(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return float64(sorted[idx].Microseconds()) / 1000.0
}

// Metrics records each request's final status and latency into col.
func Metrics(col *Collector) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		col.record(c.Writer.Status(), time.Since(start))
	}
}
