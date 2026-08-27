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

/** One write-back binding trigger — the Tessera-side event. Mirrors backend
 *  `gitlab.BindTrigger`. Nullable qualifiers (priority/completed) = "any";
 *  Gson omits them when null (matches the backend `omitempty`). */
data class GitlabBindTrigger(
    @SerializedName("type") val type: String = "column",
    @SerializedName("column_id") val columnId: String? = null,
    @SerializedName("column_name") val columnName: String? = null,
    @SerializedName("priority") val priority: Int? = null,
    @SerializedName("completed") val completed: Boolean? = null,
    @SerializedName("date_kind") val dateKind: String? = null,
)

/** One write-back binding action — the GitLab-side effect. Mirrors backend
 *  `gitlab.BindAction`. `clear_prefix` is always on the wire (no omitempty). */
data class GitlabBindAction(
    @SerializedName("type") val type: String = "set_label",
    @SerializedName("label") val label: String? = null,
    @SerializedName("clear_prefix") val clearPrefix: Boolean = false,
    @SerializedName("state") val state: String? = null,
    @SerializedName("date_kind") val dateKind: String? = null,
    @SerializedName("add_marker") val addMarker: Boolean = false,
)

/** One customizable write-back binding (Tessera trigger → GitLab action). Mirrors
 *  backend `gitlab.Binding`. */
data class GitlabBinding(
    @SerializedName("enabled") val enabled: Boolean = true,
    @SerializedName("trigger") val trigger: GitlabBindTrigger = GitlabBindTrigger(),
    @SerializedName("action") val action: GitlabBindAction = GitlabBindAction(),
)

/** Opt-in write-back (Tessera → GitLab) config; mirrors backend `gitlab.Writeback`.
 *  All off by default. The legacy toggles are kept so a pre-bindings integration can
 *  be synthesized into an editable binding set on load; a non-empty [bindings] takes
 *  over completely (the backend ignores the legacy flags then). `push_create` /
 *  `fetch_templates` are not editable here (the toggles live on web), but they are
 *  round-tripped so saving never clobbers a web-configured create-issue setup — and
 *  read by the task modal's «Создать issue» row (see `gitlabCreateCaps`). */
data class GitlabWriteback(
    @SerializedName("enabled") val enabled: Boolean = false,
    @SerializedName("push_state") val pushState: Boolean = false,
    @SerializedName("push_priority") val pushPriority: Boolean = false,
    @SerializedName("push_comments") val pushComments: Boolean = false,
    @SerializedName("push_labels") val pushLabels: Boolean = false,
    @SerializedName("push_due") val pushDue: Boolean = false,
    @SerializedName("push_assignees") val pushAssignees: Boolean = false,
    @SerializedName("push_estimate") val pushEstimate: Boolean = false,
    @SerializedName("push_create") val pushCreate: Boolean = false,
    @SerializedName("fetch_templates") val fetchTemplates: Boolean = false,
    @SerializedName("bindings") val bindings: List<GitlabBinding>? = null,
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
    // Forced periodic FULL sweep (catches deletes/drift an incremental pull can't
    // see); 0 = only on the first sync or a manual "Полная синхронизация".
    @SerializedName("full_sync_interval_sec") val fullSyncIntervalSec: Int = 86400,
    // relations_sync: off|pull — import GitLab issue links into the relations tab
    // ("two_way" is reserved on the backend for pushing Tessera relations back).
    @SerializedName("relations_sync") val relationsSync: String = "pull",
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
    @SerializedName("full_sync_interval_sec") val fullSyncIntervalSec: Int,
    @SerializedName("relations_sync") val relationsSync: String,
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

/** One issue template of the bound repo (a Markdown file under
 *  `.gitlab/issue_templates`) — GET …/gitlab/issue-templates. */
data class GitlabIssueTemplate(
    @SerializedName("name") val name: String = "",
    @SerializedName("content") val content: String = "",
)

/** Body of POST /tasks/:id/gitlab-issue. The description is optional: left null the
 *  backend uses the task's own (already saved) text, which is what we want — sending
 *  it twice would drift the issue body from the task on the next write-back. */
data class CreateGitlabIssueRequest(
    @SerializedName("description") val description: String? = null,
)

/** What the backend mirrored into the GitLab upload store while creating the issue
 *  (#2713): a skipped file is a link the issue won't render, so we name it. */
data class GitlabAssetStats(
    @SerializedName("uploaded") val uploaded: Int = 0,
    @SerializedName("skipped") val skipped: Int = 0,
)

/** POST /tasks/:id/gitlab-issue response — the fresh link plus the asset stats. */
data class GitlabIssueCreated(
    @SerializedName("iid") val iid: Long = 0,
    @SerializedName("web_url") val webUrl: String = "",
    @SerializedName("attachments") val attachments: GitlabAssetStats? = null,
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
