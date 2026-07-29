// Command tessera-mcp is a Model Context Protocol server exposing a Tessera
// workspace to an AI agent as a priority-ranked queue of tasks.
//
// It authenticates to the Tessera REST API with a Personal Access Token and
// speaks MCP over stdio, so an MCP client (e.g. Claude Code) launches it as a
// subprocess. Configuration is via environment variables:
//
//	TESSERA_BASE_URL   API base URL (default http://localhost:8090/api)
//	TESSERA_TOKEN      personal access token (tsra_…), required
//
// Mint a token locally with:  go run ./backend/cmd/token -email you@example.com -name mcp
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/tools"
)

// version is stamped from the VERSION file at build time (see Makefile); the
// default keeps `go run` working without ldflags.
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("tessera-mcp: %v", err)
	}
}

func run() error {
	baseURL := envOr("TESSERA_BASE_URL", "http://localhost:8090/api")
	token := os.Getenv("TESSERA_TOKEN")
	if token == "" {
		return fmt.Errorf("TESSERA_TOKEN is required (mint one: go run ./backend/cmd/token -email you@example.com -name mcp)")
	}

	c := client.New(baseURL, token)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "tessera",
		Title:   "Tessera",
		Version: version,
	}, nil)
	tools.Register(server, c)

	// Terminate cleanly on Ctrl-C / SIGTERM so the transport closes.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return server.Run(ctx, &mcp.StdioTransport{})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
