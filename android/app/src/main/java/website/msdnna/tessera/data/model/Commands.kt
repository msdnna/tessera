package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/*
 * Quick actions ("/close", "/assign @msdnna") typed into a comment. The backend
 * owns the parser and the built-in registry — the client only renders the popup,
 * shows a dry-run preview and reports what ran. Mirrors backend
 * `handlers/workspace_commands.go` + `handlers/quick_actions.go`.
 */

/** One built-in quick action, as served from `quickact.Registry`. */
data class CommandDef(
    @SerializedName("key") val key: String = "",
    @SerializedName("aliases") val aliases: List<String>? = null,
    /** How the argument is resolved: none|user|date|text|tag|column|milestone|priority|estimate|task_ref. */
    @SerializedName("arg") val arg: String = "none",
    @SerializedName("arg_optional") val argOptional: Boolean = false,
    @SerializedName("repeatable") val repeatable: Boolean = false,
    @SerializedName("description") val description: String = "",
    @SerializedName("example") val example: String = "",
)

/** One custom dictionary entry: suggested in the popup, never executed. */
data class WorkspaceCommand(
    @SerializedName("key") val key: String = "",
    @SerializedName("description") val description: String = "",
)

/**
 * `GET /workspaces/:id/commands`. [canManage] rides along because the client has
 * nowhere else to learn its own workspace role — it gates the dictionary editor.
 */
data class CommandRegistry(
    @SerializedName("builtin") val builtin: List<CommandDef>? = null,
    @SerializedName("custom") val custom: List<WorkspaceCommand>? = null,
    @SerializedName("can_manage") val canManage: Boolean = false,
)

data class SetWorkspaceCommandsRequest(
    @SerializedName("commands") val commands: List<WorkspaceCommand>,
)

/** What one command did — or why it didn't. [summary] is the backend's own text. */
data class CommandOutcome(
    @SerializedName("key") val key: String = "",
    @SerializedName("arg") val arg: String = "",
    @SerializedName("summary") val summary: String = "",
    @SerializedName("error") val error: String = "",
)

/** `POST /tasks/:id/commands/preview` — a dry run of the draft. */
data class CommandPreview(
    @SerializedName("commands") val commands: List<CommandOutcome>? = null,
    /** Custom keys seen in the draft — they stay in the text. */
    @SerializedName("custom") val custom: List<String>? = null,
    /** True when the draft is nothing but commands (no comment would be stored). */
    @SerializedName("rest_empty") val restEmpty: Boolean = false,
)

data class PreviewCommandsRequest(@SerializedName("body") val body: String)

/** The per-request rollup of what the comment's commands actually did. */
data class CommandSummary(
    @SerializedName("applied") val applied: List<CommandOutcome>? = null,
    @SerializedName("errors") val errors: List<CommandOutcome>? = null,
    @SerializedName("custom") val custom: List<String>? = null,
) {
    val isEmpty: Boolean get() = applied.isNullOrEmpty() && errors.isNullOrEmpty()
}

/**
 * The reply to creating a comment. The comment's own fields stay inlined at the
 * top level (clients predating quick actions read them there); a command-only
 * comment stores no row at all, so [id] is null and only the summary comes back.
 */
data class CommentResult(
    @SerializedName("id") val id: String? = null,
    @SerializedName("command_summary") val commandSummary: CommandSummary? = null,
)
