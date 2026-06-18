package website.msdnna.tessera.data.api

import okhttp3.MultipartBody
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.Multipart
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Part
import retrofit2.http.Path
import website.msdnna.tessera.data.model.AddAssigneeRequest
import website.msdnna.tessera.data.model.AddRelationRequest
import website.msdnna.tessera.data.model.AddTagRequest
import website.msdnna.tessera.data.model.AdminUser
import website.msdnna.tessera.data.model.Attachment
import website.msdnna.tessera.data.model.AuthResponse
import website.msdnna.tessera.data.model.AvatarResponse
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.BoardView
import website.msdnna.tessera.data.model.Comment
import website.msdnna.tessera.data.model.CreateCommentRequest
import website.msdnna.tessera.data.model.CreateGroupRequest
import website.msdnna.tessera.data.model.CreateProjectRequest
import website.msdnna.tessera.data.model.CreateTagRequest
import website.msdnna.tessera.data.model.CreateTaskRequest
import website.msdnna.tessera.data.model.EmailRequest
import website.msdnna.tessera.data.model.Invitation
import website.msdnna.tessera.data.model.InviteRequest
import website.msdnna.tessera.data.model.LinkResponse
import website.msdnna.tessera.data.model.LoginRequest
import website.msdnna.tessera.data.model.MeResponse
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.MoveTaskRequest
import website.msdnna.tessera.data.model.NameRequest
import website.msdnna.tessera.data.model.PasswordChange
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.model.ProfileUpdate
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.ProjectGroup
import website.msdnna.tessera.data.model.RefreshRequest
import website.msdnna.tessera.data.model.RegisterRequest
import website.msdnna.tessera.data.model.Relation
import website.msdnna.tessera.data.model.ResetPasswordRequest
import website.msdnna.tessera.data.model.RoleUpdate
import website.msdnna.tessera.data.model.SaveBoardViewRequest
import website.msdnna.tessera.data.model.SetActiveRequest
import website.msdnna.tessera.data.model.SetAdminRequest
import website.msdnna.tessera.data.model.SetParentRequest
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.model.TaskDetail
import website.msdnna.tessera.data.model.TaskEvent
import website.msdnna.tessera.data.model.TokenRequest
import website.msdnna.tessera.data.model.UpdateCommentRequest
import website.msdnna.tessera.data.model.UpdateGroupRequest
import website.msdnna.tessera.data.model.UpdateProjectRequest
import website.msdnna.tessera.data.model.UpdateTaskRequest
import website.msdnna.tessera.data.model.UploadResponse
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.data.model.Workspace

/**
 * Retrofit interface for the Tessera backend. Paths are relative to
 * `<serverUrl>/api/` (see [RetrofitClient.getService]). The interface grows
 * per feature phase; auth lives here from the start.
 */
interface ApiService {
    @POST("auth/login")
    suspend fun login(@Body body: LoginRequest): AuthResponse

    @POST("auth/register")
    suspend fun register(@Body body: RegisterRequest): AuthResponse

    @POST("auth/refresh")
    suspend fun refresh(@Body body: RefreshRequest): AuthResponse

    @GET("auth/me")
    suspend fun me(): MeResponse

    // ── self-service profile / preferences / avatar (U1) ──
    @PATCH("users/me")
    suspend fun updateProfile(@Body body: ProfileUpdate): User

    @PUT("users/me/password")
    suspend fun changePassword(@Body body: PasswordChange)

    @PUT("users/me/preferences")
    suspend fun updatePreferences(@Body body: Preferences): Preferences

    @Multipart
    @PUT("users/me/avatar")
    suspend fun uploadAvatar(@Part file: MultipartBody.Part): AvatarResponse

    @DELETE("users/me/avatar")
    suspend fun deleteAvatar()

    // ── account lifecycle (U2): verification, password reset ──
    @POST("auth/verify-email")
    suspend fun verifyEmail(@Body body: TokenRequest)

    @POST("auth/resend-verification")
    suspend fun resendVerification()

    @POST("auth/forgot-password")
    suspend fun forgotPassword(@Body body: EmailRequest)

    @POST("auth/reset-password")
    suspend fun resetPassword(@Body body: ResetPasswordRequest)

    // ── global admin panel (U3); every endpoint re-checks is_admin server-side ──
    @GET("admin/users")
    suspend fun adminUsers(): List<AdminUser>?

    @PATCH("admin/users/{id}/active")
    suspend fun adminSetActive(@Path("id") id: String, @Body body: SetActiveRequest)

    @PATCH("admin/users/{id}/admin")
    suspend fun adminSetAdmin(@Path("id") id: String, @Body body: SetAdminRequest)

    @POST("admin/users/{id}/reset-link")
    suspend fun adminResetLink(@Path("id") id: String): LinkResponse

    // ── Workspaces ──────────────────────────────────────────────────────────
    @GET("workspaces")
    suspend fun workspaces(): List<Workspace>?

    @POST("workspaces")
    suspend fun createWorkspace(@Body body: NameRequest): Workspace

    // ── Project groups ──────────────────────────────────────────────────────
    @GET("workspaces/{id}/groups")
    suspend fun groups(@Path("id") workspaceId: String): List<ProjectGroup>?

    @POST("workspaces/{id}/groups")
    suspend fun createGroup(@Path("id") workspaceId: String, @Body body: CreateGroupRequest): ProjectGroup

    @PATCH("groups/{id}")
    suspend fun updateGroup(@Path("id") groupId: String, @Body body: UpdateGroupRequest): ProjectGroup

    @PATCH("groups/{id}/move")
    suspend fun moveGroup(
        @Path("id") groupId: String,
        @Body body: website.msdnna.tessera.data.model.MoveGroupRequest,
    ): ProjectGroup

    @DELETE("groups/{id}")
    suspend fun deleteGroup(@Path("id") groupId: String)

    // ── Projects ────────────────────────────────────────────────────────────
    @GET("workspaces/{id}/projects")
    suspend fun projects(@Path("id") workspaceId: String): List<Project>?

    @POST("workspaces/{id}/projects")
    suspend fun createProject(@Path("id") workspaceId: String, @Body body: CreateProjectRequest): Project

    @PATCH("projects/{id}")
    suspend fun updateProject(@Path("id") projectId: String, @Body body: UpdateProjectRequest): Project

    @PATCH("projects/{id}/move")
    suspend fun moveProject(
        @Path("id") projectId: String,
        @Body body: website.msdnna.tessera.data.model.MoveProjectRequest,
    ): Project

    @DELETE("projects/{id}")
    suspend fun deleteProject(@Path("id") projectId: String)

    // ── Boards ──────────────────────────────────────────────────────────────
    @GET("projects/{id}/boards")
    suspend fun boards(@Path("id") projectId: String): List<Board>?

    @POST("projects/{id}/boards")
    suspend fun createBoard(@Path("id") projectId: String, @Body body: NameRequest): Board

    @GET("boards/{id}")
    suspend fun board(@Path("id") boardId: String): Board

    @PATCH("boards/{id}")
    suspend fun updateBoard(@Path("id") boardId: String, @Body body: NameRequest): Board

    @DELETE("boards/{id}")
    suspend fun deleteBoard(@Path("id") boardId: String)

    @GET("boards/{id}/columns")
    suspend fun columns(@Path("id") boardId: String): List<BoardColumn>?

    @POST("boards/{id}/columns")
    suspend fun createColumn(@Path("id") boardId: String, @Body body: website.msdnna.tessera.data.model.CreateColumnRequest): BoardColumn

    @PATCH("columns/{id}")
    suspend fun updateColumn(@Path("id") columnId: String, @Body body: website.msdnna.tessera.data.model.UpdateColumnRequest): BoardColumn

    @DELETE("columns/{id}")
    suspend fun deleteColumn(@Path("id") columnId: String)

    @PATCH("columns/{id}/move")
    suspend fun moveColumn(
        @Path("id") columnId: String,
        @Body body: website.msdnna.tessera.data.model.ColumnMoveRequest,
    ): BoardColumn

    @PATCH("boards/{id}/done-column")
    suspend fun setDoneColumn(
        @Path("id") boardId: String,
        @Body body: website.msdnna.tessera.data.model.SetDoneColumnRequest,
    ): Board

    // ── Tasks ───────────────────────────────────────────────────────────────
    @GET("tasks/{id}")
    suspend fun task(@Path("id") taskId: String): TaskDetail

    @GET("boards/{id}/tasks")
    suspend fun boardTasks(@Path("id") boardId: String): List<Task>?

    @GET("boards/{id}/subtasks")
    suspend fun boardSubtasks(@Path("id") boardId: String): List<Task>?

    @POST("boards/{id}/tasks")
    suspend fun createTask(@Path("id") boardId: String, @Body body: CreateTaskRequest): Task

    @PATCH("tasks/{id}/move")
    suspend fun moveTask(@Path("id") taskId: String, @Body body: MoveTaskRequest): Task

    @PATCH("tasks/{id}")
    suspend fun updateTask(@Path("id") taskId: String, @Body body: UpdateTaskRequest): Task

    @PATCH("tasks/{id}/parent")
    suspend fun setTaskParent(@Path("id") taskId: String, @Body body: SetParentRequest): Task

    @PATCH("tasks/{id}/archive")
    suspend fun archiveTask(@Path("id") taskId: String)

    @DELETE("tasks/{id}")
    suspend fun deleteTask(@Path("id") taskId: String)

    @POST("tasks/{id}/tags")
    suspend fun addTaskTag(@Path("id") taskId: String, @Body body: AddTagRequest)

    @DELETE("tasks/{id}/tags/{tagId}")
    suspend fun removeTaskTag(@Path("id") taskId: String, @Path("tagId") tagId: String)

    @POST("tasks/{id}/assignees")
    suspend fun addTaskAssignee(@Path("id") taskId: String, @Body body: AddAssigneeRequest)

    @DELETE("tasks/{id}/assignees/{userId}")
    suspend fun removeTaskAssignee(@Path("id") taskId: String, @Path("userId") userId: String)

    // ── Tags (project-scoped) / members (workspace-scoped) ──────────────────
    @GET("projects/{id}/tags")
    suspend fun tags(@Path("id") projectId: String): List<Tag>?

    @POST("projects/{id}/tags")
    suspend fun createTag(@Path("id") projectId: String, @Body body: CreateTagRequest): Tag

    @GET("workspaces/{id}/members")
    suspend fun members(@Path("id") workspaceId: String): List<Member>?

    @POST("workspaces/{id}/members")
    suspend fun addMember(
        @Path("id") workspaceId: String,
        @Body body: website.msdnna.tessera.data.model.AddMemberRequest,
    )

    @PATCH("workspaces/{id}/members/{userId}")
    suspend fun updateMemberRole(
        @Path("id") workspaceId: String,
        @Path("userId") userId: String,
        @Body body: RoleUpdate,
    )

    @DELETE("workspaces/{id}/members/{userId}")
    suspend fun removeMember(@Path("id") workspaceId: String, @Path("userId") userId: String)

    // ── invitations (U2) ──
    @GET("workspaces/{id}/invitations")
    suspend fun invitations(@Path("id") workspaceId: String): List<Invitation>?

    @POST("workspaces/{id}/invitations")
    suspend fun createInvitation(@Path("id") workspaceId: String, @Body body: InviteRequest): Invitation

    @DELETE("workspaces/{id}/invitations/{invId}")
    suspend fun deleteInvitation(@Path("id") workspaceId: String, @Path("invId") invId: String)

    @POST("invitations/accept")
    suspend fun acceptInvitation(@Body body: TokenRequest): Workspace

    @PATCH("tags/{id}")
    suspend fun updateTag(@Path("id") tagId: String, @Body body: CreateTagRequest): Tag

    @DELETE("tags/{id}")
    suspend fun deleteTag(@Path("id") tagId: String)

    // ── Saved board views (per-user, server-side) ────────────────────────────
    @GET("boards/{id}/views")
    suspend fun boardViews(@Path("id") boardId: String): List<BoardView>?

    @POST("boards/{id}/views")
    suspend fun saveBoardView(@Path("id") boardId: String, @Body body: SaveBoardViewRequest): BoardView

    @DELETE("views/{id}")
    suspend fun deleteBoardView(@Path("id") viewId: String)

    // ── Archive (#7) ──────────────────────────────────────────────────────────
    @GET("boards/{id}/archive")
    suspend fun boardArchive(@Path("id") boardId: String): List<Task>?

    @PATCH("tasks/{id}/restore")
    suspend fun restoreTask(@Path("id") taskId: String)

    // ── Transfer task to another board (#8) ──────────────────────────────────
    @PATCH("tasks/{id}/transfer")
    suspend fun transferTask(
        @Path("id") taskId: String,
        @Body body: website.msdnna.tessera.data.model.TransferTaskRequest,
    ): Task

    // ── Attachment download (#8) ──────────────────────────────────────────────
    @retrofit2.http.Streaming
    @GET("attachments/{id}/download")
    suspend fun downloadAttachment(@Path("id") attachmentId: String): okhttp3.ResponseBody

    // ── Task detail: comments / relations / attachments / journal (phase 5) ──
    @GET("tasks/{id}/comments")
    suspend fun comments(@Path("id") taskId: String): List<Comment>?

    @POST("tasks/{id}/comments")
    suspend fun createComment(@Path("id") taskId: String, @Body body: CreateCommentRequest): Comment

    @PATCH("comments/{id}")
    suspend fun updateComment(@Path("id") commentId: String, @Body body: UpdateCommentRequest): Comment

    @DELETE("comments/{id}")
    suspend fun deleteComment(@Path("id") commentId: String)

    @GET("tasks/{id}/relations")
    suspend fun relations(@Path("id") taskId: String): List<Relation>?

    @POST("tasks/{id}/relations")
    suspend fun addRelation(@Path("id") taskId: String, @Body body: AddRelationRequest)

    @DELETE("relations/{id}")
    suspend fun deleteRelation(@Path("id") relationId: String)

    @GET("tasks/{id}/attachments")
    suspend fun attachments(@Path("id") taskId: String): List<Attachment>?

    @Multipart
    @POST("tasks/{id}/attachments")
    suspend fun uploadAttachment(@Path("id") taskId: String, @Part file: MultipartBody.Part): Attachment

    @DELETE("attachments/{id}")
    suspend fun deleteAttachment(@Path("id") attachmentId: String)

    @GET("tasks/{id}/events")
    suspend fun events(@Path("id") taskId: String): List<TaskEvent>?

    @Multipart
    @POST("uploads")
    suspend fun uploadMedia(@Part file: MultipartBody.Part): UploadResponse

    // ── Notifications (#1) ────────────────────────────────────────────────────
    @GET("notifications")
    suspend fun notifications(): List<website.msdnna.tessera.data.model.Notification>?

    @GET("notifications/unread-count")
    suspend fun unreadCount(): website.msdnna.tessera.data.model.UnreadCount

    @POST("notifications/{id}/read")
    suspend fun markNotificationRead(@Path("id") notificationId: String)

    @POST("notifications/read-all")
    suspend fun markAllNotificationsRead()

    // ── Notification router: channels / routes / prefs / device ──────────────
    @GET("notification-channels")
    suspend fun notificationChannels(): List<website.msdnna.tessera.data.model.NotificationChannel>?

    @POST("notification-channels")
    suspend fun createNotificationChannel(
        @Body body: website.msdnna.tessera.data.model.ChannelRequest,
    ): website.msdnna.tessera.data.model.NotificationChannel

    @PATCH("notification-channels/{id}")
    suspend fun updateNotificationChannel(
        @Path("id") id: String,
        @Body body: website.msdnna.tessera.data.model.ChannelRequest,
    ): website.msdnna.tessera.data.model.NotificationChannel

    @DELETE("notification-channels/{id}")
    suspend fun deleteNotificationChannel(@Path("id") id: String)

    @POST("notification-channels/{id}/test")
    suspend fun testNotificationChannel(
        @Path("id") id: String,
    ): website.msdnna.tessera.data.model.TestChannelResult

    @POST("notification-devices")
    suspend fun registerDevice(
        @Body body: website.msdnna.tessera.data.model.RegisterDeviceRequest,
    ): website.msdnna.tessera.data.model.NotificationChannel

    @POST("notification-template-preview")
    suspend fun previewNotificationTemplate(
        @Body body: website.msdnna.tessera.data.model.TemplatePreviewRequest,
    ): website.msdnna.tessera.data.model.TemplatePreviewResult

    @GET("notification-routes")
    suspend fun notificationRoutes(): List<website.msdnna.tessera.data.model.NotificationRoute>?

    @POST("notification-routes")
    suspend fun createNotificationRoute(
        @Body body: website.msdnna.tessera.data.model.RouteRequest,
    ): website.msdnna.tessera.data.model.NotificationRoute

    @PATCH("notification-routes/{id}")
    suspend fun updateNotificationRoute(
        @Path("id") id: String,
        @Body body: website.msdnna.tessera.data.model.RouteRequest,
    ): website.msdnna.tessera.data.model.NotificationRoute

    @DELETE("notification-routes/{id}")
    suspend fun deleteNotificationRoute(@Path("id") id: String)

    @GET("notification-prefs")
    suspend fun notificationPrefs(): website.msdnna.tessera.data.model.NotificationPrefs

    @PUT("notification-prefs")
    suspend fun updateNotificationPrefs(
        @Body body: website.msdnna.tessera.data.model.NotificationPrefs,
    ): website.msdnna.tessera.data.model.NotificationPrefs

    // ── Reminders (#2) ────────────────────────────────────────────────────────
    @GET("reminders")
    suspend fun reminders(): List<website.msdnna.tessera.data.model.Reminder>?

    @POST("reminders")
    suspend fun createReminder(
        @Body body: website.msdnna.tessera.data.model.CreateReminderRequest,
    ): website.msdnna.tessera.data.model.Reminder

    @PATCH("reminders/{id}")
    suspend fun updateReminder(
        @Path("id") reminderId: String,
        @Body body: website.msdnna.tessera.data.model.UpdateReminderRequest,
    ): website.msdnna.tessera.data.model.Reminder

    @DELETE("reminders/{id}")
    suspend fun deleteReminder(@Path("id") reminderId: String)

    // ── Global search (#3) ────────────────────────────────────────────────────
    @GET("workspaces/{id}/search")
    suspend fun search(
        @Path("id") workspaceId: String,
        @retrofit2.http.Query("q") query: String,
    ): website.msdnna.tessera.data.model.SearchResults

    // ── Home dashboard (#4) ───────────────────────────────────────────────────
    @GET("workspaces/{id}/tasks")
    suspend fun workspaceTasks(
        @Path("id") workspaceId: String,
        @retrofit2.http.Query("assignee") assignee: String? = null,
    ): List<website.msdnna.tessera.data.model.WorkspaceTask>?

    @GET("workspaces/{id}/summary")
    suspend fun workspaceSummary(
        @Path("id") workspaceId: String,
    ): website.msdnna.tessera.data.model.WorkspaceSummary

    // ── Notes (#5) ────────────────────────────────────────────────────────────
    @GET("workspaces/{id}/notes")
    suspend fun notes(@Path("id") workspaceId: String): List<website.msdnna.tessera.data.model.Note>?

    @POST("workspaces/{id}/notes")
    suspend fun createNote(
        @Path("id") workspaceId: String,
        @Body body: website.msdnna.tessera.data.model.CreateNoteRequest,
    ): website.msdnna.tessera.data.model.Note

    @GET("notes/{id}")
    suspend fun note(@Path("id") noteId: String): website.msdnna.tessera.data.model.Note

    @PATCH("notes/{id}")
    suspend fun updateNote(
        @Path("id") noteId: String,
        @Body body: website.msdnna.tessera.data.model.UpdateNoteRequest,
    ): website.msdnna.tessera.data.model.Note

    @DELETE("notes/{id}")
    suspend fun deleteNote(@Path("id") noteId: String)

    // ── GitLab integration ────────────────────────────────────────────────────
    @GET("gitlab/connection")
    suspend fun gitlabConnection(): website.msdnna.tessera.data.model.GitlabConnection

    @POST("gitlab/connection")
    suspend fun gitlabConnect(
        @Body body: website.msdnna.tessera.data.model.GitlabConnectRequest,
    ): website.msdnna.tessera.data.model.GitlabConnection

    @DELETE("gitlab/connection")
    suspend fun gitlabDisconnect()

    @GET("workspaces/{id}/gitlab/integration")
    suspend fun gitlabIntegration(
        @Path("id") workspaceId: String,
    ): website.msdnna.tessera.data.model.GitlabIntegration

    @PUT("workspaces/{id}/gitlab/integration")
    suspend fun gitlabSetIntegration(
        @Path("id") workspaceId: String,
        @Body body: website.msdnna.tessera.data.model.GitlabSetIntegrationRequest,
    ): website.msdnna.tessera.data.model.GitlabIntegration

    @POST("workspaces/{id}/gitlab/sync")
    suspend fun gitlabSync(
        @Path("id") workspaceId: String,
    ): website.msdnna.tessera.data.model.GitlabSyncResult
}
