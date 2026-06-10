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
	rh := handlers.NewAPI(queries, hub, cfg.UploadDir, cfg.EncryptionKey)

	// Background GitLab auto-sync worker (idle until an integration sets a
	// positive sync interval).
	go rh.RunSyncWorker(context.Background())

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

		// Inline images embedded in descriptions/comments are served publicly
		// (an <img> can't send the bearer header); unguessable by UUID filename.
		api.GET("/uploads/:name", rh.ServeUpload)

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
			protected.GET("/workspaces/:id/search", rh.Search)
			protected.GET("/workspaces/:id/tasks", rh.ListWorkspaceTasks)
			protected.GET("/workspaces/:id/summary", rh.WorkspaceSummary)
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

			// Notes (workspace-scoped).
			protected.POST("/workspaces/:id/notes", rh.CreateNote)
			protected.GET("/workspaces/:id/notes", rh.ListNotes)

			protected.PATCH("/groups/:id", rh.UpdateProjectGroup)
			protected.PATCH("/groups/:id/move", rh.MoveProjectGroup)
			protected.DELETE("/groups/:id", rh.DeleteProjectGroup)

			protected.GET("/projects/:id", rh.GetProject)
			protected.PATCH("/projects/:id", rh.UpdateProject)
			protected.PATCH("/projects/:id/move", rh.MoveProject)
			protected.DELETE("/projects/:id", rh.DeleteProject)
			protected.POST("/projects/:id/boards", rh.CreateBoard)
			protected.GET("/projects/:id/boards", rh.ListBoards)

			protected.GET("/boards/:id", rh.GetBoard)
			protected.PATCH("/boards/:id", rh.UpdateBoard)
			protected.PATCH("/boards/:id/done-column", rh.SetDoneColumn)
			protected.DELETE("/boards/:id", rh.DeleteBoard)
			protected.POST("/boards/:id/columns", rh.CreateColumn)
			protected.GET("/boards/:id/columns", rh.ListColumns)
			protected.POST("/boards/:id/tasks", rh.CreateTask)
			protected.GET("/boards/:id/tasks", rh.ListBoardTasks)
			protected.GET("/boards/:id/subtasks", rh.ListBoardSubtasks)
			protected.GET("/boards/:id/archive", rh.ListBoardArchived)

			protected.PATCH("/columns/:id", rh.UpdateColumn)
			protected.PATCH("/columns/:id/move", rh.MoveColumn)
			protected.DELETE("/columns/:id", rh.DeleteColumn)

			protected.GET("/tasks/:id", rh.GetTask)
			protected.PATCH("/tasks/:id", rh.UpdateTask)
			protected.PATCH("/tasks/:id/move", rh.MoveTask)
			protected.PATCH("/tasks/:id/parent", rh.SetTaskParent)
			protected.PATCH("/tasks/:id/transfer", rh.TransferTask)
			protected.PATCH("/tasks/:id/archive", rh.ArchiveTask)
			protected.PATCH("/tasks/:id/restore", rh.RestoreTask)
			protected.DELETE("/tasks/:id", rh.DeleteTask)
			protected.POST("/tasks/:id/tags", rh.AddTaskTag)
			protected.DELETE("/tasks/:id/tags/:tagId", rh.RemoveTaskTag)
			protected.POST("/tasks/:id/assignees", rh.AddTaskAssignee)
			protected.DELETE("/tasks/:id/assignees/:userId", rh.RemoveTaskAssignee)

			// Rich task detail: journal, comments, relations, attachments (#8).
			protected.GET("/tasks/:id/events", rh.ListTaskEvents)
			protected.GET("/tasks/:id/comments", rh.ListComments)
			protected.POST("/tasks/:id/comments", rh.CreateComment)
			protected.PATCH("/comments/:id", rh.UpdateComment)
			protected.DELETE("/comments/:id", rh.DeleteComment)
			protected.GET("/tasks/:id/relations", rh.ListRelations)
			protected.POST("/tasks/:id/relations", rh.AddRelation)
			protected.DELETE("/relations/:id", rh.DeleteRelation)
			protected.GET("/tasks/:id/attachments", rh.ListAttachments)
			protected.POST("/tasks/:id/attachments", rh.UploadAttachment)
			protected.GET("/attachments/:id/download", rh.DownloadAttachment)
			protected.DELETE("/attachments/:id", rh.DeleteAttachment)

			// Inline image upload for descriptions/comments (served via the
			// public /uploads/:name route above).
			protected.POST("/uploads", rh.UploadMedia)

			// Persistent notifications (#3).
			protected.GET("/notifications", rh.ListNotifications)
			protected.GET("/notifications/unread-count", rh.UnreadNotificationCount)
			protected.POST("/notifications/:id/read", rh.MarkNotificationRead)
			protected.POST("/notifications/read-all", rh.MarkAllNotificationsRead)

			protected.PATCH("/tags/:id", rh.UpdateTag)
			protected.DELETE("/tags/:id", rh.DeleteTag)

			protected.GET("/notes/:id", rh.GetNote)
			protected.PATCH("/notes/:id", rh.UpdateNote)
			protected.DELETE("/notes/:id", rh.DeleteNote)

			// GitLab integration: per-user connection (PAT), per-workspace
			// config + manual pull sync (Phase A, pull-only).
			protected.GET("/gitlab/connection", rh.GetGitlabConnection)
			protected.POST("/gitlab/connection", rh.ConnectGitlab)
			protected.DELETE("/gitlab/connection", rh.DisconnectGitlab)
			protected.GET("/workspaces/:id/gitlab/integration", rh.GetGitlabIntegration)
			protected.PUT("/workspaces/:id/gitlab/integration", rh.SetGitlabIntegration)
			protected.POST("/workspaces/:id/gitlab/sync", rh.SyncGitlab)

			// Reminders (personal).
			protected.POST("/reminders", rh.CreateReminder)
			protected.GET("/reminders", rh.ListReminders)
			protected.PATCH("/reminders/:id", rh.UpdateReminder)
			protected.DELETE("/reminders/:id", rh.DeleteReminder)
		}
	}

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err) //nolint:gocritic // nothing to clean up on listen failure
	}
}
