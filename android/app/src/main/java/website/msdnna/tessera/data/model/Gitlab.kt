package website.msdnna.tessera.data.model

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

/** The workspace's GitLab integration config (mirrors backend `gitlabIntegrationView`). */
data class GitlabIntegration(
    @SerializedName("configured") val configured: Boolean = false,
    @SerializedName("project_path") val projectPath: String = "",
    @SerializedName("board_id") val boardId: String? = null,
    @SerializedName("enabled") val enabled: Boolean = true,
    @SerializedName("sync_interval_sec") val syncIntervalSec: Int = 0,
    @SerializedName("due_source") val dueSource: String = "issue_milestone",
    @SerializedName("last_synced_at") val lastSyncedAt: String? = null,
    @SerializedName("label_rules") val labelRules: GitlabRules = GitlabRules(),
)

data class GitlabSetIntegrationRequest(
    @SerializedName("project_path") val projectPath: String,
    @SerializedName("board_id") val boardId: String,
    @SerializedName("enabled") val enabled: Boolean,
    @SerializedName("sync_interval_sec") val syncIntervalSec: Int,
    @SerializedName("due_source") val dueSource: String,
    @SerializedName("label_rules") val labelRules: GitlabRules,
)

data class GitlabSyncResult(
    @SerializedName("total") val total: Int = 0,
    @SerializedName("created") val created: Int = 0,
    @SerializedName("updated") val updated: Int = 0,
)
