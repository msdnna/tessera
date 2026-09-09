package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Conference

/**
 * Which stamp a conference row describes itself by (#2896 §2, web `timeLine`).
 *
 * A row shows exactly one time, and which one depends on the state: a finished
 * call is described by its end, a running one by its start, and everything else
 * by the time it is planned for.
 */
enum class ConfTimeKind {
    ENDED,
    STARTED,
    SCHEDULED,

    /** Nothing to show — a room created without a planned time and never used. */
    NONE,
}

/** [iso] is null exactly for [ConfTimeKind.NONE]. */
data class ConfTimeLine(val kind: ConfTimeKind, val iso: String?)

/**
 * Picks the one time line for [c].
 *
 * The status is consulted before the stamps, not after, because a reusable room
 * (#2879) keeps `started_at` from its last session while sitting back in
 * `scheduled`. Reading the stamps first — "has started_at, so it started" —
 * would label this morning's idle standup «Началась: вчера, 10:00», which is the
 * one thing the row is supposed to answer correctly.
 */
fun conferenceTimeLine(c: Conference): ConfTimeLine = when {
    c.isEnded && !c.endedAt.isNullOrBlank() -> ConfTimeLine(ConfTimeKind.ENDED, c.endedAt)
    c.isLive && !c.startedAt.isNullOrBlank() -> ConfTimeLine(ConfTimeKind.STARTED, c.startedAt)
    !c.scheduledAt.isNullOrBlank() -> ConfTimeLine(ConfTimeKind.SCHEDULED, c.scheduledAt)
    else -> ConfTimeLine(ConfTimeKind.NONE, null)
}

/**
 * The server's own recording retention, in days. Pre-filled into the schedule
 * form rather than left blank: a field that silently becomes 30 is how people
 * find out about a retention policy by losing a file.
 */
const val RECORDING_TTL_DEFAULT = 30

/** What the schedule form's «хранить записи, дней» box currently holds. */
sealed interface TtlInput {
    /** A number the backend will accept; 0 means "keep indefinitely". */
    data class Days(val value: Int) : TtlInput

    /** Not a number, or negative — the backend answers 400, so the form refuses first. */
    data object Invalid : TtlInput
}

/**
 * Reads the retention box.
 *
 * Blank is [RECORDING_TTL_DEFAULT], the value the server would have applied
 * anyway; anything unparseable is [TtlInput.Invalid] rather than silently
 * falling back, because a typo ("3O") that quietly becomes 30 days differs from
 * the 300 the user meant only when the recording is already gone.
 */
fun parseTtlDays(raw: String): TtlInput {
    val text = raw.trim()
    if (text.isEmpty()) return TtlInput.Days(RECORDING_TTL_DEFAULT)
    val days = text.toIntOrNull() ?: return TtlInput.Invalid
    return if (days < 0) TtlInput.Invalid else TtlInput.Days(days)
}

/** What the schedule form sends once it is fit to send. */
data class ConferenceDraft(
    val title: String,
    val description: String,
    /** RFC3339, or null for "start it whenever". */
    val scheduledAtIso: String?,
    val ttlDays: Int,
)

/**
 * Reads the whole schedule form; null means "not submittable yet".
 *
 * One function decides both whether the button is enabled and what it sends, so
 * the two cannot disagree — an enabled button that builds a different payload
 * than the one it was enabled for is the failure this shape rules out. It is
 * also the only way to test the form here: the dialog holds text fields, whose
 * blinking cursor never lets a Robolectric composition reach idle (see
 * `ComponentLocaleTest`), so the rendered dialog cannot be driven from a spec.
 */
fun conferenceDraft(
    title: String,
    description: String,
    scheduledAtIso: String?,
    ttlText: String,
): ConferenceDraft? {
    val name = title.trim()
    if (name.isEmpty()) return null
    val ttl = parseTtlDays(ttlText) as? TtlInput.Days ?: return null
    return ConferenceDraft(
        title = name,
        description = description.trim(),
        scheduledAtIso = scheduledAtIso?.takeIf { it.isNotBlank() },
        ttlDays = ttl.value,
    )
}
