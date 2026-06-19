package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * Full task detail — mirrors the backend `taskDetail` returned by
 * `GET /tasks/:id`: the task itself plus its tags, assignees and direct
 * subtasks (with meta, for the subtasks tab).
 */
data class TaskDetail(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("column_id") val columnId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("start_date") val startDate: String? = null,
    @SerializedName("estimate") val estimate: Double? = null,
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("completed_at") val completedAt: String? = null,
    @SerializedName("number") val number: Long? = null,
    @SerializedName("recurrence") val recurrence: Recurrence? = null,
    @SerializedName("due_lead_minutes") val dueLeadMinutes: Int? = null,
    @SerializedName("due_repeat_minutes") val dueRepeatMinutes: Int? = null,
    @SerializedName("due_notify_enabled") val dueNotifyEnabled: Boolean? = null,
    // Nullable: the backend serialises empty slices as JSON `null`, and Gson
    // ignores Kotlin defaults — so these arrive null when the task has none.
    // Always read them through the non-null accessors below.
    @SerializedName("tags") private val tagsRaw: List<Tag>? = null,
    @SerializedName("assignees") private val assigneesRaw: List<AssigneeUser>? = null,
    @SerializedName("gitlab_assignees") private val gitlabAssigneesRaw: List<GitlabAssignee>? = null,
    @SerializedName("subtasks") private val subtasksRaw: List<Task>? = null,
    // GitLab provenance — present when this task mirrors a GitLab issue.
    @SerializedName("gitlab") val gitlab: GitlabLink? = null,
) {
    val isCompleted: Boolean get() = completedAt != null
    val tags: List<Tag> get() = tagsRaw.orEmpty()
    val assignees: List<AssigneeUser> get() = assigneesRaw.orEmpty()
    val gitlabAssignees: List<GitlabAssignee> get() = gitlabAssigneesRaw.orEmpty()
    val subtasks: List<Task> get() = subtasksRaw.orEmpty()

    /** A lightweight [Task] view of this detail, for reusing card-level pickers. */
    fun asTask(): Task = Task(
        id = id, boardId = boardId, columnId = columnId, parentId = parentId,
        title = title, description = description, priority = priority, dueDate = dueDate,
        startDate = startDate, estimate = estimate,
        createdBy = createdBy, completedAt = completedAt, number = number, recurrence = recurrence,
        tagIds = tags.map { it.id }, assigneeIds = assignees.map { it.id },
    )
}

/** A task assignee — mirrors `ListTaskAssigneesRow`. */
data class AssigneeUser(
    @SerializedName("id") val id: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
)

/** GitLab provenance attached to a synced task (issue number, link, author). */
data class GitlabLink(
    @SerializedName("iid") val iid: Long = 0,
    @SerializedName("web_url") val webUrl: String = "",
    @SerializedName("author") val author: String = "",
    @SerializedName("author_name") val authorName: String = "",
    @SerializedName("author_avatar_url") val authorAvatarUrl: String = "",
    @SerializedName("project_path") val projectPath: String = "",
)

/** An external GitLab assignee (no Tessera account) — display-only. */
data class GitlabAssignee(
    @SerializedName("gl_username") val glUsername: String = "",
    @SerializedName("gl_name") val glName: String = "",
    @SerializedName("gl_avatar_url") val glAvatarUrl: String = "",
)

/** A task comment — mirrors `ListTaskCommentsRow`. */
data class Comment(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("body") val body: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("author_name") val authorName: String? = null,
    @SerializedName("author_email") val authorEmail: String? = null,
    // GitLab note author (when the comment was synced from GitLab; author_id null).
    @SerializedName("gl_author_login") val glAuthorLogin: String? = null,
    @SerializedName("gl_author_name") val glAuthorName: String? = null,
    @SerializedName("gl_author_avatar_url") val glAuthorAvatarUrl: String? = null,
) {
    /** Display name: the Tessera author, else the GitLab note author. */
    val displayName: String? get() = authorName ?: glAuthorName

    /** True when this comment came from GitLab (no local author). */
    val isGitlab: Boolean get() = authorId == null && glAuthorName != null
}

/** A task relation (referenced by #N) — mirrors `ListTaskRelationsRow`. */
data class Relation(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("related_task_id") val relatedTaskId: String = "",
    @SerializedName("kind") val kind: String = "relates",
    @SerializedName("related_number") val relatedNumber: Long? = null,
    @SerializedName("related_title") val relatedTitle: String = "",
    @SerializedName("related_board_id") val relatedBoardId: String = "",
    @SerializedName("related_completed_at") val relatedCompletedAt: String? = null,
    @SerializedName("related_archived_at") val relatedArchivedAt: String? = null,
)

/**
 * One blocking edge of a board's dependency graph (`GET /boards/:id/dependencies`).
 * Raw relation row; the Gantt view normalises to blocker→blocked (kind='blocks' →
 * taskId blocks relatedTaskId; 'blocked_by' → the reverse).
 */
data class BoardDependency(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("related_task_id") val relatedTaskId: String = "",
    @SerializedName("kind") val kind: String = "blocks",
) {
    val blockerId: String get() = if (kind == "blocked_by") relatedTaskId else taskId
    val blockedId: String get() = if (kind == "blocked_by") taskId else relatedTaskId
}

/** A task attachment — mirrors `ListTaskAttachmentsRow`. */
data class Attachment(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("uploader_id") val uploaderId: String? = null,
    @SerializedName("filename") val filename: String = "",
    @SerializedName("content_type") val contentType: String = "",
    @SerializedName("size") val size: Long = 0,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("uploader_name") val uploaderName: String? = null,
)

/** A task journal entry — mirrors the backend `taskEventResp`. `data` is opaque. */
data class TaskEvent(
    @SerializedName("id") val id: String = "",
    @SerializedName("kind") val kind: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("actor_name") val actorName: String? = null,
)

/** `{ "url": "/api/uploads/<uuid>.<ext>" }` from `POST /api/uploads`. */
data class UploadResponse(@SerializedName("url") val url: String = "")

// ── Request bodies ──────────────────────────────────────────────────────────

data class CreateCommentRequest(
    @SerializedName("body") val body: String,
    @SerializedName("mentions") val mentions: List<String> = emptyList(),
)

data class UpdateCommentRequest(@SerializedName("body") val body: String)

data class AddRelationRequest(
    @SerializedName("number") val number: Long,
    @SerializedName("kind") val kind: String = "relates",
)
