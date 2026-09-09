package website.msdnna.tessera.ui.screens

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.ui.screens.documents.docSwitchRows

/**
 * The two rules the #2894 rework put into the shell: what the title menu offers
 * to jump to, and when the sidebar's edge-swipe is allowed to fire.
 *
 * Both live one call away from a Composable, so they are checked here rather
 * than through a screen: the bug being guarded against is a wrong answer, not a
 * wrong pixel.
 */
class DocChromeRulesTest {
    private fun doc(id: String, parent: String? = null, title: String = id) =
        Document(id = id, parentId = parent, title = title, icon = "📄")

    @Test
    fun `the parent leads, then the nested ones`() {
        val docs = listOf(
            doc("root"),
            doc("open", parent = "root"),
            doc("kid-a", parent = "open"),
            doc("kid-b", parent = "open"),
            doc("stranger", parent = "root"),
        )
        val rows = docSwitchRows(docs, docs[1])
        assertThat(rows.map { it.id }).containsExactly("root", "kid-a", "kid-b").inOrder()
        assertThat(rows.first().parent).isTrue()
        assertThat(rows.drop(1).map { it.parent }).containsExactly(false, false)
        // A sibling is not a neighbour: it is neither the step out nor a step in.
        assertThat(rows.map { it.id }).doesNotContain("stranger")
    }

    @Test
    fun `a root document with no children offers nothing`() {
        val docs = listOf(doc("open"), doc("other"))
        assertThat(docSwitchRows(docs, docs[0])).isEmpty()
        // And with nothing open there is no menu to fill at all.
        assertThat(docSwitchRows(docs, null)).isEmpty()
    }

    @Test
    fun `a parent the list does not carry is simply absent`() {
        // The list is loaded per folder, so the document above the open one may
        // not be in it. Better no row than a row named after nothing.
        val docs = listOf(doc("open", parent = "elsewhere"), doc("kid", parent = "open"))
        assertThat(docSwitchRows(docs, docs[0]).map { it.id }).containsExactly("kid")
    }

    @Test
    fun `a document is never a way to itself`() {
        // Server data has been seen self-referencing; a menu item that goes
        // where you already are reads as broken, and its bin is worse.
        val self = doc("open", parent = "open")
        assertThat(docSwitchRows(listOf(self), self)).isEmpty()
    }

    @Test
    fun `an open document stops the sidebar swipe, an open drawer restores it`() {
        assertThat(
            drawerGesturesEnabled(timelineLike = false, documentOpen = false, callOnScreen = false, drawerOpen = false),
        ).isTrue()
        // The regression: scrolling a document pulled the sidebar in.
        assertThat(
            drawerGesturesEnabled(timelineLike = false, documentOpen = true, callOnScreen = false, drawerOpen = false),
        ).isFalse()
        assertThat(
            drawerGesturesEnabled(timelineLike = true, documentOpen = false, callOnScreen = false, drawerOpen = false),
        ).isFalse()
        // The call stage owns pinch-zoom and pan the same way (#2896).
        assertThat(
            drawerGesturesEnabled(timelineLike = false, documentOpen = false, callOnScreen = true, drawerOpen = false),
        ).isFalse()
        // Open, the drawer must stay draggable — that is how it closes.
        assertThat(
            drawerGesturesEnabled(timelineLike = false, documentOpen = true, callOnScreen = false, drawerOpen = true),
        ).isTrue()
        assertThat(
            drawerGesturesEnabled(timelineLike = true, documentOpen = true, callOnScreen = true, drawerOpen = true),
        ).isTrue()
    }
}
