package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// goSpawn is a Spawner for tests: a plain goroutine under a cancelable context.
func goSpawn(ctx context.Context) Spawner {
	return func(_ string, fn func(context.Context)) bool {
		go fn(ctx)
		return true
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never held")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func status(r *Registry, key string) Status {
	e, _ := r.Get(key)
	return e.Status
}

// A failed attempt is retried after the backoff and the run records which attempt
// it is on; a permanent error is not retried.
func TestStartRetriesWithBackoff(t *testing.T) {
	r := testRegistry()
	var saved []Entry
	var mu sync.Mutex
	r.SetStore(func(e Entry, _ []LogLine) { mu.Lock(); saved = append(saved, e); mu.Unlock() })
	var calls atomic.Int32
	_, ok := r.Start(Spec{Key: "k", Kind: KindExport, MaxAttempts: 3, Backoff: func(int) time.Duration { return time.Millisecond }},
		goSpawn(context.Background()), func(context.Context, *Handle) error {
			if calls.Add(1) < 3 {
				return errors.New("flaky")
			}
			return nil
		})
	if !ok {
		t.Fatal("start refused")
	}
	waitFor(t, func() bool { return status(r, "k") == StatusDone })
	e, _ := r.Get("k")
	if e.Attempt != 3 || e.MaxAttempts != 3 {
		t.Fatalf("attempt %d of %d, want 3 of 3", e.Attempt, e.MaxAttempts)
	}
	if e.LogWarns != 2 {
		t.Fatalf("each failed attempt should leave a warning, got %d", e.LogWarns)
	}
	mu.Lock()
	last := saved[len(saved)-1]
	mu.Unlock()
	if last.Status != StatusDone {
		t.Fatalf("history did not get the final state: %+v", last)
	}

	calls.Store(0)
	r.Start(Spec{Key: "p", Kind: KindExport, MaxAttempts: 3}, goSpawn(context.Background()),
		func(context.Context, *Handle) error { calls.Add(1); return Permanent(errors.New("bad input")) })
	waitFor(t, func() bool { return status(r, "p") == StatusFailed })
	if n := calls.Load(); n != 1 {
		t.Fatalf("a permanent error was retried: %d calls", n)
	}
}

// Queued jobs wait for a slot and show their place in the queue; a queued job can
// be cancelled before it ever runs.
func TestStartQueueAndCancel(t *testing.T) {
	r := testRegistry()
	r.SetConcurrency(1)
	release := make(chan struct{})
	block := func(ctx context.Context, _ *Handle) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.Start(Spec{Key: "a", Kind: KindExport, Queued: true}, goSpawn(context.Background()), block)
	waitFor(t, func() bool { return status(r, "a") == StatusRunning })
	r.Start(Spec{Key: "b", Kind: KindExport, Queued: true}, goSpawn(context.Background()), block)
	r.Start(Spec{Key: "c", Kind: KindExport, Queued: true}, goSpawn(context.Background()), block)
	waitFor(t, func() bool {
		pos := map[string]int{}
		for _, e := range r.Snapshot() {
			pos[e.Key] = e.QueuePos
		}
		return pos["b"] == 1 && pos["c"] == 2
	})
	if !r.Cancel("c") {
		t.Fatal("a queued job should be cancelable")
	}
	waitFor(t, func() bool { return status(r, "c") == StatusCancelled })
	close(release)
	waitFor(t, func() bool { return status(r, "a") == StatusDone && status(r, "b") == StatusDone })
}

// Steps, progress, the ETA and the log as the window reads them.
func TestStepsProgressAndLog(t *testing.T) {
	r := testRegistry()
	base := time.Unix(1_000_000, 0)
	now := base
	r.now = func() time.Time { return now }
	h, _ := r.BeginSpec(Spec{Key: "k", Kind: KindSync}, nil)
	h.Steps(Step{Key: "a", Label: "A"}, Step{Key: "b", Label: "B"})
	h.Step("a")
	h.Progress(0, 100)
	now = base.Add(10 * time.Second)
	h.Progress(50, 100)
	h.Step("b")
	h.Log(LevelInfo, "one")
	h.Log(LevelError, "two")
	e, _ := r.Get("k")
	if e.Steps[0].Status != StepDone || e.Steps[1].Status != StepRunning {
		t.Fatalf("steps: %+v", e.Steps)
	}
	if e.EtaSec != 10 {
		t.Fatalf("eta = %d, want 10 (50 done in 10s, 50 left)", e.EtaSec)
	}
	if e.LogLines != 2 || e.LogErrors != 1 {
		t.Fatalf("log counters: %+v", e)
	}
	lines, _ := r.LogSince("k", 1)
	if len(lines) != 1 || lines[0].Text != "two" {
		t.Fatalf("LogSince(1) = %+v", lines)
	}
	ups := r.TakeUpdates()
	if len(ups) != 1 || len(ups[0].Lines) != 2 {
		t.Fatalf("first update should carry both lines: %+v", ups)
	}
	h.Log(LevelInfo, "three")
	ups = r.TakeUpdates()
	if len(ups) != 1 || len(ups[0].Lines) != 1 || ups[0].Lines[0].Seq != 3 {
		t.Fatalf("second update should carry only the new line: %+v", ups)
	}
	h.Finish(errors.New("boom"))
	e, _ = r.Get("k")
	if e.Steps[1].Status != StepFailed {
		t.Fatalf("the running step should fail with the run: %+v", e.Steps)
	}
}

// A quietable run that did nothing keeps no history row; one that did, does.
func TestQuietRunSkipsHistory(t *testing.T) {
	r := testRegistry()
	n := 0
	r.SetStore(func(Entry, []LogLine) { n++ })
	h, _ := r.BeginSpec(Spec{Key: "q", Kind: KindSync}, nil)
	h.SetQuietable(true)
	h.Finish(nil)
	if n != 0 {
		t.Fatalf("quiet run was saved %d times", n)
	}
	h, _ = r.BeginSpec(Spec{Key: "q", Kind: KindSync}, nil)
	h.SetQuietable(true)
	h.SetCounts(1, 0)
	h.Finish(nil)
	if n != 1 {
		t.Fatalf("a run that changed something must be saved, saved %d", n)
	}
}

// A handle from a finished run must not write into the next run under the same key.
func TestStaleHandleDoesNotTouchNextRun(t *testing.T) {
	r := testRegistry()
	old, _ := r.BeginSpec(Spec{Key: "k", Kind: KindSync}, nil)
	old.Finish(nil)
	r.BeginSpec(Spec{Key: "k", Kind: KindSync}, nil)
	old.Log(LevelInfo, "late")
	if e, _ := r.Get("k"); e.LogLines != 0 {
		t.Fatalf("stale handle wrote into the new run: %+v", e)
	}
}
