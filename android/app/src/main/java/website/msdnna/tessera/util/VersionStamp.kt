package website.msdnna.tessera.util

/**
 * Build info of one component — the app itself or the API it talks to (#2859,
 * web `VersionBadge.vue` + `useVersionInfo.js`).
 *
 * [commit] and [builtAt] are optional on purpose: a dev backend answers `/version`
 * without them, and a build made outside a git checkout has no commit to report.
 * Absent means "omit the line", never "show an empty one".
 */
data class VersionStamp(
    val version: String,
    val commit: String = "",
    val builtAt: String = "",
)

/**
 * The lines describing one [stamp], head first — `Клиент 0.75.0`, then
 * `коммит abc1234`, then `сборка 02.09.2026`. Empty when there is no version at
 * all (the API hasn't answered, or answered offline).
 *
 * Pure, and the labels arrive as already-resolved strings rather than resource
 * ids: this is what makes it unit-testable on the JVM, and it keeps the "what to
 * show" decision out of the composable. [formatDate] gets the raw ISO timestamp
 * and may return "" for one it cannot parse — then the line is dropped rather
 * than printed raw, since a half-parsed date reads as a bug to the user.
 */
fun versionLines(
    label: String,
    stamp: VersionStamp?,
    commitLabel: (String) -> String,
    builtLabel: (String) -> String,
    formatDate: (String) -> String,
): List<String> {
    if (stamp == null || stamp.version.isBlank()) return emptyList()
    val out = mutableListOf("$label ${stamp.version}")
    if (stamp.commit.isNotBlank()) out += commitLabel(stamp.commit)
    if (stamp.builtAt.isNotBlank()) formatDate(stamp.builtAt).takeIf { it.isNotBlank() }?.let { out += builtLabel(it) }
    return out
}
