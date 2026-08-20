package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Comment

/** A comment thread: the root comment and the replies hanging off it. */
data class CommentThread(val root: Comment, val replies: List<Comment>)

/**
 * Assembles the flat comment list into "root + its replies" threads.
 *
 * The API returns a FLAT array carrying `parent_id`, already ordered as
 * root → its replies → next root. Threads are two levels deep: the backend
 * collapses a reply-to-reply onto the same root, so a reply never has replies
 * of its own.
 *
 * Two cases the server does not rule out: a reply whose parent is missing from
 * the list, and a reply that arrives before its root (an out-of-order list).
 * Both become roots of their own so the comment stays visible instead of being
 * dropped. Mirrors `groupThreads` in `frontend/src/utils/taskFeed.js`.
 */
fun groupThreads(comments: List<Comment>): List<CommentThread> {
    val known = comments.mapTo(mutableSetOf()) { it.id }
    val out = mutableListOf<Pair<Comment, MutableList<Comment>>>()
    val byRoot = mutableMapOf<String, MutableList<Comment>>()
    for (cm in comments) {
        val parentId = cm.parentId?.takeIf { known.contains(it) }
        val replies = parentId?.let { byRoot[it] }
        if (replies != null) {
            replies += cm
            continue
        }
        val fresh = mutableListOf<Comment>()
        out += cm to fresh
        // Only a genuine root opens a thread others can join: the fallback above
        // is a reply shown as a root, and its own id must not swallow siblings.
        if (parentId == null) byRoot[cm.id] = fresh
    }
    return out.map { (root, replies) -> CommentThread(root, replies) }
}

/** «3 ответа» — the reply count with the right Russian plural form. */
fun replyCountLabel(n: Int): String {
    val form = when {
        n % 10 == 1 && n % 100 != 11 -> "ответ"
        n % 10 in 2..4 && (n % 100 < 10 || n % 100 >= 20) -> "ответа"
        else -> "ответов"
    }
    return "$n $form"
}
