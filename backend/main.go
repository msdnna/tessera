// Command tessera is the backend API server (gin + pgx + WebSocket hub).
package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"

	"tessera/config"
	"tessera/internal/database"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

func main() {
	_ = godotenv.Load()

	cfg := config.New()

	pool := database.Connect(context.Background(), cfg.DatabaseURL)
	defer pool.Close()

	queries := db.New(pool)

	// Realtime fan-out hub for live board updates.
	hub := realtime.NewHub()
	go hub.Run()

	// Email transport (U2 invites / verification / reset). No-op when SMTP is
	// unconfigured (self-host) — links are logged instead of sent.
	mailer := mail.New(mail.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser,
		Password: cfg.SMTPPass, From: cfg.SMTPFrom,
	})
	log.Printf("mail: enabled=%v", mailer.Enabled())

	r, rh := newRouter(cfg, queries, pool, hub, mailer)

	// Give boards/notes that predate the slug column a human-readable slug.
	rh.BackfillSlugs(context.Background())

	// Close sync runs left "running" by a process that died mid-sync — nothing of
	// ours is in flight yet, so they can only be stale.
	rh.FailStaleSyncRuns(context.Background())

	// Background GitLab auto-sync worker (idle until an integration sets a
	// positive sync interval).
	go rh.RunSyncWorker(context.Background())

	// Background GitLab write-back worker — drains the outbox of task changes to
	// push to linked issues. Idle until a user enables write-back on an integration.
	go rh.RunGitlabWriteBackWorker(context.Background())

	// Background notification delivery worker — drains the outbox of channel
	// deliveries (email/telegram/webhook). Idle until a user configures channels.
	go rh.RunNotificationWorker(context.Background())

	// Background scanner — emits due-date + reminder notifications on schedule.
	go rh.RunNotificationScanner(context.Background())

	// Background worker — advances schedule-triggered recurring tasks once due.
	// Idle until a task carries a "schedule"-trigger recurrence rule.
	go rh.RunRecurrenceWorker(context.Background())

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err) //nolint:gocritic // nothing to clean up on listen failure
	}
}
