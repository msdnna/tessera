// Package database opens and configures the pgx connection pool.
package database

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxuuid "github.com/vgarvardt/pgx-google-uuid/v5"
)

// Connect opens a pgx connection pool, retrying so the backend can start before
// Postgres is ready. The compose healthcheck covers the happy path; this makes
// host-run dev resilient to a DB that's still booting.
//
// Every connection registers the google/uuid codec so sqlc-generated
// uuid.UUID fields scan/encode directly (sqlc maps Postgres uuid → uuid.UUID).
func Connect(ctx context.Context, url string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		log.Fatalf("invalid DATABASE_URL: %v", err)
	}
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxuuid.Register(conn.TypeMap())
		return nil
	}

	// Pool sizing. Explicit pool_* params in DATABASE_URL still win (ParseConfig
	// already applied them); we only fill sane defaults where the URL was silent.
	// MinConns keeps a few connections warm so the first burst of a page load
	// doesn't pay per-request connect+TLS+auth latency (a big share of the
	// "first wave is slow, second is fast" the org install shows). MaxConns floors
	// the pool high enough for the ~10 parallel calls a board open fires at once.
	if cfg.MaxConns < 20 {
		cfg.MaxConns = 20
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = 4
	}
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	for i := 0; i < 10; i++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			log.Printf("Postgres pool init attempt %d failed: %v", i+1, err)
			time.Sleep(3 * time.Second)
			continue
		}

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			log.Println("Connected to Postgres")
			return pool
		}

		log.Printf("Postgres ping attempt %d failed: %v", i+1, err)
		pool.Close()
		time.Sleep(3 * time.Second)
	}

	log.Fatal("Failed to connect to Postgres after 10 attempts")
	return nil
}
