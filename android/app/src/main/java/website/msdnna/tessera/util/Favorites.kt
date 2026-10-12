package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.BoardFavorite

/**
 * Favourite boards (#3115) — the pure half of the sidebar section, the same
 * rules as the web's `utils/favorites.js`.
 */
object Favorites {
    /** The list with [boardId] stepped one place up ([delta] = -1) or down (+1),
     *  and the neighbours of its new place for `PATCH /boards/:id/favorite/move`
     *  (before = the row above, after = the row below). Null when it can't move. */
    fun step(list: List<BoardFavorite>, boardId: String, delta: Int): Moved? {
        val from = list.indexOfFirst { it.boardId == boardId }
        val to = from + delta
        if (from < 0 || to < 0 || to >= list.size) return null
        val next = list.toMutableList()
        val row = next.removeAt(from)
        next.add(to, row)
        return Moved(next, next.getOrNull(to - 1)?.boardId, next.getOrNull(to + 1)?.boardId)
    }

    data class Moved(val list: List<BoardFavorite>, val beforeId: String?, val afterId: String?)
}
