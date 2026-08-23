package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Comment

class CommentThreadsTest {
    private fun cm(id: String, parent: String? = null) = Comment(id = id, parentId = parent, body = id)

    @Test
    fun `roots without replies stay in order`() {
        val threads = groupThreads(listOf(cm("a"), cm("b")))
        assertThat(threads.map { it.root.id }).containsExactly("a", "b").inOrder()
        assertThat(threads.flatMap { it.replies }).isEmpty()
    }

    @Test
    fun `replies land under their root`() {
        // The API order: root, its replies, next root.
        val threads = groupThreads(listOf(cm("a"), cm("a1", "a"), cm("a2", "a"), cm("b"), cm("b1", "b")))
        assertThat(threads.map { it.root.id }).containsExactly("a", "b").inOrder()
        assertThat(threads[0].replies.map { it.id }).containsExactly("a1", "a2").inOrder()
        assertThat(threads[1].replies.map { it.id }).containsExactly("b1")
    }

    @Test
    fun `reply to a reply hangs off the same root`() {
        // The backend collapses reply-to-reply, so parent_id already points at the
        // root — but a1's own id must never open a thread of its own.
        val threads = groupThreads(listOf(cm("a"), cm("a1", "a"), cm("a2", "a")))
        assertThat(threads).hasSize(1)
        assertThat(threads[0].replies.map { it.id }).containsExactly("a1", "a2").inOrder()
    }

    @Test
    fun `orphan reply is shown as a root instead of dropped`() {
        val threads = groupThreads(listOf(cm("x", "gone"), cm("a")))
        assertThat(threads.map { it.root.id }).containsExactly("x", "a").inOrder()
        assertThat(threads[0].replies).isEmpty()
    }

    @Test
    fun `reply before its root stays visible`() {
        // Out-of-order list: the reply is promoted to a root, and the real root
        // still opens its own thread.
        val threads = groupThreads(listOf(cm("a1", "a"), cm("a"), cm("a2", "a")))
        assertThat(threads.map { it.root.id }).containsExactly("a1", "a").inOrder()
        assertThat(threads[0].replies).isEmpty()
        assertThat(threads[1].replies.map { it.id }).containsExactly("a2")
    }

    @Test
    fun `an orphan carries its own replies`() {
        // "x" answers a comment outside the list, so it heads a thread itself —
        // and "y", which answers x, still belongs under it.
        val threads = groupThreads(listOf(cm("x", "gone"), cm("y", "x")))
        assertThat(threads.map { it.root.id }).containsExactly("x")
        assertThat(threads[0].replies.map { it.id }).containsExactly("y")
    }

    @Test
    fun `a reply promoted for being early does not swallow the rest`() {
        // a1 is promoted (its root came later), so it opens no thread others can
        // join: a2, which answers it, is kept visible as a root of its own rather
        // than nesting two levels deep.
        val threads = groupThreads(listOf(cm("a1", "a"), cm("a"), cm("a2", "a1")))
        assertThat(threads.map { it.root.id }).containsExactly("a1", "a", "a2").inOrder()
        assertThat(threads.flatMap { it.replies }).isEmpty()
    }

    @Test
    fun `empty list`() {
        assertThat(groupThreads(emptyList())).isEmpty()
    }
}
