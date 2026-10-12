package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.BoardFavorite

/** Избранные доски (#3115): «Выше» / «Ниже» в меню строки и соседи для move. */
class FavoritesTest {
    private val list = listOf("a", "b", "c").map { BoardFavorite(boardId = it, name = it, projectId = "p") }

    @Test
    fun `выше — на место соседа сверху, соседи нового места`() {
        val m = Favorites.step(list, "c", -1)!!
        assertThat(m.list.map { it.boardId }).containsExactly("a", "c", "b").inOrder()
        assertThat(m.beforeId).isEqualTo("a")
        assertThat(m.afterId).isEqualTo("b")
    }

    @Test
    fun `ниже до конца списка — соседа снизу нет`() {
        val m = Favorites.step(list, "b", 1)!!
        assertThat(m.list.map { it.boardId }).containsExactly("a", "c", "b").inOrder()
        assertThat(m.beforeId).isEqualTo("c")
        assertThat(m.afterId).isNull()
    }

    @Test
    fun `на краю и для чужой доски — некуда`() {
        assertThat(Favorites.step(list, "a", -1)).isNull()
        assertThat(Favorites.step(list, "c", 1)).isNull()
        assertThat(Favorites.step(list, "zzz", 1)).isNull()
    }

    @Test
    fun `строка открывает свою доску`() {
        val b = BoardFavorite(boardId = "b1", name = "Спринт", projectId = "p1", color = "#2f9e6e").toBoard()
        assertThat(b.id).isEqualTo("b1")
        assertThat(b.projectId).isEqualTo("p1")
        assertThat(b.color).isEqualTo("#2f9e6e")
    }
}
