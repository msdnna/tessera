package website.msdnna.tessera.util

import com.google.gson.JsonParser

/**
 * Reactions in a call (#3054, web `utils/confReactions.js`).
 *
 * A reaction is a moment, not a record: it travels over the SFU room's data
 * channel (topic [CONF_REACTION_TOPIC], lossy) to whoever is in the call right
 * now and is never stored. Whatever arrives off the wire is checked against the
 * palette, so a peer cannot put arbitrary text over everyone's stage. The wire
 * form is the web's: `{"t":"reaction","e":"👍"}`.
 */
val CONF_REACTIONS = listOf("👍", "👏", "😂", "🎉", "❤️", "🤔")

const val CONF_REACTION_TOPIC = "reaction"

/** How long a bubble floats over the stage. */
const val CONF_REACTION_TTL_MS = 4_000L

/** At most this many bubbles at once — applause, not a curtain over the speaker. */
const val CONF_REACTION_MAX = 12

/** Our own sending is throttled: at most [CONF_REACTION_BURST] per window. */
const val CONF_REACTION_BURST = 4
const val CONF_REACTION_WINDOW_MS = 2_000L

/** One bubble. [lane] spreads a burst across the stage's bottom edge. */
data class ConfReaction(
    val id: Long,
    val identity: String,
    val name: String,
    val emoji: String,
    val at: Long,
    val lane: Int,
)

fun encodeReaction(emoji: String): ByteArray? =
    if (emoji in CONF_REACTIONS) """{"t":"reaction","e":"$emoji"}""".toByteArray(Charsets.UTF_8) else null

/** The emoji a payload carries, or null for anything that is not one of ours. */
fun decodeReaction(payload: ByteArray?): String? {
    if (payload == null) return null
    return runCatching {
        val obj = JsonParser.parseString(payload.toString(Charsets.UTF_8)).asJsonObject
        val e = obj.get("e")?.asString
        e.takeIf { obj.get("t")?.asString == "reaction" && it in CONF_REACTIONS }
    }.getOrNull()
}

/** Whether we may send another one at [now], given the times of our recent ones. */
fun canReact(sentAt: List<Long>, now: Long): Boolean =
    sentAt.count { now - it < CONF_REACTION_WINDOW_MS } < CONF_REACTION_BURST

/** The feed with one more bubble, newest last, capped at [CONF_REACTION_MAX]. */
fun addReaction(feed: List<ConfReaction>, r: ConfReaction): List<ConfReaction> = (feed + r).takeLast(CONF_REACTION_MAX)

/** The feed without the bubbles that have had their time. */
fun pruneReactions(feed: List<ConfReaction>, now: Long): List<ConfReaction> =
    feed.filter { now - it.at < CONF_REACTION_TTL_MS }
