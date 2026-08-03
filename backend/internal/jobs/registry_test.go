package jobs

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func testRegistry() *Registry {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestBeginBusyGuard(t *testing.T) {
	r := testRegistry()
	if _, ok := r.Begin("k", "job", KindSync, "", nil); !ok {
		t.Fatal("first Begin should start")
	}
	if _, ok := r.Begin("k", "job", KindSync, "", nil); ok {
		t.Fatal("second Begin on a running key must report busy")
	}
}

func TestFinishAllowsRestart(t *testing.T) {
	r := testRegistry()
	h, _ := r.Begin("k", "job", KindSync, "ws1", nil)
	h.SetCounts(2, 3)
	h.Finish(nil)
	e, ok := r.Get("k")
	if !ok || e.Status != StatusDone || e.Created != 2 || e.Updated != 3 || e.FinishedAt == nil {
		t.Fatalf("finished entry wrong: %+v", e)
	}
	// A finished key is free to run again.
	if _, ok := r.Begin("k", "job", KindSync, "ws1", nil); !ok {
		t.Fatal("Begin after Finish should start a new run")
	}
}

func TestFinishError(t *testing.T) {
	r := testRegistry()
	h, _ := r.Begin("k", "job", KindSync, "", nil)
	h.Finish(io.EOF)
	e, _ := r.Get("k")
	if e.Status != StatusFailed || e.Error == "" {
		t.Fatalf("failed entry wrong: %+v", e)
	}
}

func TestCancel(t *testing.T) {
	r := testRegistry()
	cancelled := false
	r.Begin("k", "job", KindSync, "", func() { cancelled = true })
	if !r.Cancel("k") || !cancelled {
		t.Fatal("Cancel should invoke the cancel func")
	}
	// A worker (nil cancel) is not cancelable.
	r.RegisterWorker("w", "worker")
	if r.Cancel("w") {
		t.Fatal("worker without cancel must not be cancelable")
	}
	if r.Cancel("missing") {
		t.Fatal("unknown key must not cancel")
	}
}

func TestSnapshotOrdersWorkersFirst(t *testing.T) {
	r := testRegistry()
	r.Begin("s", "sync", KindSync, "", nil)
	r.RegisterWorker("w", "worker")
	snap := r.Snapshot()
	if len(snap) != 2 || snap[0].Kind != KindWorker {
		t.Fatalf("workers should sort first: %+v", snap)
	}
}

func TestLogRunningSummaryCountsOnlySyncs(t *testing.T) {
	r := testRegistry()
	r.RegisterWorker("w", "worker") // workers don't count as running work
	if n := r.LogRunningSummary(); n != 0 {
		t.Fatalf("idle summary should be 0, got %d", n)
	}
	h, _ := r.Begin("s", "sync", KindSync, "", nil)
	if n := r.LogRunningSummary(); n != 1 {
		t.Fatalf("one running sync should count 1, got %d", n)
	}
	h.Finish(nil)
	if n := r.LogRunningSummary(); n != 0 {
		t.Fatalf("finished sync should not count, got %d", n)
	}
}

func TestTickUpdatesHeartbeat(t *testing.T) {
	r := testRegistry()
	base := time.Unix(1_000_000, 0)
	r.now = func() time.Time { return base }
	r.RegisterWorker("w", "worker")
	r.now = func() time.Time { return base.Add(time.Minute) }
	r.Tick("w", "working")
	e, _ := r.Get("w")
	if e.CurrentOp != "working" || e.LastTickAt == nil || !e.LastTickAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("tick did not update heartbeat: %+v", e)
	}
}
