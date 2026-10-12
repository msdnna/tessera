package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Spawner starts fn detached from the caller, inside the process's background
// scope (handlers.API.runDetached). It reports false once shutdown is under way.
type Spawner func(name string, fn func(context.Context)) bool

// RunFunc is the body of a queued job. It reports progress through the handle;
// an error fails the attempt (and, with attempts left, schedules the next one).
type RunFunc func(ctx context.Context, h *Handle) error

// permanent marks an error that another attempt cannot fix (bad input, a deleted
// board) — the job fails at once instead of burning its retries.
type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Permanent wraps err so Start does not retry it.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

// IsPermanent reports whether err was wrapped by Permanent.
func IsPermanent(err error) bool {
	var p permanent
	return errors.As(err, &p)
}

// DefaultBackoff waits 5 s, 30 s, 2 min … between attempts.
func DefaultBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 30 * time.Second
	default:
		return 2 * time.Minute
	}
}

// ErrShuttingDown is the outcome of a job that could not start because the
// process is stopping.
var ErrShuttingDown = errors.New("server is shutting down")

// Start registers a job and runs it detached: it waits for a slot of the shared
// queue when spec.Queued (visible as «В очереди» with its place), runs fn, and on
// failure retries with backoff up to spec.MaxAttempts («Попытка 2 из 3»). The job
// is cancelable throughout — in the queue, mid-run and between attempts.
//
// Returns (nil, false) when a job with this key is already pending or running, or
// when the spawner refuses (shutdown).
func (r *Registry) Start(spec Spec, spawn Spawner, fn RunFunc) (*Handle, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	runID, ok := r.open(spec, cancel, StatusPending)
	if !ok {
		cancel()
		return nil, false
	}
	h := &Handle{r: r, key: spec.Key, run: runID}
	r.save(h)
	started := spawn("job:"+spec.Key, func(bg context.Context) {
		// The process's shutdown reaches the run through its own context.
		stop := context.AfterFunc(bg, cancel)
		defer stop()
		defer cancel()
		r.loop(ctx, h, spec, fn)
	})
	if !started {
		cancel()
		h.Finish(ErrShuttingDown)
		return nil, false
	}
	return h, true
}

func (r *Registry) loop(ctx context.Context, h *Handle, spec Spec, fn RunFunc) {
	maxA := spec.MaxAttempts
	if maxA < 1 {
		maxA = 1
	}
	backoff := spec.Backoff
	if backoff == nil {
		backoff = DefaultBackoff
	}
	for attempt := 1; ; attempt++ {
		if spec.Queued && !r.acquire(ctx, h) {
			h.Finish(ctx.Err())
			return
		}
		h.attemptStarted(attempt)
		r.save(h)
		err := safeRun(ctx, h, fn)
		if spec.Queued {
			<-r.slots
		}
		if err == nil || ctx.Err() != nil || attempt >= maxA || IsPermanent(err) {
			h.Finish(err)
			return
		}
		wait := backoff(attempt)
		h.Logf(LevelWarn, "Попытка %d из %d не удалась: %v — повтор через %s", attempt, maxA, err, wait.Round(time.Second))
		h.retryAt(h.r.now().Add(wait))
		r.save(h)
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			h.Finish(ctx.Err())
			return
		}
	}
}

// safeRun runs one attempt, turning a panic into the attempt's error so the run
// still finishes (a recovered panic further up would leave it «running» forever).
func safeRun(ctx context.Context, h *Handle, fn RunFunc) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Permanent(fmt.Errorf("panic: %v", p))
		}
	}()
	return fn(ctx, h)
}

// acquire waits for a queue slot, showing the job as queued meanwhile.
func (r *Registry) acquire(ctx context.Context, h *Handle) bool {
	select {
	case r.slots <- struct{}{}:
		return true
	default:
	}
	h.with(func(s *slot) { s.waiting, s.e.Status = true, StatusPending })
	select {
	case r.slots <- struct{}{}:
		h.with(func(s *slot) { s.waiting = false })
		return true
	case <-ctx.Done():
		return false
	}
}

func (h *Handle) attemptStarted(attempt int) {
	h.with(func(s *slot) {
		now := h.r.now()
		s.waiting = false
		s.e.Status = StatusRunning
		s.e.Attempt = attempt
		s.e.RetryAt = nil
		s.etaFrom, s.etaFromDone = time.Time{}, 0
		if s.e.StartedAt == nil {
			s.e.StartedAt = &now
		}
	})
}

func (h *Handle) retryAt(at time.Time) {
	h.with(func(s *slot) {
		now := h.r.now()
		s.e.Status = StatusPending
		s.e.RetryAt = &at
		s.e.CurrentOp, s.e.CurrentOpKey = "", ""
		// The step the attempt failed on reads as failed until the next attempt
		// lays the plan out again.
		for i := range s.e.Steps {
			if s.e.Steps[i].Status == StepRunning {
				s.e.Steps[i].Status = StepFailed
				s.e.Steps[i].FinishedAt = &now
			}
		}
	})
}

// save hands the run's current state to the history store, so a queued or running
// export/import is in the history from the start (and a crash leaves a row the
// next start closes as interrupted).
func (r *Registry) save(h *Handle) {
	if r.store == nil {
		return
	}
	r.mu.RLock()
	s, ok := r.m[h.key]
	if !ok || s.e.RunID != h.run {
		r.mu.RUnlock()
		return
	}
	e, tail := r.view(s, r.now()), s.tail(LogTail)
	r.mu.RUnlock()
	r.persist(e, tail)
}
