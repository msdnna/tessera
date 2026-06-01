// Command migrate applies the embedded SQL migrations against DATABASE_URL.
//
//	go run ./cmd/migrate            # apply all pending (up)
//	go run ./cmd/migrate -down 1    # roll back N steps
//	go run ./cmd/migrate -version   # print current schema version
package main

import (
	"errors"
	"flag"
	"log"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joho/godotenv"

	"tessera/config"
	"tessera/migrations"
)

func main() {
	_ = godotenv.Load()

	down := flag.Int("down", 0, "roll back N migration steps")
	showVersion := flag.Bool("version", false, "print current schema version and exit")
	flag.Parse()

	cfg := config.New()

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("load migrations: %v", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5(cfg.DatabaseURL))
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	defer m.Close()

	switch {
	case *showVersion:
		v, dirty, err := m.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			log.Fatalf("version: %v", err)
		}
		log.Printf("schema version: %d (dirty=%v)", v, dirty)
	case *down > 0:
		if err := m.Steps(-*down); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("down: %v", err)
		}
		log.Printf("rolled back %d step(s)", *down)
	default:
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("up: %v", err)
		}
		log.Println("migrations applied")
	}
}

// toPgx5 rewrites a postgres:// URL to the pgx5:// scheme that golang-migrate's
// pgx/v5 driver registers under.
func toPgx5(u string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(u, p) {
			return "pgx5://" + strings.TrimPrefix(u, p)
		}
	}
	return u
}
