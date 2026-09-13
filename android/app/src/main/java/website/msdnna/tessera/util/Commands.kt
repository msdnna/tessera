package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
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

/**
 * Every key the backend treats as built-in: the registry keys AND their aliases.
 * `quickact.byKey` maps both, and `IsBuiltin` — what the dictionary PUT checks —
 * looks the key up in that same map. Collecting only `key` lets a custom "/done"
 * (alias of /close) or "/status" (alias of /move) past the editor's inline
 * validation and into a 400 that rejects the WHOLE dictionary, with an error
 * that cannot name the offending row. Mirrors web `builtinCommandKeys`.
 */
fun builtinCommandKeys(items: List<CommandDef>): Set<String> {
    val out = LinkedHashSet<String>()
    for (cmd in items) {
        for (raw in listOf(cmd.key) + cmd.aliases.orEmpty()) {
            val key = canonCommandKey(raw)
            if (key.isNotEmpty()) out.add(key)
        }
    }
    return out
}

/** An open `/`-query: [start] is the index of the slash, [query] what follows it. */
data class SlashQuery(val start: Int, val query: String)

// Unlike @-mentions the trigger is a slash at the START OF A LINE only. Firing
// after any whitespace would pop the menu on `cd /home`, `src/utils` and `24/7` —
// all common in task text. This mirrors the backend, which only treats a line
// whose first non-space character is `/` as a command.
//
// `\z`, not `$`: java's `$` also matches *before* a trailing newline, so a query
// the user has already finished with Enter would stay open — and `start`, measured
// back from the end of the text, would land on the newline instead of the slash.
// JS has no such rule, which is why the web regex can end in `$`.
private val SLASH_RE = Regex("(?:^|\n)/([a-z0-9_-]*)\\z")

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
 * Описание встроенной команды → строковый ресурс (#2932).
 *
 * Реестр быстрых действий живёт на бэкенде (`internal/quickact/registry.go`), и
 * `Description` там захардкожен по-русски: API отдаёт его клиентам как есть, так что
 * на английском интерфейсе попап читался целиком по-русски. Сам парсер при этом
 * двуязычен (`args.go` понимает и «высокий», и `high`) — отстала ровно витрина.
 *
 * Мапа явная, а не через `getIdentifier`: имя, собранное в рантайме, R8 не видит и
 * вычищает строку из релизной сборки. Полноту сторожит `i18n/CommandLabelsTest`.
 */
val BUILTIN_COMMAND_DESCRIPTIONS: Map<String, Int> = mapOf(
    "assign" to R.string.cmd_assign_desc,
    "unassign" to R.string.cmd_unassign_desc,
    "due" to R.string.cmd_due_desc,
    "remove_due" to R.string.cmd_remove_due_desc,
    "start" to R.string.cmd_start_desc,
    "remove_start" to R.string.cmd_remove_start_desc,
    "estimate" to R.string.cmd_estimate_desc,
    "remove_estimate" to R.string.cmd_remove_estimate_desc,
    "priority" to R.string.cmd_priority_desc,
    "title" to R.string.cmd_title_desc,
    "tag" to R.string.cmd_tag_desc,
    "untag" to R.string.cmd_untag_desc,
    "milestone" to R.string.cmd_milestone_desc,
    "remove_milestone" to R.string.cmd_remove_milestone_desc,
    "move" to R.string.cmd_move_desc,
    "close" to R.string.cmd_close_desc,
    "reopen" to R.string.cmd_reopen_desc,
    "archive" to R.string.cmd_archive_desc,
    "relate" to R.string.cmd_relate_desc,
    "blocks" to R.string.cmd_blocks_desc,
    "blocked_by" to R.string.cmd_blocked_by_desc,
    "duplicates" to R.string.cmd_duplicates_desc,
    "unlink" to R.string.cmd_unlink_desc,
    "subtask" to R.string.cmd_subtask_desc,
    "parent" to R.string.cmd_parent_desc,
    "unparent" to R.string.cmd_unparent_desc,
)

/**
 * Переводит описания встроенных команд на язык интерфейса, оставляя серверный текст
 * там, где своего перевода нет: новая команда реестра всё равно что-то покажет, а
 * словарная команда воркспейса — текст её автора, его переводить нечем и незачем.
 *
 * Делать это надо ДО [matchCommands], иначе поиск по попапу остаётся русским на
 * английском интерфейсе: фильтр смотрит в то же поле `description`.
 */
fun localizedCommands(items: List<CommandItem>, resources: Resources): List<CommandItem> =
    items.map { item ->
        if (!item.builtin) {
            item
        } else {
            item.copy(description = builtinCommandDescription(resources, item.key, item.description))
        }
    }

/**
 * Описание встроенной команды на языке интерфейса, с серверным текстом как
 * фолбэком. Отдельная функция нужна справочному списку встроенных команд в
 * редакторе словаря: там строки — это [CommandDef] прямо из реестра, а не уже
 * собранные [CommandItem] попапа, и без этого список оставался русским.
 */
fun builtinCommandDescription(resources: Resources, key: String, fallback: String): String =
    BUILTIN_COMMAND_DESCRIPTIONS[key]?.let(resources::getString) ?: fallback

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
