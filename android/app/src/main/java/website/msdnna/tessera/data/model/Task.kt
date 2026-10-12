package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A recurrence rule on a task: repeat every [interval] [freq]-units. The backend
 * also stores a day/month anchor for monthly/yearly rules, but the client only
 * ever sends {freq, interval} and lets the server manage the anchor.
 */
data class Recurrence(
    @SerializedName("freq") val freq: String = "",
    @SerializedName("interval") val interval: Int = 1,
    // Weekly: weekdays to repeat on (0=Sun … 6=Sat). Custom: explicit dates
    // ("yyyy-MM-dd"). Both omitted (null) when not applicable.
    @SerializedName("weekdays") val weekdays: List<Int>? = null,
    @SerializedName("dates") val dates: List<String>? = null,
    // Monthly/yearly day-of-month anchor (server-managed; `month` only for yearly).
    // A short month temporarily moves the due date, and without the anchor every
    // later occurrence inherits the clamp — a task repeating on the 31st becomes a
    // 28th task the first time it passes through February. The editor's controls
    // never hold it, so a rule rebuilt from them borrows it back (withStoredAnchor).
    @SerializedName("day") val day: Int = 0,
    @SerializedName("month") val month: Int = 0,
    // The IANA zone the rule's calendar arithmetic runs in on the server ("every
    // Monday", "the 31st" are calendar statements, and a due date picked here is
    // local midnight — which east of UTC belongs to the previous UTC day). Kept on
    // the model even though nothing local reads it: the picker rebuilds the whole
    // rule out of its controls (`buildRule`), so a field the client forgets is a
    // field the server loses the moment the recurrence is touched.
    @SerializedName("tz") val tz: String? = null,
    @SerializedName("trigger") val trigger: String? = null, // complete|column|schedule
    @SerializedName("trigger_column") val triggerColumn: String? = null,
    @SerializedName("target_column") val targetColumn: String? = null,
    @SerializedName("create_new") val createNew: Boolean = false,
    @SerializedName("once") val once: Boolean = false,
    @SerializedName("skip_weekends") val skipWeekends: Boolean = false,
)

/**
 * A board card (top-level task) or subtask, with tag/assignee ids aggregated —
 * mirrors the backend `ListBoardTasksWithMeta` / `ListSubtasksWithMeta` rows.
 */
data class Task(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("column_id") val columnId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("start_date") val startDate: String? = null,
    // Canonical estimate; its unit is resolved from the project/workspace config
    // (time → minutes, points → point number, custom → count).
    @SerializedName("estimate") val estimate: Double? = null,
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("completed_at") val completedAt: String? = null,
    @SerializedName("archived_at") val archivedAt: String? = null,
    @SerializedName("number") val number: Long? = null,
    @SerializedName("recurrence") val recurrence: Recurrence? = null,
    // Per-task due-notification overrides (null = inherit the user default).
    @SerializedName("due_lead_minutes") val dueLeadMinutes: Int? = null,
    @SerializedName("due_repeat_minutes") val dueRepeatMinutes: Int? = null,
    @SerializedName("due_notify_enabled") val dueNotifyEnabled: Boolean? = null,
    // Eisenhower-matrix override: pinned quadrant 0-3, or null = derive from
    // priority + due-date (see the matrix view). Encoding matches the backend.
    @SerializedName("eisenhower_quadrant") val eisenhowerQuadrant: Int? = null,
    // Assigned milestone («Этап»), resolved to a name from the board's milestone
    // list (null = unassigned). Mirrors the web `task.milestone_id`.
    @SerializedName("milestone_id") val milestoneId: String? = null,
    @SerializedName("tag_ids") val tagIds: List<String> = emptyList(),
    @SerializedName("assignee_ids") val assigneeIds: List<String> = emptyList(),
    // GitLab provenance (present when the card is mirrored from a GitLab issue).
    @SerializedName("gitlab_iid") val gitlabIid: Long? = null,
    @SerializedName("gitlab_url") val gitlabUrl: String? = null,
    @SerializedName("gitlab_author") val gitlabAuthor: String? = null,
    @SerializedName("gitlab_author_name") val gitlabAuthorName: String? = null,
    @SerializedName("gitlab_author_avatar_url") val gitlabAuthorAvatarUrl: String? = null,
    // External GitLab assignees (no Tessera account) — display names only.
    @SerializedName("gitlab_assignees") val gitlabAssignees: List<String> = emptyList(),
    // Logins of the GitLab-member assignees (parallel to the names) so the on-card
    // picker can tell which GL members are currently assigned.
    @SerializedName("gitlab_assignee_logins") val gitlabAssigneeLogins: List<String> = emptyList(),
    // GitLab hierarchy state, filled ONLY by `ListSubtasksWithMeta` (the subtasks of an
    // open task). Deliberately separate names from the `gitlab_*` fields above: the
    // board query aliases the same columns as `gitlab_iid`/`gitlab_url`, the subtask
    // query returns them raw — one Kotlin class, two wire shapes, so both sets exist.
    // A board card therefore has gitlabIid but no glIid, and that is not a bug.
    @SerializedName("gl_iid") val glIid: Long? = null,
    @SerializedName("gl_web_url") val glWebUrl: String? = null,
    // Empty/absent = GitLab did not put the issue under a parent work item.
    @SerializedName("gl_parent_global_id") val glParentGlobalId: String? = null,
) {
    val isCompleted: Boolean get() = completedAt != null
}

data class CreateTaskRequest(
    @SerializedName("column_id") val columnId: String,
    @SerializedName("title") val title: String,
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("description") val description: String = "",
    @SerializedName("priority") val priority: Int = 0,
    @SerializedName("due_date") val dueDate: String? = null,
    @SerializedName("start_date") val startDate: String? = null,
)

data class MoveTaskRequest(
    @SerializedName("column_id") val columnId: String,
    @SerializedName("before_id") val beforeId: String? = null,
    @SerializedName("after_id") val afterId: String? = null,
)

/** Per-task due-notification overrides; null fields inherit the user default. */
data class DueNotifyRequest(
    @SerializedName("lead_minutes") val leadMinutes: Int? = null,
    @SerializedName("repeat_minutes") val repeatMinutes: Int? = null,
    @SerializedName("enabled") val enabled: Boolean? = null,
)

/** Pin a task to an Eisenhower quadrant (0-3), or null to clear the override. */
data class SetEisenhowerRequest(@SerializedName("quadrant") val quadrant: Int?)

data class SetParentRequest(@SerializedName("parent_id") val parentId: String?)
data class AddTagRequest(@SerializedName("tag_id") val tagId: String)
data class AddAssigneeRequest(@SerializedName("user_id") val userId: String)

/** A GitLab project member, assignable on integration boards even without a
 *  Tessera account (synced from GitLab Project Members). */
data class GitlabMember(
    @SerializedName("gl_user_id") val glUserId: Long = 0,
    @SerializedName("gl_username") val glUsername: String = "",
    @SerializedName("gl_name") val glName: String = "",
    @SerializedName("gl_avatar_url") val glAvatarUrl: String = "",
    // The Tessera account this GitLab member is linked to, if any. The server
    // reports the two links *separately* (ListGitlabProjectMembersByWorkspace):
    // [tesseraUserId] comes from an OAuth identity, [tesseraUserIdPat] from a
    // connected personal access token. A PAT-linked person leaves the first one
    // null, so reading only [tesseraUserId] loses them — see `buildMentionItems`.
    @SerializedName("tessera_user_id") val tesseraUserId: String? = null,
    @SerializedName("tessera_user_id_pat") val tesseraUserIdPat: String? = null,
)

data class PinGitlabAssigneeRequest(
    @SerializedName("gl_username") val glUsername: String,
    @SerializedName("gl_name") val glName: String,
    @SerializedName("gl_avatar_url") val glAvatarUrl: String,
)

/** Move a task (and its subtasks) to another board; column optional (defaults to first). */
data class TransferTaskRequest(
    @SerializedName("board_id") val boardId: String,
    @SerializedName("column_id") val columnId: String? = null,
)

/** Invite an existing user to a workspace by email. role = member|admin. */
data class AddMemberRequest(
    @SerializedName("email") val email: String,
    @SerializedName("role") val role: String = "member",
)

/** A workspace member — mirrors the backend `ListMembersRow`. */
data class Member(
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("role") val role: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
    // The member's GitLab login (empty when their account isn't linked), so an
    // @-mention can insert `@login` — a display name with spaces resolves to
    // nobody once the comment is pushed to GitLab.
    @SerializedName("gl_username") val glUsername: String = "",
    // Activity (#3073): «был в приложении» and «последнее действие здесь».
    @SerializedName("last_seen_at") val lastSeenAt: String? = null,
    @SerializedName("last_active_at") val lastActiveAt: String? = null,
)

/** A pending request to be let into a workspace (#3077), as its managers see it. */
data class AccessRequest(
    @SerializedName("id") val id: String = "",
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("user_name") val userName: String = "",
    @SerializedName("user_email") val userEmail: String = "",
    @SerializedName("board_name") val boardName: String = "",
    @SerializedName("message") val message: String = "",
    @SerializedName("created_at") val createdAt: String = "",
)

data class TransferOwnerRequest(@SerializedName("user_id") val userId: String)

data class CreateTagRequest(
    @SerializedName("name") val name: String,
    @SerializedName("color") val color: String,
)

/** A project's friendly display name for a tag prefix (mirrors backend `db.TagPrefix`).
 *  `prefix` is the canonical key (trimmed + lowercased) the backend stores. */
data class TagPrefix(
    @SerializedName("project_id") val projectId: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("prefix") val prefix: String = "",
    @SerializedName("label") val label: String = "",
)

/** PUT body: the complete desired set of prefix display names (replace-all). */
data class SetTagPrefixesRequest(
    @SerializedName("prefixes") val prefixes: List<TagPrefixEntry>,
)

data class TagPrefixEntry(
    @SerializedName("prefix") val prefix: String,
    @SerializedName("label") val label: String,
)

data class UpdateColumnRequest(
    @SerializedName("name") val name: String,
    @SerializedName("color") val color: String = "",
)

data class CreateColumnRequest(@SerializedName("name") val name: String)

data class SetDoneColumnRequest(@SerializedName("column_id") val columnId: String?)

data class ColumnMoveRequest(
    @SerializedName("before_id") val beforeId: String?,
    @SerializedName("after_id") val afterId: String?,
)
