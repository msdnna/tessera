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
	rh := handlers.NewAPI(queries, hub)

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

			// Workspaces & membership.
			protected.POST("/workspaces", rh.CreateWorkspace)
			protected.GET("/workspaces", rh.ListWorkspaces)
			protected.GET("/workspaces/:id", rh.GetWorkspace)
			protected.PATCH("/workspaces/:id", rh.UpdateWorkspace)
			protected.DELETE("/workspaces/:id", rh.DeleteWorkspace)
			protected.GET("/workspaces/:id/members", rh.ListMembers)
			protected.POST("/workspaces/:id/members", rh.AddMember)
			protected.DELETE("/workspaces/:id/members/:userId", rh.RemoveMember)

			// Project groups & projects (nested under a workspace).
			protected.POST("/workspaces/:id/groups", rh.CreateProjectGroup)
			protected.GET("/workspaces/:id/groups", rh.ListProjectGroups)
			protected.POST("/workspaces/:id/projects", rh.CreateProject)
			protected.GET("/workspaces/:id/projects", rh.ListProjects)

			// Tags (workspace-scoped).
			protected.POST("/workspaces/:id/tags", rh.CreateTag)
			protected.GET("/workspaces/:id/tags", rh.ListTags)

			protected.PATCH("/groups/:id", rh.UpdateProjectGroup)
			protected.DELETE("/groups/:id", rh.DeleteProjectGroup)

			protected.GET("/projects/:id", rh.GetProject)
			protected.PATCH("/projects/:id", rh.UpdateProject)
			protected.DELETE("/projects/:id", rh.DeleteProject)
			protected.POST("/projects/:id/boards", rh.CreateBoard)
			protected.GET("/projects/:id/boards", rh.ListBoards)

			protected.GET("/boards/:id", rh.GetBoard)
			protected.PATCH("/boards/:id", rh.UpdateBoard)
			protected.DELETE("/boards/:id", rh.DeleteBoard)
			protected.POST("/boards/:id/columns", rh.CreateColumn)
			protected.GET("/boards/:id/columns", rh.ListColumns)
			protected.POST("/boards/:id/tasks", rh.CreateTask)
			protected.GET("/boards/:id/tasks", rh.ListBoardTasks)

			protected.PATCH("/columns/:id", rh.UpdateColumn)
			protected.PATCH("/columns/:id/move", rh.MoveColumn)
			protected.DELETE("/columns/:id", rh.DeleteColumn)

			protected.GET("/tasks/:id", rh.GetTask)
			protected.PATCH("/tasks/:id", rh.UpdateTask)
			protected.PATCH("/tasks/:id/move", rh.MoveTask)
			protected.DELETE("/tasks/:id", rh.DeleteTask)
			protected.POST("/tasks/:id/tags", rh.AddTaskTag)
			protected.DELETE("/tasks/:id/tags/:tagId", rh.RemoveTaskTag)
			protected.POST("/tasks/:id/assignees", rh.AddTaskAssignee)
			protected.DELETE("/tasks/:id/assignees/:userId", rh.RemoveTaskAssignee)

			protected.DELETE("/tags/:id", rh.DeleteTag)
		}
	}

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err) //nolint:gocritic // nothing to clean up on listen failure
	}
}
