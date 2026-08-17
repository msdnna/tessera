package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Document

class DocumentTreeTest {
    private fun doc(id: String, parent: String? = null, position: Double = 0.0, created: String = "") =
        Document(id = id, title = id, parentId = parent, position = position, createdAt = created)

    @Test
    fun `empty list stays empty`() {
        assertThat(documentTree(emptyList())).isEmpty()
    }

    @Test
    fun `children follow their parent and carry depth`() {
        val rows = documentTree(
            listOf(
                doc("child", parent = "root", position = 1.0),
                doc("root", position = 1.0),
                doc("grandchild", parent = "child"),
                doc("other", position = 2.0),
            ),
        )
        assertThat(rows.map { it.doc.id }).containsExactly("root", "child", "grandchild", "other").inOrder()
        assertThat(rows.map { it.depth }).containsExactly(0, 1, 2, 0).inOrder()
    }

    @Test
    fun `siblings order by position then creation time`() {
        val rows = documentTree(
            listOf(
                doc("c", position = 2.0),
                doc("a", position = 1.0, created = "2026-08-01"),
                doc("b", position = 1.0, created = "2026-08-02"),
            ),
        )
        assertThat(rows.map { it.doc.id }).containsExactly("a", "b", "c").inOrder()
    }

    @Test
    fun `an orphan whose parent is not in the list is shown at the root`() {
        // Happens on a project-scoped listing, or when the parent is one the
        // caller cannot see. Dropping it would hide a document that exists.
        val rows = documentTree(listOf(doc("orphan", parent = "missing")))
        assertThat(rows.map { it.doc.id }).containsExactly("orphan")
        assertThat(rows.single().depth).isEqualTo(0)
    }

    @Test
    fun `a parent cycle terminates and still lists every document`() {
        val rows = documentTree(
            listOf(
                doc("a", parent = "b"),
                doc("b", parent = "a"),
                doc("free"),
            ),
        )
        assertThat(rows.map { it.doc.id }).containsExactly("free", "a", "b")
    }
}
