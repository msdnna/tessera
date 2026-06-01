package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"tessera/config"
	"tessera/handlers"
	"tessera/internal/database"
	"tessera/internal/db"
	"tessera/internal/realtime"
	"tessera/middleware"
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

	versionHandler := handlers.NewVersionHandler(appVersion)
	wsHandler := handlers.NewWSHandler(hub)
	authHandler := handlers.NewAuthHandler(queries, cfg.JWTSecret)

	r := gin.Default()
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		log.Printf("Warning: failed to set trusted proxies: %v", err)
	}
	r.Use(middleware.CORS())

	api := r.Group("/api")
	{
		// Public — no auth required.
		api.GET("/health", healthHandler)
		api.GET("/version", versionHandler.Get)
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/refresh", authHandler.Refresh)

		// Live updates. Per-workspace scoping + WS auth land in a later phase.
		api.GET("/ws", wsHandler.Connect)

		// Protected — require a valid access token.
		protected := api.Group("/")
		protected.Use(middleware.Auth(cfg.JWTSecret))
		{
			protected.GET("/auth/me", authHandler.Me)
		}
	}

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err) //nolint:gocritic // nothing to clean up on listen failure
	}
}
