// Router assembly, extracted from main() so tests can boot the full HTTP
// surface (httptest.Server) against a real database without the background
// workers.
package main

import (
	"log"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"tessera/config"
	"tessera/handlers"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/realtime"
	"tessera/middleware"
)

// Routes that carry a body budget of their own. Named as constants because the
// limits table below and the route registration further down are two places
// that must agree: a typo here would silently downgrade attachment uploads to
// the blanket 1 MiB, and nothing but a 413 in production would say so.
// TestBodyLimitRoutesExist checks every one of these against the built router.
const (
	routeWS          = "/api/ws"
	routeUploads     = "/api/uploads"
	routeAvatar      = "/api/users/me/avatar"
	routeAttachments = "/api/tasks/:id/attachments"
)

// authRateRules throttles the unauthenticated auth surface — the routes where a
// cheap anonymous request costs the server a bcrypt round, an outgoing email or
// a database write, and where an unthrottled attacker gets free credential
// stuffing. Sustained budgets: 10/min for the credential routes, 60/min for
// refresh (a busy client legitimately rotates far more often than it logs in).
//
// The credential routes are also keyed by email, so spreading the attempts
// across addresses still can't exceed the budget for one account.
func authRateRules() map[string]middleware.RateRule {
	credential := middleware.RateRule{Every: 6 * time.Second, Burst: 10, ByEmail: true}
	token := middleware.RateRule{Every: 6 * time.Second, Burst: 10}
	return map[string]middleware.RateRule{
		"/api/auth/login":           credential,
		"/api/auth/register":        credential,
		"/api/auth/forgot-password": credential,
		// No email in the body — these carry an opaque token, so IP is the only
		// key available.
		"/api/auth/reset-password": token,
		"/api/auth/verify-email":   token,
		"/api/auth/refresh":        {Every: time.Second, Burst: 60},
	}
}

// orDefault falls back when a limit is unset, so a Config built as a literal
// (tests, embedding) still gets the production ceilings instead of a zero that
// would reject every request with a body.
func orDefault(v, def int64) int64 {
	if v <= 0 {
		return def
	}
	return v
}

// newRouter wires every route of the API onto a fresh gin engine and returns
// it together with the resource-handler layer (main() reuses the latter for
// the background workers and slug backfill).
func newRouter(cfg *config.Config, queries *db.Queries, pool *pgxpool.Pool, hub *realtime.Hub, mailer mail.Mailer) (*gin.Engine, *handlers.API) {
	versionHandler := handlers.NewVersionHandler(appVersion)
	wsHandler := handlers.NewWSHandler(hub, queries, cfg.JWTSecret, append([]string{cfg.CORSOrigin}, cfg.DesktopOrigins...)...)
	authHandler := handlers.NewAuthHandler(queries, cfg.JWTSecret, cfg.EncryptionKey, mailer, cfg.PublicURL)
	rh := handlers.NewAPI(queries, pool, hub, cfg.UploadDir, cfg.EncryptionKey, mailer, cfg.PublicURL)

	// gin.Default wires gin.Logger, which prints the full request path + query
	// to stdout — so an OAuth callback's ?code=…&state=… landed in the container
	// log stream. gin.New with Recovery + AccessLog keeps panic recovery and the
	// access trace but redacts those secrets (see middleware/accesslog.go).
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.AccessLog(), gin.Recovery())
	trusted := cfg.TrustedProxies
	if len(trusted) == 0 {
		trusted = []string{"127.0.0.1", "::1"}
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		log.Printf("Warning: failed to set trusted proxies: %v", err)
	}
	// gin buffers a multipart upload in memory up to this and spills the rest to
	// disk. The stock 32 MiB is a per-request memory bill nobody asked for —
	// a handful of concurrent uploads would outweigh the whole process.
	r.MaxMultipartMemory = orDefault(cfg.MaxUploadBytes, config.DefaultMaxUploadBytes)
	// Compress JSON responses. The big list/journal payloads are highly
	// repetitive text and shrink ~10x, which is the difference between a snappy
	// board and a multi-second wait on a constrained link (the org install sits
	// behind a proxy that doesn't compress /api). The WebSocket endpoint is
	// excluded — its response is hijacked for the upgrade and must not be wrapped.
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{routeWS})))
	r.Use(middleware.CORS(cfg.CORSOrigin, cfg.DesktopOrigins...))

	// Cap request bodies before anything reads one. Ordering matters twice over:
	// this must precede the rate limiter (whose by-email keying reads the body,
	// and is only bounded because of this), and both must precede the handlers.
	r.Use(middleware.BodyLimit(orDefault(cfg.MaxBodyBytes, config.DefaultMaxBodyBytes), map[string]int64{
		routeAttachments: orDefault(cfg.MaxAttachmentBytes, config.DefaultMaxAttachmentBytes),
		routeUploads:     orDefault(cfg.MaxUploadBytes, config.DefaultMaxUploadBytes),
		routeAvatar:      orDefault(cfg.MaxUploadBytes, config.DefaultMaxUploadBytes),
		routeWS:          middleware.NoBodyLimit,
	}))
	if cfg.RateLimitEnabled {
		r.Use(middleware.RateLimit(authRateRules()))
	}

	api := r.Group("/api")
	{
		// Public — no auth required.
		api.GET("/health", healthHandler)
		api.GET("/version", versionHandler.Get)
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/refresh", authHandler.Refresh)
		// Token-based account flows (the token IS the auth): email verification +
		// password reset.
		api.POST("/auth/verify-email", authHandler.VerifyEmail)
		api.POST("/auth/forgot-password", authHandler.ForgotPassword)
		api.POST("/auth/reset-password", authHandler.ResetPassword)
		// GitLab OAuth ("Login with GitLab"): public discovery + redirect flow.
		api.GET("/auth/providers", authHandler.Providers)
		api.GET("/auth/gitlab/authorize", authHandler.GitlabAuthorize)
		api.GET("/auth/gitlab/callback", authHandler.GitlabCallback)

		// Live updates. Not in the protected group because the browser
		// WebSocket API can't send an Authorization header — the handler does
		// its own bearer check (header or subprotocol) before upgrading, and
		// scopes the socket to the user's workspaces.
		api.GET("/ws", wsHandler.Connect)

		// Inline images embedded in descriptions/comments are served publicly
		// (an <img> can't send the bearer header); unguessable by UUID filename.
		api.GET("/uploads/:name", rh.ServeUpload)

		// Signed proxy for GitLab attachments embedded in synced content
		// (public — an <img> can't send auth; HMAC-signed so only Tessera
		// links work, fetched with the integration owner's token).
		api.GET("/gitlab/asset", rh.GitlabAsset)
		api.GET("/gitlab/avatar", rh.GitlabAvatar)

		// Avatar blobs served publicly (an <img> can't send the bearer header);
		// keyed by user UUID, low-sensitivity.
		api.GET("/users/:id/avatar", rh.GetUserAvatar)

		// Protected — require a valid access token.
		protected := api.Group("/")
		protected.Use(middleware.Auth(cfg.JWTSecret, queries, cfg.PATTouchInterval))
		{
			protected.GET("/auth/me", authHandler.Me)
			protected.POST("/auth/resend-verification", authHandler.ResendVerification)

			// Personal access tokens: long-lived, revocable bearer credentials
			// for headless clients (MCP server, CI). Managed from a live session.
			protected.POST("/auth/tokens", rh.CreatePAT)
			protected.GET("/auth/tokens", rh.ListPATs)
			protected.DELETE("/auth/tokens/:id", rh.RevokePAT)

			// Self-service profile / password / preferences / avatar.
			protected.PATCH("/users/me", rh.UpdateMyProfile)
			protected.PUT("/users/me/password", rh.ChangeMyPassword)
			protected.PUT("/users/me/preferences", rh.UpdateMyPreferences)
			protected.PUT("/users/me/avatar", rh.UploadMyAvatar)
			protected.DELETE("/users/me/avatar", rh.DeleteMyAvatar)

			// Workspaces & membership.
			protected.POST("/workspaces", rh.CreateWorkspace)
			protected.GET("/workspaces", rh.ListWorkspaces)
			protected.GET("/workspaces/:id", rh.GetWorkspace)
			protected.PATCH("/workspaces/:id", rh.UpdateWorkspace)
			protected.DELETE("/workspaces/:id", rh.DeleteWorkspace)
			protected.GET("/workspaces/:id/search", rh.Search)
			protected.GET("/workspaces/:id/tasks", rh.ListWorkspaceTasks)
			protected.GET("/workspaces/:id/tasks/by-number/:number", rh.GetTaskByNumber)
			// Resolve a /project/<slug>/board/<slug> pair to its board.
			protected.GET("/board-by-slug", rh.ResolveBoardBySlug)
			protected.GET("/workspaces/:id/summary", rh.WorkspaceSummary)
			protected.GET("/workspaces/:id/members", rh.ListMembers)
			protected.POST("/workspaces/:id/members", rh.AddMember)
			protected.PATCH("/workspaces/:id/members/:userId", rh.UpdateMemberRole)
			protected.DELETE("/workspaces/:id/members/:userId", rh.RemoveMember)

			// Workspace invitations (invite-by-email; invitee may be unregistered).
			protected.POST("/workspaces/:id/invitations", rh.CreateInvitation)
			protected.GET("/workspaces/:id/invitations", rh.ListInvitations)
			protected.DELETE("/workspaces/:id/invitations/:invId", rh.DeleteInvitation)
			protected.POST("/invitations/accept", rh.AcceptInvitation)

			// Global admin panel (separate /admin prefix so it doesn't collide
			// with the /users/me routes). Each handler re-checks is_admin.
			protected.GET("/admin/users", rh.ListAllUsers)
			protected.PATCH("/admin/users/:id/active", rh.SetUserActive)
			protected.PATCH("/admin/users/:id/admin", rh.SetUserAdmin)
			protected.POST("/admin/users/:id/reset-link", rh.CreateUserResetLink)
			// GitLab OAuth app config (admin-only).
			protected.GET("/admin/oauth/gitlab", rh.GetOAuthConfig)
			protected.PUT("/admin/oauth/gitlab", rh.SetOAuthConfig)
			// Background jobs panel (admin-only): observe/run/cancel background work.
			protected.GET("/admin/jobs", rh.ListJobs)
			protected.GET("/admin/jobs/:key", rh.GetJob)
			protected.POST("/admin/jobs/:key/run", rh.RunJob)
			protected.POST("/admin/jobs/:key/cancel", rh.CancelJob)

			// Project groups & projects (nested under a workspace).
			protected.POST("/workspaces/:id/groups", rh.CreateProjectGroup)
			protected.GET("/workspaces/:id/groups", rh.ListProjectGroups)
			protected.POST("/workspaces/:id/projects", rh.CreateProject)
			protected.GET("/workspaces/:id/projects", rh.ListProjects)

			// Tags: project-scoped create/list; workspace-wide read for
			// cross-project views (Home).
			protected.POST("/projects/:id/tags", rh.CreateTag)
			protected.GET("/projects/:id/tags", rh.ListTags)
			protected.GET("/workspaces/:id/tags", rh.ListWorkspaceTags)
			// Tag-prefix display names (provider-neutral; GitLab modal is one editor).
			protected.GET("/projects/:id/tag-prefixes", rh.ListTagPrefixes)
			protected.PUT("/projects/:id/tag-prefixes", rh.SetTagPrefixes)
			protected.GET("/workspaces/:id/tag-prefixes", rh.ListWorkspaceTagPrefixes)

			// Quick-action registry: built-in commands + the workspace's custom
			// dictionary, for the markdown editor's "/" autocomplete.
			protected.GET("/workspaces/:id/commands", rh.ListWorkspaceCommands)
			protected.PUT("/workspaces/:id/commands", rh.SetWorkspaceCommands)

			// Two-level task-estimation config (workspace default + project override).
			protected.PUT("/workspaces/:id/estimation", rh.SetWorkspaceEstimation)
			protected.PUT("/projects/:id/estimation", rh.SetProjectEstimation)

			// Notes (workspace-scoped).
			protected.POST("/workspaces/:id/notes", rh.CreateNote)
			protected.GET("/workspaces/:id/notes", rh.ListNotes)

			protected.PATCH("/groups/:id", rh.UpdateProjectGroup)
			protected.PATCH("/groups/:id/move", rh.MoveProjectGroup)
			protected.DELETE("/groups/:id", rh.DeleteProjectGroup)

			protected.GET("/projects/:id", rh.GetProject)
			protected.PATCH("/projects/:id", rh.UpdateProject)
			protected.PATCH("/projects/:id/slug", rh.SetProjectSlug)
			protected.PATCH("/projects/:id/move", rh.MoveProject)
			protected.POST("/projects/:id/transfer", rh.TransferProject)
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
			protected.GET("/boards/:id/dependencies", rh.BoardDependencies)
			protected.GET("/boards/:id/views", rh.ListBoardViews)
			protected.POST("/boards/:id/views", rh.SaveBoardView)
			protected.DELETE("/views/:id", rh.DeleteBoardView)

			protected.PATCH("/columns/:id", rh.UpdateColumn)
			protected.PATCH("/columns/:id/move", rh.MoveColumn)
			protected.DELETE("/columns/:id", rh.DeleteColumn)

			protected.GET("/tasks/:id", rh.GetTask)
			protected.GET("/tasks/:id/description", rh.GetTaskDescription)
			protected.PATCH("/tasks/:id", rh.UpdateTask)
			protected.PATCH("/tasks/:id/move", rh.MoveTask)
			protected.PATCH("/tasks/:id/eisenhower", rh.SetTaskEisenhower)
			protected.PATCH("/tasks/:id/parent", rh.SetTaskParent)
			protected.PATCH("/tasks/:id/transfer", rh.TransferTask)
			protected.PATCH("/tasks/:id/archive", rh.ArchiveTask)
			protected.PATCH("/tasks/:id/restore", rh.RestoreTask)
			protected.DELETE("/tasks/:id", rh.DeleteTask)
			protected.POST("/tasks/:id/tags", rh.AddTaskTag)
			protected.DELETE("/tasks/:id/tags/:tagId", rh.RemoveTaskTag)
			protected.POST("/tasks/:id/assignees", rh.AddTaskAssignee)
			protected.DELETE("/tasks/:id/assignees/:userId", rh.RemoveTaskAssignee)
			protected.POST("/tasks/:id/gitlab-assignees", rh.PinTaskGitlabAssignee)
			protected.DELETE("/tasks/:id/gitlab-assignees/:username", rh.RemoveTaskGitlabAssignee)
			protected.POST("/tasks/:id/gitlab-issue", rh.CreateGitlabIssueFromTask)

			// Rich task detail: journal, comments, relations, attachments (#8).
			protected.GET("/tasks/:id/events", rh.ListTaskEvents)
			protected.GET("/tasks/:id/comments", rh.ListComments)
			protected.POST("/tasks/:id/comments", rh.CreateComment)
			// Dry-run of the quick actions in a draft comment, so the editor's
			// "Будет применено: …" hint comes from the same parser that executes.
			protected.POST("/tasks/:id/commands/preview", rh.PreviewCommands)
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

			// Notification router: per-user delivery channels + routing rules
			// (email/telegram/webhook). Top-level prefixes avoid colliding with
			// the /notifications/:id param route above.
			protected.GET("/notification-channels", rh.ListNotificationChannels)
			protected.POST("/notification-channels", rh.CreateNotificationChannel)
			protected.PATCH("/notification-channels/:id", rh.UpdateNotificationChannel)
			protected.DELETE("/notification-channels/:id", rh.DeleteNotificationChannel)
			protected.POST("/notification-channels/:id/test", rh.TestNotificationChannel)
			protected.POST("/notification-devices", rh.RegisterDeviceChannel)
			protected.POST("/notification-template-preview", rh.PreviewNotificationTemplate)
			protected.GET("/notification-prefs", rh.GetMyNotificationPrefs)
			protected.PUT("/notification-prefs", rh.UpdateMyNotificationPrefs)
			protected.PATCH("/tasks/:id/due-notify", rh.SetTaskDueNotify)
			protected.GET("/notification-routes", rh.ListNotificationRoutes)
			protected.POST("/notification-routes", rh.CreateNotificationRoute)
			protected.PATCH("/notification-routes/:id", rh.UpdateNotificationRoute)
			protected.DELETE("/notification-routes/:id", rh.DeleteNotificationRoute)

			protected.PATCH("/tags/:id", rh.UpdateTag)
			protected.DELETE("/tags/:id", rh.DeleteTag)

			// Milestones («Этап») — project-scoped planning unit.
			protected.GET("/workspaces/:id/milestones", rh.ListWorkspaceMilestones)
			protected.GET("/projects/:id/milestones", rh.ListMilestones)
			protected.POST("/projects/:id/milestones", rh.CreateMilestone)
			protected.PATCH("/milestones/:id", rh.UpdateMilestone)
			protected.DELETE("/milestones/:id", rh.DeleteMilestone)
			protected.POST("/tasks/:id/milestone", rh.SetTaskMilestone)
			protected.DELETE("/tasks/:id/milestone", rh.ClearTaskMilestone)
			protected.POST("/milestones/:id/gitlab", rh.PushMilestoneToGitlab)

			protected.GET("/notes/:id", rh.GetNote)
			protected.PATCH("/notes/:id", rh.UpdateNote)
			protected.DELETE("/notes/:id", rh.DeleteNote)

			// GitLab integration: per-user connection (PAT), per-workspace
			// config + manual pull sync (Phase A, pull-only).
			protected.GET("/gitlab/connection", rh.GetGitlabConnection)
			protected.POST("/gitlab/connection", rh.ConnectGitlab)
			protected.DELETE("/gitlab/connection", rh.DisconnectGitlab)
			// Multi-binding: a workspace can mirror several GitLab projects, each
			// into its own board.
			protected.GET("/workspaces/:id/gitlab/integrations", rh.ListGitlabIntegrations)
			protected.POST("/workspaces/:id/gitlab/integrations", rh.CreateGitlabIntegration)
			protected.PUT("/workspaces/:id/gitlab/integrations/:integrationId", rh.UpdateGitlabIntegration)
			protected.DELETE("/workspaces/:id/gitlab/integrations/:integrationId", rh.DeleteGitlabIntegration)
			protected.POST("/workspaces/:id/gitlab/integrations/:integrationId/sync", rh.SyncGitlab)
			protected.GET("/workspaces/:id/gitlab/members", rh.ListGitlabMembers)
			protected.GET("/workspaces/:id/gitlab/issue-templates", rh.ListGitlabIssueTemplates)
			// Sync journal: run/action history + retry of failed pushes.
			protected.GET("/workspaces/:id/gitlab/sync-runs", rh.ListGitlabSyncRuns)
			protected.GET("/workspaces/:id/gitlab/sync-runs/:runId/actions", rh.ListGitlabSyncActions)
			protected.GET("/workspaces/:id/gitlab/sync-runs/:runId/actions/:actionId/detail", rh.GetGitlabSyncActionDetail)
			protected.POST("/workspaces/:id/gitlab/sync-runs/:runId/actions/:actionId/retry", rh.RetryGitlabWriteback)
			// Write-back conflicts: inbox + interactive ours/theirs/manual resolution.
			protected.GET("/workspaces/:id/gitlab/conflicts", rh.ListGitlabConflicts)
			protected.POST("/tasks/:id/gitlab/conflicts/:conflictId/resolve", rh.ResolveGitlabConflict)

			// Reminders (personal).
			protected.POST("/reminders", rh.CreateReminder)
			protected.GET("/reminders", rh.ListReminders)
			protected.PATCH("/reminders/:id", rh.UpdateReminder)
			protected.DELETE("/reminders/:id", rh.DeleteReminder)
		}
	}

	return r, rh
}
