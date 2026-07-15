package website.msdnna.tessera.data.model

import com.google.gson.JsonObject
import com.google.gson.annotations.SerializedName

/** The current user's GitLab connection (the token itself is never returned). */
data class GitlabConnection(
    @SerializedName("connected") val connected: Boolean = false,
    @SerializedName("base_url") val baseUrl: String = "",
    @SerializedName("gl_username") val glUsername: String = "",
)

data class GitlabConnectRequest(
    @SerializedName("base_url") val baseUrl: String,
    @SerializedName("token") val token: String,
)

/** One label rule: match a label, apply an action. Mirrors the backend
 *  `gitlab.Rule`. value_map keys vary by action (status→column name, priority→
 *  level "0".."4", board→board id). */
data class GitlabRule(
    @SerializedName("match") val match: String = "",
    @SerializedName("match_type") val matchType: String = "prefix",
    @SerializedName("action") val action: String = "tag",
    @SerializedName("value_map") val valueMap: Map<String, String>? = null,
    @SerializedName("keep_prefix") val keepPrefix: Boolean = false,
)

/** The generic label rule engine config (mirrors backend `gitlab.Rules`). */
data class GitlabRules(
    @SerializedName("rules") private val rulesRaw: List<GitlabRule>? = null,
    @SerializedName("default_column") val defaultColumn: String = "",
    @SerializedName("default_action") val defaultAction: String = "tag",
    @SerializedName("tag_keep_prefix") val tagKeepPrefix: Boolean = true,
) {
    val rules: List<GitlabRule> get() = rulesRaw.orEmpty()
}

/** Opt-in write-back (Tessera → GitLab) config; mirrors backend `gitlab.Writeback`.
 *  All off by default. We round-trip the 7 toggles the editor exposes (the backend
 *  also carries push_title_desc / column_label_bindings, deferred features). */
data class GitlabWriteback(
    @SerializedName("enabled") val enabled: Boolean = false,
    @SerializedName("push_state") val pushState: Boolean = false,
    @SerializedName("push_priority") val pushPriority: Boolean = false,
    @SerializedName("push_comments") val pushComments: Boolean = false,
    @SerializedName("push_labels") val pushLabels: Boolean = false,
    @SerializedName("push_due") val pushDue: Boolean = false,
    @SerializedName("push_assignees") val pushAssignees: Boolean = false,
    @SerializedName("push_estimate") val pushEstimate: Boolean = false,
)

/** One GitLab binding (GL-project → board). Multiple per workspace since be 0.71
 *  (mirrors backend `gitlabIntegrationView`). */
data class GitlabIntegration(
    @SerializedName("configured") val configured: Boolean = false,
    @SerializedName("id") val id: String? = null,
    @SerializedName("name") val name: String = "",
    @SerializedName("project_path") val projectPath: String = "",
    @SerializedName("board_id") val boardId: String? = null,
    // The integration board's project — used to resolve project-scoped tag-prefixes.
    @SerializedName("project_id") val projectId: String? = null,
    @SerializedName("enabled") val enabled: Boolean = true,
    @SerializedName("sync_interval_sec") val syncIntervalSec: Int = 0,
    @SerializedName("due_source") val dueSource: String = "issue_milestone",
    @SerializedName("start_source") val startSource: String = "created",
    // scope: assigned|all — how much of the GL project to import.
    @SerializedName("scope") val scope: String = "assigned",
    // closed_policy: all|archive_closed_sprints|period.
    @SerializedName("closed_policy") val closedPolicy: String = "all",
    @SerializedName("closed_after") val closedAfter: String? = null,
    @SerializedName("last_synced_at") val lastSyncedAt: String? = null,
    @SerializedName("label_rules") val labelRules: GitlabRules = GitlabRules(),
    @SerializedName("writeback") val writeback: GitlabWriteback = GitlabWriteback(),
    // Resolved estimation unit of the integration board; the estimate toggle is
    // only meaningful when it's "time".
    @SerializedName("estimation_unit") val estimationUnit: String = "time",
)

/** GET /workspaces/:id/gitlab/integrations — all bindings + capability flags. */
data class GitlabIntegrationsResponse(
    @SerializedName("integrations") private val integrationsRaw: List<GitlabIntegration>? = null,
    @SerializedName("default_rules") val defaultRules: GitlabRules = GitlabRules(),
    // An instance-wide service token is set → bindings can be configured/synced
    // without a personal PAT.
    @SerializedName("service_configured") val serviceConfigured: Boolean = false,
    // Only admins may create/update/delete bindings.
    @SerializedName("is_admin") val isAdmin: Boolean = false,
) {
    val integrations: List<GitlabIntegration> get() = integrationsRaw.orEmpty()
}

/** Create/update body for a binding (POST/PUT …/gitlab/integrations[/:id]). */
data class GitlabIntegrationRequest(
    @SerializedName("name") val name: String,
    @SerializedName("project_path") val projectPath: String,
    @SerializedName("board_id") val boardId: String,
    @SerializedName("enabled") val enabled: Boolean,
    @SerializedName("sync_interval_sec") val syncIntervalSec: Int,
    @SerializedName("due_source") val dueSource: String,
    @SerializedName("start_source") val startSource: String,
    @SerializedName("scope") val scope: String,
    @SerializedName("closed_policy") val closedPolicy: String,
    @SerializedName("closed_after") val closedAfter: String? = null,
    @SerializedName("label_rules") val labelRules: GitlabRules,
    @SerializedName("writeback") val writeback: GitlabWriteback,
)

data class GitlabSyncResult(
    @SerializedName("total") val total: Int = 0,
    @SerializedName("created") val created: Int = 0,
    @SerializedName("updated") val updated: Int = 0,
)

/** One sync-journal run header (mirrors backend `db.GitlabSyncRun`). `kind` =
 *  pull|push, `trigger` = manual|auto, `status` = ok|partial|error|fail. */
data class GitlabSyncRun(
    @SerializedName("id") val id: String = "",
    @SerializedName("kind") val kind: String = "pull",
    @SerializedName("trigger") val trigger: String = "auto",
    @SerializedName("status") val status: String = "ok",
    @SerializedName("created_count") val createdCount: Int = 0,
    @SerializedName("updated_count") val updatedCount: Int = 0,
    @SerializedName("deleted_count") val deletedCount: Int = 0,
    @SerializedName("action_count") val actionCount: Int = 0,
    @SerializedName("error") val error: String = "",
    @SerializedName("started_at") val startedAt: String? = null,
    @SerializedName("finished_at") val finishedAt: String? = null,
)

/**
 * A parked write-back conflict (mirrors backend `conflictDTO`): GitLab and Tessera
 * both changed the same field(s) since the last sync. `change_kind` ∈
 * due|estimate|title_desc|state|priority. Discrete kinds (state/priority) can't be
 * manually merged. Resolved via ours/theirs/manual.
 */
data class GitlabConflict(
    @SerializedName("id") val id: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("task_title") val taskTitle: String = "",
    @SerializedName("task_number") val taskNumber: Long? = null,
    @SerializedName("change_kind") val changeKind: String = "",
    @SerializedName("gl_iid") val glIid: Long = 0,
    @SerializedName("fields") private val fieldsRaw: List<ConflictField>? = null,
    @SerializedName("detected_at") val detectedAt: String? = null,
) {
    val fields: List<ConflictField> get() = fieldsRaw.orEmpty()

    /** Manual merge is only meaningful for free-text/numeric fields. */
    val manualAllowed: Boolean get() = fields.all { it.field != "state" && it.field != "priority" }
}

/** One diverged field of a conflict: the last-synced [base], Tessera's [ours], GitLab's [theirs]. */
data class ConflictField(
    @SerializedName("field") val field: String = "",
    @SerializedName("base") val base: String = "",
    @SerializedName("ours") val ours: String = "",
    @SerializedName("theirs") val theirs: String = "",
) {
    val isText: Boolean get() = this.field == "title" || this.field == "description"
}

/** Resolve a conflict: [resolution] ∈ ours|theirs|manual; [value] carries the merged
 *  field values for a manual resolution (keyed by field name). */
data class ResolveConflictRequest(
    @SerializedName("resolution") val resolution: String,
    @SerializedName("value") val value: Map<String, String>? = null,
)

/** One action within a run (mirrors backend `syncActionDTO`). `detail` is the raw
 *  before/after (pull) or payload/result (push) blob, rendered ad-hoc by the UI. */
data class GitlabSyncAction(
    @SerializedName("id") val id: String = "",
    @SerializedName("seq") val seq: Int = 0,
    @SerializedName("direction") val direction: String = "pull",
    @SerializedName("entity_type") val entityType: String = "",
    @SerializedName("op") val op: String = "",
    @SerializedName("gl_iid") val glIid: Long? = null,
    @SerializedName("summary") val summary: String = "",
    @SerializedName("detail") val detail: JsonObject? = null,
    @SerializedName("status") val status: String = "ok",
    @SerializedName("error") val error: String = "",
)
