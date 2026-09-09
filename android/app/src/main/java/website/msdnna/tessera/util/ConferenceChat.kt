package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.ConferenceMessage

/*
 * The rules the in-call chat runs on (#2896 §7, web `ConferenceChat.vue`).
 *
 * The transport is the thing to keep in mind while reading these. Messages
 * travel over HTTP; the room socket only says «the chat changed» (a payload-free
 * `chatNudge`), because the room's broadcast contract evicts a participant whose
 * buffer overflows — chat bodies on that socket would let a lively conversation
 * disconnect the people having it. So every rule below is about reconciling a
 * page the server just handed us with what is already on this phone.
 */

/** How many messages one page carries. Matches the web rail. */
const val CONF_CHAT_PAGE = 50

/** The server's `maxChatFiles`. */
const val CONF_CHAT_MAX_FILES = 5

/** The server's per-file cap, checked here so a 30 MB video fails instantly. */
const val CONF_CHAT_MAX_BYTES = 25L * 1024 * 1024

/** The server's `maxChatBody`, in *runes* — see [confChatBodyLength]. */
const val CONF_CHAT_MAX_BODY = 4000

/** A file picked but not yet sent. [uri] is opaque here on purpose: these rules
 *  are about counting and sizing, and nothing in them needs Android. */
data class ConfPendingFile(
    val uri: String,
    val name: String,
    val size: Long,
    val mime: String? = null,
)

/** Why a picked file did not make it onto the message. */
enum class ConfPickRefusal { NONE, TOO_BIG, TOO_MANY }

/**
 * The composer's attachments after a pick, and the one refusal worth telling
 * the user about.
 *
 * One refusal rather than a list: picking a folder of holiday photos would
 * otherwise answer with a wall of near-identical lines, and the first one
 * already says what happened. [refusedName] carries the file it is about, since
 * «файл больше 25 МБ» without a name is unanswerable when five were picked.
 */
data class ConfPickResult(
    val pending: List<ConfPendingFile>,
    val refusal: ConfPickRefusal = ConfPickRefusal.NONE,
    val refusedName: String = "",
)

/**
 * Adds picked files to the composer, dropping what the server would refuse.
 *
 * Checked here as well as on the server because the two failures cost wildly
 * different amounts: the client's is instant, the server's arrives after
 * uploading 25 MB over a phone connection — on the same link that is carrying
 * the call.
 */
fun confChatAccept(pending: List<ConfPendingFile>, picked: List<ConfPendingFile>): ConfPickResult {
    val out = pending.toMutableList()
    var refusal = ConfPickRefusal.NONE
    var name = ""
    for (file in picked) {
        if (out.size >= CONF_CHAT_MAX_FILES) {
            // Stop rather than carry on: every remaining file fails the same way.
            if (refusal == ConfPickRefusal.NONE) {
                refusal = ConfPickRefusal.TOO_MANY
                name = file.name
            }
            break
        }
        // An oversized file is skipped, not fatal — the rest of the pick is
        // still perfectly sendable, and the refusal below says what was lost.
        if (file.size <= CONF_CHAT_MAX_BYTES) {
            out += file
        } else if (refusal == ConfPickRefusal.NONE) {
            refusal = ConfPickRefusal.TOO_BIG
            name = file.name
        }
    }
    return ConfPickResult(out, refusal, name)
}

/**
 * The message body's length as the server counts it.
 *
 * Code points, not `String.length`. Kotlin measures UTF-16 units, Go measures
 * runes, and every emoji is two of the former and one of the latter — counting
 * the Kotlin way makes the client stricter than the server and refuses to send
 * a message the server would have accepted. Trimmed because the server trims
 * before it counts.
 */
fun confChatBodyLength(body: String): Int = body.trim().let { it.codePointCount(0, it.length) }

/** Whether the composer's send button does anything. */
fun confChatCanSend(draft: String, pending: List<ConfPendingFile>, sending: Boolean): Boolean {
    if (sending) return false
    if (confChatBodyLength(draft) > CONF_CHAT_MAX_BODY) return false
    return draft.isNotBlank() || pending.isNotEmpty()
}

/**
 * Whether this phone may delete that line.
 *
 * Mirrors the server's check rather than replacing it: a moderator may remove
 * anyone's, everyone may remove their own. A message whose author is gone
 * (`user_id` null — the row outlives the account) belongs to nobody, so only
 * moderation reaches it.
 */
fun confChatCanRemove(message: ConferenceMessage, meId: String, canModerate: Boolean): Boolean {
    if (canModerate) return true
    return meId.isNotBlank() && !message.userId.isNullOrBlank() && message.userId == meId
}

/**
 * The result of folding a newest page into the list on screen.
 *
 * [stitched] says whether the pages above the tail survived. It is what the
 * «показать более ранние» flag has to be recomputed from: a list that is exactly
 * the fetched page knows only what that page said about older text, while a
 * stitched one still has whatever the reader had paged in above it.
 */
data class ConfChatMerge(val messages: List<ConferenceMessage>, val stitched: Boolean)

/**
 * Folds a freshly fetched newest page into what is already on screen.
 *
 * The web replaces the whole list here, and that costs a reader their place:
 * load three pages of history, have somebody say «ага» while you are reading,
 * and the nudge throws all of it away. Merging keeps the pages above the tail.
 *
 * Inside the tail's own window the page is authoritative — anything on screen
 * that the server did not send back has been deleted, and a union would leave
 * removed lines on the phone of everyone who had the rail open. Outside it (the
 * older pages) nothing is dropped, because the page says nothing about them.
 *
 * Stitching requires an overlap, and that is the whole reason this returns a
 * flag. Come back from a tunnel to fifty-one new messages and the page no longer
 * touches what we hold: keeping both halves would paint a conversation with a
 * silent hole in the middle, where the missing lines are the ones nobody can ask
 * for. A shorter list that is true beats a longer one that is not.
 *
 * An empty page means an empty chat: the server always answers with the newest
 * messages that exist, so «none» is a fact about the whole conversation.
 */
fun confChatMergeTail(current: List<ConferenceMessage>, tail: List<ConferenceMessage>): ConfChatMerge {
    if (tail.isEmpty()) return ConfChatMerge(emptyList(), stitched = false)
    if (current.isEmpty()) return ConfChatMerge(tail, stitched = false)
    val ids = tail.mapTo(HashSet()) { it.id }
    if (current.none { it.id in ids }) return ConfChatMerge(tail, stitched = false)
    val edge = tail.first().createdAt
    // Strictly older than the page's first message, and not a duplicate of one
    // inside it: an optimistic append of our own message shares its timestamp
    // with the copy the page carries.
    val kept = current.filter { it.createdAt < edge && it.id !in ids }
    return ConfChatMerge(kept + tail, stitched = true)
}

/**
 * Prepends a page of older messages.
 *
 * Deduplicated by id even though the cursor excludes the newer side: the tail is
 * refetched on every nudge, and a message that arrived between two «показать
 * более ранние» taps can legitimately be in both answers.
 */
fun confChatPrepend(older: List<ConferenceMessage>, current: List<ConferenceMessage>): List<ConferenceMessage> {
    if (older.isEmpty()) return current
    val ids = current.mapTo(HashSet()) { it.id }
    return older.filter { it.id !in ids } + current
}

/**
 * How many lines have arrived that this phone has not shown.
 *
 * Counted off ids rather than off nudges (which is what the web does): the room
 * nudges for our own message too, so a nudge counter puts a badge on the chat
 * for something we just said ourselves. Anchoring on the last message the reader
 * saw also survives a reconnect, where nudges are simply lost.
 *
 * An unknown anchor means everything is unread — but the screen sets the anchor
 * on the first load precisely so that joining a call with a hundred messages of
 * history does not open with a badge reading «100» about a conversation that
 * happened before we arrived.
 */
fun confChatUnread(messages: List<ConferenceMessage>, lastSeenId: String, meId: String): Int {
    if (messages.isEmpty()) return 0
    val seen = messages.indexOfLast { it.id == lastSeenId }
    val tail = if (seen >= 0) messages.drop(seen + 1) else messages
    return tail.count { it.userId.isNullOrBlank() || it.userId != meId }
}

/** A file size split into a number and a unit, so the unit can be localised. */
enum class ConfSizeUnit { BYTES, KB, MB }

data class ConfFileSize(val amount: String, val unit: ConfSizeUnit)

/** Formats a byte count the way the task attachments do. */
fun confFileSize(bytes: Long): ConfFileSize = when {
    bytes >= 1L shl 20 -> ConfFileSize(
        String.format(java.util.Locale.US, "%.1f", bytes / (1L shl 20).toDouble()),
        ConfSizeUnit.MB,
    )

    bytes >= 1L shl 10 -> ConfFileSize(
        String.format(java.util.Locale.US, "%.0f", bytes / (1L shl 10).toDouble()),
        ConfSizeUnit.KB,
    )

    else -> ConfFileSize(bytes.coerceAtLeast(0).toString(), ConfSizeUnit.BYTES)
}
