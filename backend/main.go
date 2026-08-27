// Command tessera is the backend API server (gin + pgx + WebSocket hub).
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"tessera/config"
	"tessera/internal/database"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/observability"
	"tessera/internal/realtime"
)

// setupLogging configures the default slog logger — the one behind fail()/soft()
// and any structured log — from the environment. LOG_LEVEL (debug|info|warn|
// error, default info) sets the threshold; LOG_FORMAT=json switches from logfmt
// text to JSON for a log aggregator. The stdlib log.Printf access/shutdown lines
// are deliberately left as-is.
func setupLogging() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "json") {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}

func main() {
	_ = godotenv.Load()
	setupLogging()

	cfg := config.New()

	// Optional error/performance telemetry. No-op unless SENTRY_DSN is set; the
	// deferred flush drains buffered events so a crash-on-shutdown still lands.
	flushSentry := observability.InitSentry(cfg.SentryDSN, cfg.SentryEnv, "tessera-backend@"+appVersion, cfg.SentryTracesRate)
	defer flushSentry()

	// Attachments write to UPLOAD_DIR; nothing else in the boot path does, so a
	// volume we cannot write to would stay invisible until a user uploads a file.
	// Runs after InitSentry so the report lands there too.
	preflightUploadDir(cfg.UploadDir)

	// Cancelled on SIGINT/SIGTERM; every background worker hangs off it, so a
	// deploy stops them at a tick boundary instead of mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool := database.Connect(context.Background(), cfg.DatabaseURL)
	defer pool.Close()

	queries := db.New(pool)

	// Realtime fan-out hub for live board updates.
	hub := realtime.NewHub()
	var hubWG sync.WaitGroup
	hubWG.Add(1)
	go func() {
		defer hubWG.Done()
		// A panic in the fan-out loop would otherwise kill the process and take
		// every live board with it; report it instead of crashing.
		defer observability.Recover("realtime-hub")
		hub.Run()
	}()

	// Email transport (U2 invites / verification / reset). No-op when SMTP is
	// unconfigured (self-host) — links are logged instead of sent.
	mailer := mail.New(mail.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser,
		Password: cfg.SMTPPass, From: cfg.SMTPFrom,
	})
	log.Printf("mail: enabled=%v", mailer.Enabled())

	r, rh := newRouter(cfg, queries, pool, hub, mailer)

	// Give boards/notes that predate the slug column a human-readable slug.
	rh.BackfillSlugs(ctx)

	// Close sync runs left "running" by a process that died mid-sync — nothing of
	// ours is in flight yet, so they can only be stale.
	rh.FailStaleSyncRuns(ctx)

	// Workers all block until ctx is done; the WaitGroup lets shutdown wait for
	// them to finish the tick they're on before the pool closes under them. Each is
	// run under superviseWorker so a panic in one tick is reported to Sentry and the
	// loop restarts instead of taking the whole process down.
	var workers sync.WaitGroup
	spawn := func(name string, fn func(context.Context)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			superviseWorker(ctx, name, fn)
		}()
	}

	// Record the tick-loop workers in the jobs registry (heartbeat entries) and start
	// the supervisor that logs a periodic summary of in-flight background jobs.
	rh.RegisterBackgroundWorkers()
	spawn("job_supervisor", rh.RunJobSupervisor)

	// Background GitLab auto-sync worker (idle until an integration sets a
	// positive sync interval).
	spawn("gitlab_sync_cron", rh.RunSyncWorker)

	// Background GitLab write-back worker — drains the outbox of task changes to
	// push to linked issues. Idle until a user enables write-back on an integration.
	spawn("gitlab_writeback", rh.RunGitlabWriteBackWorker)

	// Background notification delivery worker — drains the outbox of channel
	// deliveries (email/telegram/webhook). Idle until a user configures channels.
	spawn("notify_delivery", rh.RunNotificationWorker)

	// Background scanner — emits due-date + reminder notifications on schedule.
	spawn("notify_scanner", rh.RunNotificationScanner)

	// Background worker — advances schedule-triggered recurring tasks once due.
	// Idle until a task carries a "schedule"-trigger recurrence rule.
	spawn("recurrence", rh.RunRecurrenceWorker)

	// Explicit server so we can bound the header read and idle keep-alive without
	// capping ReadTimeout/WriteTimeout — those would forcibly cut long-lived
	// WebSocket connections and large attachment up/downloads. ReadHeaderTimeout
	// blunts slow-header (Slowloris) clients; IdleTimeout recycles dead keep-alives.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("Server starting on :%s", cfg.Port)
	listenErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
		}
		close(listenErr)
	}()

	select {
	case err := <-listenErr:
		if err != nil {
			log.Fatal(err) //nolint:gocritic // nothing to clean up on listen failure
		}
	case <-ctx.Done():
	}

	stop() // restore default signal handling: a second Ctrl-C kills immediately
	// Document rooms go first: their locks are in-memory, so a participant left
	// connected here would be shown as "editing" by everyone who reconnects to
	// the replacement process, with nobody able to release it (#2729).
	rh.CloseDocRooms()
	drain(srv, &workers, hub, &hubWG, cfg.GracefulTimeout)
	// pool.Close runs deferred, after everything that could still use it.
}

// superviseWorker runs a background worker under panic recovery: a panic in one
// tick is reported to Sentry (tagged with name) and the loop is restarted after a
// short pause instead of taking the whole process down with it — the gap gin's
// recovery and middleware.SentryReport can't cover, since neither sees a
// non-HTTP goroutine. Returns when ctx is cancelled; a clean return from fn on a
// live ctx is treated as an early exit and restarted the same way.
func superviseWorker(ctx context.Context, name string, fn func(context.Context)) {
	for ctx.Err() == nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					observability.CapturePanic("worker:"+name, r)
				}
			}()
			fn(ctx)
		}()
		// fn returned. On a cancelled ctx that was a clean shutdown; otherwise it
		// panicked (recovered above) or exited early — pause so a tight panic loop
		// can't spin, then restart.
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// closer is the slice of *realtime.Hub that drain needs (kept as an interface so
// the ordering can be exercised without a WebSocket stack).
type closer interface{ Close() }

// drain shuts the process down in the one order that doesn't corrupt anything:
// HTTP first (in-flight requests still need the workers and the pool), then the
// workers at a tick boundary, then the hub. Reversed, a worker mid-sync would
// hit a closed pool halfway through a multi-step write.
//
// The timeout is a single budget shared by both waits — it is a bound on total
// shutdown, not per stage, because that is what an orchestrator's kill grace
// period actually measures.
func drain(srv *http.Server, workers *sync.WaitGroup, hub closer, hubRun *sync.WaitGroup, timeout time.Duration) {
	log.Printf("shutdown: draining (timeout %s)", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: http drain incomplete: %v", err)
	}

	drained := make(chan struct{})
	go func() {
		workers.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-ctx.Done():
		log.Printf("shutdown: workers still busy after %s, exiting anyway", timeout)
	}

	// Closing the hub last hands every browser a normal close frame, so clients
	// reconnect to the replacement process instead of reporting a broken socket.
	hub.Close()
	hubRun.Wait()
	log.Printf("shutdown: complete")
}
