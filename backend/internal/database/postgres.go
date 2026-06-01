package database

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx connection pool, retrying so the backend can start before
// Postgres is ready. The compose healthcheck covers the happy path; this makes
// host-run dev resilient to a DB that's still booting.
func Connect(ctx context.Context, url string) *pgxpool.Pool {
	for i := 0; i < 10; i++ {
		pool, err := pgxpool.New(ctx, url)
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
