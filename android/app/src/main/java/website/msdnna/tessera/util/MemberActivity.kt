package website.msdnna.tessera.util

import java.time.OffsetDateTime

/**
 * Активность участника (#3073, зеркало веба `utils/memberActivity.js`): позднейшая
 * из «был в приложении» (`last_seen_at`) и «последнее действие здесь»
 * (`last_active_at`), разложенная по корзинам строки окна «Участники».
 */
enum class ActivityBucket { NEVER, NOW, MIN, HOURS, YESTERDAY, DAYS, LONG_AGO }

data class MemberActivity(val bucket: ActivityBucket, val n: Long, val online: Boolean)

/** Окно «в сети», как на вебе: четверть часа. */
const val ONLINE_MS: Long = 15 * 60 * 1000

private fun epochMs(iso: String?): Long =
    if (iso.isNullOrBlank()) 0L else runCatching { OffsetDateTime.parse(iso).toInstant().toEpochMilli() }.getOrDefault(0L)

fun memberActivity(lastSeenAt: String?, lastActiveAt: String?, nowMs: Long = System.currentTimeMillis()): MemberActivity {
    val at = maxOf(epochMs(lastSeenAt), epochMs(lastActiveAt))
    if (at == 0L) return MemberActivity(ActivityBucket.NEVER, 0, false)
    val diff = maxOf(0L, nowMs - at)
    val online = diff < ONLINE_MS
    val min = diff / 60_000
    val h = min / 60
    val d = h / 24
    return when {
        min < 2 -> MemberActivity(ActivityBucket.NOW, 0, online)
        min < 60 -> MemberActivity(ActivityBucket.MIN, min, online)
        h < 24 -> MemberActivity(ActivityBucket.HOURS, h, online)
        d < 2 -> MemberActivity(ActivityBucket.YESTERDAY, 1, online)
        d < 7 -> MemberActivity(ActivityBucket.DAYS, d, online)
        else -> MemberActivity(ActivityBucket.LONG_AGO, d, online)
    }
}
