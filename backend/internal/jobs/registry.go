// Package jobs is a small in-memory registry that makes background work
// observable and (for long jobs) controllable: the tick-loop workers register a
// heartbeat, GitLab syncs and exports/imports register a cancelable run, and a
// supervisor prints a periodic logfmt summary.
//
// A run carries what the jobs window shows while it works (#3067): progress with
// an ETA, a list of steps, a ring buffer of log lines, the attempt it is on and
// its place in the queue. The registry itself stays in memory; a finished run is
// handed to an optional store (SetStore) that keeps the 30-day history, and the
// changes are pulled by a publisher (Changed / TakeUpdates) that fans them out
// over the realtime hub. A restart starts with an empty registry.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status is a job's lifecycle state.
type Status string

// Job lifecycle states.
const (
	StatusPending   Status = "pending"   // queued, waiting for a slot or for its next attempt
	StatusRunning   Status = "running"   // in flight (or, for a worker, its loop is alive)
	StatusDone      Status = "done"      // finished successfully
	StatusFailed    Status = "failed"    // finished with an error
	StatusCancelled Status = "cancelled" // stopped from the jobs window
)

// Kind distinguishes the shapes of work the registry tracks.
const (
	KindSync   = "sync"   // a discrete run (e.g. a GitLab pull) — cancelable, keeps its last outcome
	KindWorker = "worker" // a persistent background loop — heartbeat only, not cancelable
	KindRun    = "run"    // one on-demand pass of a worker («Запустить сейчас»)
	KindExport = "export" // a board/task export to a downloadable file (#3068)
	KindImport = "import" // a task import from a file (#3068)
)

// Types group jobs for the window's «Тип» filter. They name real job families,
// not the mockup's invented ones.
const (
	TypeGitlab       = "gitlab"
	TypeNotify       = "notify"
	TypeMaintenance  = "maintenance"
	TypeImportExport = "import_export"
)

// Log levels of a run's live log.
const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Step states.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
)

// LogRing is how many lines a running job keeps in memory; LogTail is how many
// of the last ones its history row keeps.
const (
	LogRing = 1000
	LogTail = 200
)

// keepFinished is how long a finished discrete run stays in the live registry
// after it ends. Past that, the history (store) is where it is read from.
const keepFinished = 2 * time.Minute

// Name is a job's display name in two forms: a stable Key (plus an optional Arg
// spliced into the phrase) that the client translates from its own catalog, and a
// pre-rendered Text kept as a fallback for anything that doesn't know the key.
type Name struct {
	Key  string // catalog key, e.g. "gitlab_sync"
	Arg  string // argument inside the phrase, e.g. the integration label
	Text string // pre-rendered fallback
}

// Step is one stage of a run («Подключение к GitLab», «Задачи и комментарии»).
// Label is the rendered fallback; the client translates Key when it knows it.
type Step struct {
	Key        string     `json:"key"`
	Label      string     `json:"label"`
	Status     string     `json:"status"`
	Done       int        `json:"done,omitempty"`
	Total      int        `json:"total,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// LogLine is one line of a run's live log. Seq grows by one per line of the run,
// so a client that already holds lines up to N asks for (and appends) only the
// ones after it — the same line arriving twice is recognisable.
type LogLine struct {
	Seq   int       `json:"seq"`
	At    time.Time `json:"at"`
	Level string    `json:"level"`
	Text  string    `json:"text"`
}

// Entry is a snapshot of one tracked job, safe to serialize to JSON for the API.
// Name/CurrentOp carry the rendered fallback; NameKey/NameArg/CurrentOpKey carry the
// same thing as translatable keys (see Name).
type Entry struct {
	Key           string         `json:"key"`
	RunID         string         `json:"run_id,omitempty"`
	Name          string         `json:"name"`
	NameKey       string         `json:"name_key,omitempty"`
	NameArg       string         `json:"name_arg,omitempty"`
	Kind          string         `json:"kind"`
	Type          string         `json:"type,omitempty"`
	WorkspaceID   string         `json:"workspace_id,omitempty"`
	ActorID       string         `json:"actor_id,omitempty"`
	Trigger       string         `json:"trigger,omitempty"`
	Status        Status         `json:"status"`
	CurrentOp     string         `json:"current_op,omitempty"`
	CurrentOpKey  string         `json:"current_op_key,omitempty"`
	IntervalSec   int            `json:"interval_sec,omitempty"` // worker tick interval (for next-run estimate)
	QueuedAt      *time.Time     `json:"queued_at,omitempty"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
	LastTickAt    *time.Time     `json:"last_tick_at,omitempty"`
	Created       int            `json:"created"`
	Updated       int            `json:"updated"`
	Error         string         `json:"error,omitempty"`
	Cancelable    bool           `json:"cancelable"`
	Attempt       int            `json:"attempt,omitempty"`
	MaxAttempts   int            `json:"max_attempts,omitempty"`
	RetryAt       *time.Time     `json:"retry_at,omitempty"`
	QueuePos      int            `json:"queue_pos,omitempty"`
	ProgressDone  int            `json:"progress_done,omitempty"`
	ProgressTotal int            `json:"progress_total,omitempty"`
	EtaSec        int            `json:"eta_sec,omitempty"`
	Steps         []Step         `json:"steps,omitempty"`
	LogLines      int            `json:"log_lines"`
	LogWarns      int            `json:"log_warns"`
	LogErrors     int            `json:"log_errors"`
	Meta          map[string]any `json:"meta,omitempty"`
}

func (e Entry) clone() Entry {
	if e.Steps != nil {
		e.Steps = append([]Step(nil), e.Steps...)
	}
	if e.Meta != nil {
		m := make(map[string]any, len(e.Meta))
		for k, v := range e.Meta {
			m[k] = v
		}
		e.Meta = m
	}
	return e
}

type slot struct {
	e         Entry
	cancel    context.CancelFunc
	cancelled bool // stopped through Cancel, not by a failure or shutdown
	quietable bool // a no-op finish is not worth a history row (auto syncs)
	waiting   bool // waiting for a queue slot (QueuePos is computed from these)

	log    []LogLine
	seq    int
	pubSeq int // last line handed to the publisher
	dirty  bool

	// ETA: the progress mark the rate is measured from.
	etaFrom     time.Time
	etaFromDone int
}

// Registry tracks jobs under a mutex. The zero value is not usable — call New.
type Registry struct {
	mu      sync.RWMutex
	m       map[string]*slot
	log     *slog.Logger
	now     func() time.Time
	changed chan struct{}
	slots   chan struct{} // the shared queue: one token per job allowed to run at once
	store   func(Entry, []LogLine)
	typeOf  func(Entry) string
}

// DefaultConcurrency is how many queued jobs (exports, imports) run at once.
const DefaultConcurrency = 2

// New builds a registry logging through the given slog.Logger (a TextHandler gives
// the logfmt summary the task asks for).
func New(logger *slog.Logger) *Registry {
	return &Registry{
		m: map[string]*slot{}, log: logger, now: time.Now,
		changed: make(chan struct{}, 1),
		slots:   make(chan struct{}, DefaultConcurrency),
	}
}

// SetConcurrency resizes the shared queue. Call before any job is started.
func (r *Registry) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	r.slots = make(chan struct{}, n)
}

// SetStore installs the history sink: it is called (outside the lock) with a
// run's state and log tail whenever a run starts, changes state, or finishes.
func (r *Registry) SetStore(fn func(Entry, []LogLine)) { r.store = fn }

// SetTypeOf installs the fallback that names a job's family when Begin's caller
// doesn't (the GitLab syncs predate types).
func (r *Registry) SetTypeOf(fn func(Entry) string) { r.typeOf = fn }

// Changed is signalled (coalesced) whenever a job changes in a way a watcher
// should hear about; TakeUpdates then hands over what changed.
func (r *Registry) Changed() <-chan struct{} { return r.changed }

func (r *Registry) signal() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

// markLocked flags a slot for the publisher. Caller holds r.mu.
func (r *Registry) markLocked(s *slot) {
	s.dirty = true
	r.signal()
}

// RegisterWorker records a persistent tick-loop worker as a running heartbeat entry.
// intervalSec is its tick period, so the UI can show when it next fires. Idempotent —
// re-registering the same key refreshes its start time. A worker's own key doubles as
// its name key: the roster is fixed, so the client translates it directly.
func (r *Registry) RegisterWorker(key, name string, intervalSec int) {
	r.RegisterWorkerType(key, name, "", intervalSec)
}

// RegisterWorkerType is RegisterWorker with the job family for the type filter.
func (r *Registry) RegisterWorkerType(key, name, typ string, intervalSec int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.m[key] = &slot{e: Entry{Key: key, Name: name, NameKey: key, Kind: KindWorker, Type: typ, Status: StatusRunning, IntervalSec: intervalSec, StartedAt: &now, LastTickAt: &now}}
}

// Tick updates a worker's heartbeat (last-tick time and current operation, as both a
// translatable key and rendered text). No-op for an unknown key so a worker that
// predates registration can't panic. Not published: the client counts the next
// tick from last_tick_at and interval_sec itself.
func (r *Registry) Tick(key, opKey, op string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[key]
	if !ok {
		return
	}
	now := r.now()
	s.e.LastTickAt = &now
	s.e.CurrentOp = op
	s.e.CurrentOpKey = opKey
}

// Handle is a live reference to one running job, used to update it while it runs.
type Handle struct {
	r   *Registry
	key string
	run string
}

// Spec describes a discrete run.
type Spec struct {
	Key         string
	Name        Name
	Kind        string
	Type        string
	WorkspaceID string
	ActorID     string
	Trigger     string // manual | auto | webhook
	MaxAttempts int    // ≥1; >1 retries a failed attempt with backoff (Start only)
	Queued      bool   // waits for a slot of the shared queue (Start only)
	Quietable   bool   // a finish with nothing done and nothing logged keeps no history row
	Meta        map[string]any
	Backoff     func(attempt int) time.Duration
}

// Begin starts a cancelable job under key. If a job with that key is already
// pending or running it returns (nil, false) — the caller should back off (this
// replaces the old runningSyncs busy-guard). A finished entry from a previous run
// is overwritten. cancel may be nil (job is not cancelable).
func (r *Registry) Begin(key string, name Name, kind, workspaceID string, cancel context.CancelFunc) (*Handle, bool) {
	return r.BeginSpec(Spec{Key: key, Name: name, Kind: kind, WorkspaceID: workspaceID}, cancel)
}

// BeginSpec is Begin for a caller that runs the job inline: the entry is running
// straight away, attempt 1 of 1.
func (r *Registry) BeginSpec(spec Spec, cancel context.CancelFunc) (*Handle, bool) {
	s, ok := r.open(spec, cancel, StatusRunning)
	if !ok {
		return nil, false
	}
	return &Handle{r: r, key: spec.Key, run: s}, true
}

// open registers a new run. Returns its run id.
func (r *Registry) open(spec Spec, cancel context.CancelFunc, st Status) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.m[spec.Key]; ok && (s.e.Status == StatusPending || s.e.Status == StatusRunning) {
		return "", false
	}
	now := r.now()
	maxA := spec.MaxAttempts
	if maxA < 1 {
		maxA = 1
	}
	e := Entry{
		Key: spec.Key, RunID: uuid.NewString(), Name: spec.Name.Text, NameKey: spec.Name.Key, NameArg: spec.Name.Arg,
		Kind: spec.Kind, Type: spec.Type, WorkspaceID: spec.WorkspaceID, ActorID: spec.ActorID, Trigger: spec.Trigger,
		Status: st, QueuedAt: &now, Cancelable: cancel != nil, MaxAttempts: maxA, Meta: spec.Meta,
	}
	if st == StatusRunning {
		e.StartedAt = &now
		e.Attempt = 1
	}
	if e.Type == "" && r.typeOf != nil {
		e.Type = r.typeOf(e)
	}
	s := &slot{e: e, cancel: cancel, quietable: spec.Quietable}
	r.m[spec.Key] = s
	r.markLocked(s)
	return e.RunID, true
}

// with runs fn on the handle's slot under the lock, if the slot still belongs to
// this run (a key reused by a later run must not be written by a stale handle).
func (h *Handle) with(fn func(s *slot)) {
	if h == nil {
		return
	}
	h.r.mu.Lock()
	defer h.r.mu.Unlock()
	if s, ok := h.r.m[h.key]; ok && s.e.RunID == h.run {
		fn(s)
		h.r.markLocked(s)
	}
}

// Live returns a handle to the running job under key, or nil — for code deep in a
// run that reports progress without having the handle passed down to it. A nil
// handle is safe to call.
func (r *Registry) Live(key string) *Handle {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.m[key]; ok && s.e.Status == StatusRunning && s.e.FinishedAt == nil {
		return &Handle{r: r, key: key, run: s.e.RunID}
	}
	return nil
}

// RunID is the id the run's history row is stored under.
func (h *Handle) RunID() string {
	if h == nil {
		return ""
	}
	return h.run
}

// SetOp records what a running job is currently doing (shown in the API/summary), as
// both a translatable key and rendered text.
func (h *Handle) SetOp(opKey, op string) {
	h.with(func(s *slot) { s.e.CurrentOp, s.e.CurrentOpKey = op, opKey })
}

// SetCounts records progress counters (e.g. created/updated tasks) for a job.
func (h *Handle) SetCounts(created, updated int) {
	h.with(func(s *slot) { s.e.Created, s.e.Updated = created, updated })
}

// SetMeta records one fact about the run (mode, board, result file …).
func (h *Handle) SetMeta(key string, v any) {
	h.with(func(s *slot) {
		if s.e.Meta == nil {
			s.e.Meta = map[string]any{}
		}
		s.e.Meta[key] = v
	})
}

// SetTrigger records who or what started the run, and the actor when known.
func (h *Handle) SetTrigger(trigger, actorID string) {
	h.with(func(s *slot) {
		s.e.Trigger = trigger
		if actorID != "" {
			s.e.ActorID = actorID
		}
	})
}

// SetQuietable marks a run whose no-op finish keeps no history row.
func (h *Handle) SetQuietable(q bool) { h.with(func(s *slot) { s.quietable = q }) }

// Steps lays out the plan of a run: every step pending. Calling it again (a retry)
// starts the plan over.
func (h *Handle) Steps(steps ...Step) {
	h.with(func(s *slot) {
		s.e.Steps = make([]Step, len(steps))
		for i, st := range steps {
			s.e.Steps[i] = Step{Key: st.Key, Label: st.Label, Status: StepPending}
		}
	})
}

// Step starts the named step, closing the one before it as done. A step not in
// the plan is appended.
func (h *Handle) Step(key string) {
	h.with(func(s *slot) {
		now := h.r.now()
		idx := -1
		for i := range s.e.Steps {
			if s.e.Steps[i].Status == StepRunning {
				s.e.Steps[i].Status = StepDone
				s.e.Steps[i].FinishedAt = &now
			}
			if s.e.Steps[i].Key == key {
				idx = i
			}
		}
		if idx < 0 {
			s.e.Steps = append(s.e.Steps, Step{Key: key, Label: key})
			idx = len(s.e.Steps) - 1
		}
		s.e.Steps[idx].Status = StepRunning
		s.e.Steps[idx].StartedAt = &now
		s.e.Steps[idx].FinishedAt = nil
	})
}

// StepCount records the running step's own counter («248 / 401»). total 0 shows a
// plain count.
func (h *Handle) StepCount(done, total int) {
	h.with(func(s *slot) {
		for i := range s.e.Steps {
			if s.e.Steps[i].Status == StepRunning {
				s.e.Steps[i].Done, s.e.Steps[i].Total = done, total
			}
		}
	})
}

// Progress records the run's overall progress; the ETA is measured from the first
// mark of this attempt.
func (h *Handle) Progress(done, total int) {
	h.with(func(s *slot) {
		if s.etaFrom.IsZero() || done < s.etaFromDone {
			s.etaFrom, s.etaFromDone = h.r.now(), done
		}
		s.e.ProgressDone, s.e.ProgressTotal = done, total
	})
}

// Log appends a line to the run's live log.
func (h *Handle) Log(level, text string) {
	h.with(func(s *slot) { s.append(h.r.now(), level, text) })
}

// Logf is Log with formatting.
func (h *Handle) Logf(level, format string, args ...any) { h.Log(level, fmt.Sprintf(format, args...)) }

func (s *slot) append(at time.Time, level, text string) {
	s.seq++
	if len(s.log) >= LogRing {
		copy(s.log, s.log[1:])
		s.log = s.log[:len(s.log)-1]
	}
	s.log = append(s.log, LogLine{Seq: s.seq, At: at, Level: level, Text: text})
	s.e.LogLines = s.seq
	switch level {
	case LevelWarn:
		s.e.LogWarns++
	case LevelError:
		s.e.LogErrors++
	}
}

func (s *slot) tail(n int) []LogLine {
	if len(s.log) <= n {
		return append([]LogLine(nil), s.log...)
	}
	return append([]LogLine(nil), s.log[len(s.log)-n:]...)
}

// Finish marks a job done (failed when err != nil, cancelled when it was stopped
// through Cancel), clears its cancel handle and current op, and keeps the entry as
// the job's last-run record. The run then goes to the history store — unless it is
// quietable and did nothing worth a row.
func (h *Handle) Finish(err error) {
	if h == nil {
		return
	}
	var (
		save Entry
		tail []LogLine
		keep bool
	)
	h.with(func(s *slot) {
		now := h.r.now()
		s.e.FinishedAt = &now
		s.e.CurrentOp, s.e.CurrentOpKey = "", ""
		s.e.RetryAt = nil
		s.waiting = false
		s.cancel = nil
		s.e.Cancelable = false
		stepState := StepDone
		switch {
		case err == nil:
			s.e.Status = StatusDone
		case s.cancelled && errors.Is(err, context.Canceled):
			s.e.Status = StatusCancelled
			stepState = StepFailed
		default:
			s.e.Status, s.e.Error = StatusFailed, err.Error()
			stepState = StepFailed
		}
		for i := range s.e.Steps {
			if s.e.Steps[i].Status == StepRunning {
				s.e.Steps[i].Status = stepState
				s.e.Steps[i].FinishedAt = &now
			}
		}
		if s.e.Attempt == 0 {
			s.e.Attempt = 1
		}
		quiet := s.quietable && s.e.Status == StatusDone && s.e.Created+s.e.Updated == 0 && s.e.LogWarns+s.e.LogErrors == 0
		keep = !quiet
		save, tail = s.e.clone(), s.tail(LogTail)
	})
	if keep {
		h.r.persist(save, tail)
	}
}

func (r *Registry) persist(e Entry, tail []LogLine) {
	if r.store != nil && e.RunID != "" && e.Kind != KindWorker {
		r.store(e, tail)
	}
}

// Cancel invokes a running (or queued) job's cancel func, if it has one. Returns
// false when the key is unknown, already finished, or not cancelable.
func (r *Registry) Cancel(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[key]
	if !ok || s.cancel == nil {
		return false
	}
	s.cancelled = true
	s.cancel()
	r.markLocked(s)
	return true
}

// view is the entry as the API shows it right now: a clone with the ETA worked out.
func (r *Registry) view(s *slot, now time.Time) Entry {
	e := s.e.clone()
	if e.Status == StatusRunning && e.ProgressTotal > 0 && !s.etaFrom.IsZero() {
		done := e.ProgressDone - s.etaFromDone
		left := e.ProgressTotal - e.ProgressDone
		if done > 0 && left > 0 {
			per := now.Sub(s.etaFrom) / time.Duration(done)
			e.EtaSec = int((per * time.Duration(left)).Round(time.Second) / time.Second)
		}
	}
	return e
}

// Snapshot returns a copy of every entry, workers first then jobs, each group
// newest-started first — a stable order for the management UI. Queued jobs get
// their place in the queue.
func (r *Registry) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now()
	out := make([]Entry, 0, len(r.m))
	for _, s := range r.m {
		out = append(out, r.view(s, now))
	}
	r.queuePositions(out)
	sortEntries(out)
	return out
}

// queuePositions numbers the jobs waiting for a slot, oldest first. Caller holds
// at least the read lock.
func (r *Registry) queuePositions(out []Entry) {
	var waiting []int
	for i := range out {
		if s := r.m[out[i].Key]; s != nil && s.waiting {
			waiting = append(waiting, i)
		}
	}
	for i := 1; i < len(waiting); i++ {
		for j := i; j > 0 && queuedBefore(out[waiting[j]], out[waiting[j-1]]); j-- {
			waiting[j], waiting[j-1] = waiting[j-1], waiting[j]
		}
	}
	for pos, i := range waiting {
		out[i].QueuePos = pos + 1
	}
}

func queuedBefore(a, b Entry) bool {
	if a.QueuedAt == nil || b.QueuedAt == nil {
		return a.QueuedAt != nil
	}
	return a.QueuedAt.Before(*b.QueuedAt)
}

// Get returns one entry by key.
func (r *Registry) Get(key string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.m[key]; ok {
		return r.view(s, r.now()), true
	}
	return Entry{}, false
}

// GetRun returns the live entry of a run by its run id.
func (r *Registry) GetRun(runID string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.m {
		if s.e.RunID == runID {
			return r.view(s, r.now()), true
		}
	}
	return Entry{}, false
}

// LogSince returns the lines of a live job (by key or run id) after seq `after`,
// and whether the job is live at all. Lines that already fell out of the ring are
// gone; the first returned seq tells the client how many it missed.
func (r *Registry) LogSince(keyOrRun string, after int) ([]LogLine, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.m[keyOrRun]
	if !ok {
		for _, x := range r.m {
			if x.e.RunID == keyOrRun {
				s, ok = x, true
				break
			}
		}
	}
	if !ok {
		return nil, false
	}
	out := make([]LogLine, 0, 64)
	for _, l := range s.log {
		if l.Seq > after {
			out = append(out, l)
		}
	}
	return out, true
}

// Update is one changed job as the publisher sees it: its state and the log lines
// added since the last update.
type Update struct {
	Entry Entry
	Lines []LogLine
}

// maxLinesPerUpdate caps one update's log batch; a client that falls behind
// reads the rest through the log endpoint.
const maxLinesPerUpdate = 200

// TakeUpdates hands over every job that changed since the last call, and drops
// finished runs that have been kept long enough.
func (r *Registry) TakeUpdates() []Update {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	var out []Update
	var views []Entry
	for key, s := range r.m {
		if s.dirty {
			s.dirty = false
			var lines []LogLine
			for _, l := range s.log {
				if l.Seq > s.pubSeq {
					lines = append(lines, l)
				}
			}
			if len(lines) > maxLinesPerUpdate {
				lines = lines[len(lines)-maxLinesPerUpdate:]
			}
			s.pubSeq = s.seq
			views = append(views, r.view(s, now))
			out = append(out, Update{Lines: lines})
			continue
		}
		if s.e.Kind != KindWorker && s.e.FinishedAt != nil && now.Sub(*s.e.FinishedAt) > keepFinished {
			delete(r.m, key)
		}
	}
	r.queuePositions(views)
	for i := range out {
		out[i].Entry = views[i]
	}
	return out
}

// LogRunningSummary prints one logfmt line summarizing background work: the running
// sync jobs (with the current one and how long it's been going) or, when nothing is
// running, an idle heartbeat so the log always shows the supervisor is alive and the
// system is quiet. Returns the number of running/pending sync jobs.
func (r *Registry) LogRunningSummary() int {
	r.mu.RLock()
	var queue, workers int
	var current string
	var since time.Time
	for _, s := range r.m {
		if s.e.Kind == KindWorker {
			workers++
			continue
		}
		if s.e.Status == StatusRunning || s.e.Status == StatusPending {
			queue++
			if s.e.Status == StatusRunning && s.e.StartedAt != nil && (since.IsZero() || s.e.StartedAt.Before(since)) {
				current, since = s.e.Name, *s.e.StartedAt
			}
		}
	}
	r.mu.RUnlock()
	if queue == 0 {
		// Idle: no discrete job in flight; the tick-loop workers are alive (heartbeat).
		r.log.Info("background tasks are idle",
			slog.Int("queue", 0), slog.String("status", string(StatusPending)), slog.Int("workers", workers))
		return 0
	}
	attrs := []any{slog.Int("queue", queue), slog.String("status", string(StatusRunning))}
	if current != "" {
		attrs = append(attrs,
			slog.String("current", current),
			slog.String("processing", r.now().Sub(since).Truncate(time.Second).String()),
		)
	}
	r.log.Info("background tasks are still running", attrs...)
	return queue
}

func sortEntries(e []Entry) {
	// Simple insertion sort keeps the package tiny; the registry never holds more
	// than a few dozen entries.
	for i := 1; i < len(e); i++ {
		for j := i; j > 0 && lessEntry(e[j], e[j-1]); j-- {
			e[j], e[j-1] = e[j-1], e[j]
		}
	}
}

// lessEntry orders workers before discrete jobs, then newest start (or queueing)
// first.
func lessEntry(a, b Entry) bool {
	if (a.Kind == KindWorker) != (b.Kind == KindWorker) {
		return a.Kind == KindWorker
	}
	return startOf(a).After(startOf(b))
}

func startOf(e Entry) time.Time {
	switch {
	case e.StartedAt != nil:
		return *e.StartedAt
	case e.QueuedAt != nil:
		return *e.QueuedAt
	}
	return time.Time{}
}
