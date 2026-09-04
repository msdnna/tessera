package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Document

class DocListTest {
    private fun doc(id: String, parent: String? = null, position: Double = 0.0, created: String = "", title: String = id) =
        Document(id = id, title = title, parentId = parent, position = position, createdAt = created)

    @Test
    fun `empty list stays empty`() {
        assertThat(docTiles(emptyList(), null)).isEmpty()
        assertThat(docChildCount(emptyList(), "any")).isEqualTo(0)
    }

    @Test
    fun `a level shows its own children and nothing below them`() {
        val docs = listOf(
            doc("child", parent = "root"),
            doc("root"),
            doc("grandchild", parent = "child"),
            doc("other"),
        )
        assertThat(docTiles(docs, null).map { it.id }).containsExactly("root", "other")
        assertThat(docTiles(docs, "root").map { it.id }).containsExactly("child")
        assertThat(docTiles(docs, "child").map { it.id }).containsExactly("grandchild")
    }

    @Test
    fun `siblings order by position then creation time`() {
        val tiles = docTiles(
            listOf(
                doc("c", position = 2.0),
                doc("a", position = 1.0, created = "2026-08-01"),
                doc("b", position = 1.0, created = "2026-08-02"),
            ),
            null,
        )
        assertThat(tiles.map { it.id }).containsExactly("a", "b", "c").inOrder()
    }

    @Test
    fun `an orphan whose parent is not in the list is shown at the root`() {
        // Happens on a project-scoped listing, or when the parent is one the
        // caller cannot see. Dropping it would hide a document that exists —
        // and unlike the tree, a grid level has no other place to show it.
        val docs = listOf(doc("orphan", parent = "missing"))
        assertThat(docTiles(docs, null).map { it.id }).containsExactly("orphan")
    }

    @Test
    fun `a parent cycle is broken instead of hiding the documents in it`() {
        val docs = listOf(doc("a", parent = "b"), doc("b", parent = "a"), doc("free"))
        assertThat(docTiles(docs, null).map { it.id }).containsExactly("free", "a", "b")
    }

    @Test
    fun `child count matches what the level shows`() {
        val docs = listOf(doc("root"), doc("x", parent = "root"), doc("y", parent = "root"), doc("z"))
        assertThat(docChildCount(docs, "root")).isEqualTo(2)
        assertThat(docChildCount(docs, "z")).isEqualTo(0)
    }

    @Test
    fun `crumbs keep steps that still exist and re-read their titles`() {
        val docs = listOf(doc("a", title = "Регламент"), doc("b", parent = "a", title = "Приложение"))
        val kept = pruneCrumbs(docs, listOf(DocCrumb("a", "старое имя"), DocCrumb("b", "Приложение")))
        assertThat(kept.map { it.id }).containsExactly("a", "b").inOrder()
        assertThat(kept.first().title).isEqualTo("Регламент")
    }

    @Test
    fun `a deleted crumb takes everything below it`() {
        // Otherwise the grid stands on a level that no longer exists and shows
        // nothing, with no way back but the root crumb.
        val docs = listOf(doc("a"))
        val kept = pruneCrumbs(docs, listOf(DocCrumb("a", "A"), DocCrumb("gone", "Ушёл"), DocCrumb("under", "Ниже")))
        assertThat(kept.map { it.id }).containsExactly("a")
    }
}
