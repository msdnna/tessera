// Command token mints a personal access token for a user, bypassing the HTTP
// layer. It exists to bootstrap headless clients (the MCP server) locally
// without a browser session or a frontend UI.
//
//	go run ./cmd/token -email you@example.com -name mcp
//	go run ./cmd/token -email you@example.com -name ci -days 90
//
// The plaintext token is printed once — store it now; only its hash is kept.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"tessera/config"
	"tessera/internal/auth"
	"tessera/internal/database"
	"tessera/internal/db"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	_ = godotenv.Load()

	email := flag.String("email", "", "email of the user to mint a token for (required)")
	name := flag.String("name", "mcp", "human-readable label for the token")
	days := flag.Int("days", 0, "expiry in days (0 = never expires)")
	flag.Parse()

	if *email == "" {
		return fmt.Errorf("-email is required")
	}

	cfg := config.New()
	pool := database.Connect(context.Background(), cfg.DatabaseURL)
	defer pool.Close()
	q := db.New(pool)

	ctx := context.Background()
	user, err := q.GetUserByEmail(ctx, strings.ToLower(*email))
	if err != nil {
		return fmt.Errorf("no user with email %q: %w", *email, err)
	}

	token, hash, err := auth.NewPAT()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	var expires *time.Time
	if *days > 0 {
		t := time.Now().Add(time.Duration(*days) * 24 * time.Hour)
		expires = &t
	}

	if _, err := q.CreatePAT(ctx, db.CreatePATParams{
		UserID:    user.ID,
		Name:      *name,
		TokenHash: hash,
		LastFour:  token[len(token)-4:],
		ExpiresAt: expires,
	}); err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	exp := "never"
	if expires != nil {
		exp = expires.Format(time.RFC3339)
	}
	fmt.Printf("Personal access token for %s (%s), expires: %s\n", user.Email, *name, exp)
	fmt.Println("Store it now — it will not be shown again:")
	fmt.Println()
	fmt.Println("  " + token)
	fmt.Println()
	return nil
}
