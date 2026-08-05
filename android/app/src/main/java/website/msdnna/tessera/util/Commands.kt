package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.CommandDef
import website.msdnna.tessera.data.model.WorkspaceCommand

/*
 * Quick-action helpers for the markdown editor's `/`-autocomplete. Port of web
 * `frontend/src/utils/commands.js`.
 *
 * The parser itself lives on the backend (internal/quickact) — these are only the
 * bits the editor needs locally: where a `/`-query starts, which registry entries
 * match it, and what text to insert. Keep [canonCommandKey] in sync with
 * commandKeyRe in backend/handlers/workspace_commands.go.
 */

private val KEY_RE = Regex("^[a-z0-9][a-z0-9_-]{0,31}$")

/**
 * A `/`-popup row: a built-in quick action the backend executes, or a custom
 * dictionary entry that only stays in the comment text as a note to the reader.
 */
data class CommandItem(
    val key: String,
    val description: String = "",
    val aliases: List<String> = emptyList(),
    val arg: String = "none",
    val example: String = "",
    val builtin: Boolean = true,
)

/**
 * Normalises a user-typed key to its storage form: no leading slash, lowercase,
 * trimmed. Returns "" when nothing usable is left.
 */
fun canonCommandKey(raw: String?): String =
    (raw ?: "").trim().trimStart('/').trim().lowercase()

fun isValidCommandKey(key: String?): Boolean = KEY_RE.matches(canonCommandKey(key))

/** An open `/`-query: [start] is the index of the slash, [query] what follows it. */
data class SlashQuery(val start: Int, val query: String)

// Unlike @-mentions the trigger is a slash at the START OF A LINE only. Firing
// after any whitespace would pop the menu on `cd /home`, `src/utils` and `24/7` —
// all common in task text. This mirrors the backend, which only treats a line
// whose first non-space character is `/` as a command.
private val SLASH_RE = Regex("(?:^|\n)/([a-z0-9_-]*)$")

/** Finds an open `/`-query in the text up to the caret, or null. */
fun detectSlashQuery(upto: String?): SlashQuery? {
    val text = upto ?: return null
    val m = SLASH_RE.find(text) ?: return null
    val query = m.groupValues[1]
    return SlashQuery(start = text.length - query.length - 1, query = query)
}

/**
 * Flattens the API's registry response into popup rows. Built-in commands come
 * first (registry order = popup order), custom ones after.
 */
fun commandItems(builtin: List<CommandDef>, custom: List<WorkspaceCommand>): List<CommandItem> =
    builtin.map {
        CommandItem(
            key = it.key,
            description = it.description,
            aliases = it.aliases.orEmpty(),
            arg = it.arg.ifBlank { "none" },
            example = it.example,
            builtin = true,
        )
    } + custom.map {
        CommandItem(key = it.key, description = it.description, example = "/${it.key}", builtin = false)
    }

/**
 * Filters rows for a query, matching key, aliases and description (so «срок»
 * finds /due). An empty query lists everything, capped at [limit].
 */
fun matchCommands(items: List<CommandItem>, query: String?, limit: Int = 8): List<CommandItem> {
    val q = (query ?: "").lowercase()
    return items.filter {
        q.isEmpty() ||
            it.key.contains(q, ignoreCase = true) ||
            it.aliases.any { a -> a.contains(q, ignoreCase = true) } ||
            it.description.contains(q, ignoreCase = true)
    }.take(limit)
}

/**
 * What picking a row types into the editor: commands that take an argument leave
 * the caret after a space, argument-less ones end the line so the next command
 * can follow.
 */
fun commandInsertText(item: CommandItem?): String {
    if (item == null) return ""
    val takesArg = item.builtin && item.arg.isNotBlank() && item.arg != "none"
    return if (takesArg) "/${item.key} " else "/${item.key}\n"
}

// The cheap gate before asking the backend for a command preview.
private val COMMAND_LINE_RE = Regex("(?:^|\n)\\s*/[a-zA-Z]")

/** Reports whether a body has any line that starts with a slash. */
fun hasCommandLine(body: String?): Boolean = COMMAND_LINE_RE.containsMatchIn(body ?: "")
