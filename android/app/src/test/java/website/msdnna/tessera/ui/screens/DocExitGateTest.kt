package website.msdnna.tessera.ui.screens

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.ui.screens.documents.DocAction
import website.msdnna.tessera.ui.screens.documents.DocExitGate
import website.msdnna.tessera.ui.screens.documents.docActionNeedsFlush
import website.msdnna.tessera.util.DocSaveStatus

/**
 * The rule that keeps the last sentence typed on a phone (#2932).
 *
 * Leaving the editor destroys the WebView, and with it the save that was only
 * just asked for. Everything here is about that ordering: what may leave at
 * once, what has to wait, and what must never be waited on forever.
 */
class DocExitGateTest {
    private class Door {
        var left = 0
        var flushes = 0
        val exit: () -> Unit = { left++ }
        val flush: () -> Unit = { flushes++ }
    }

    @Test
    fun `nothing to write leaves straight away`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.SAVED, door.flush, door.exit)
        assertThat(door.left).isEqualTo(1)
        // Nothing queued, so the page is not disturbed on the way out.
        assertThat(door.flushes).isEqualTo(0)
        assertThat(gate.waiting).isFalse()
    }

    @Test
    fun `queued edits hold the door and are flushed`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.DIRTY, door.flush, door.exit)
        assertThat(door.flushes).isEqualTo(1)
        assertThat(door.left).isEqualTo(0)
        assertThat(gate.waiting).isTrue()
        // The write starting is not the write landing.
        assertThat(gate.onStatus(DocSaveStatus.SAVING)).isFalse()
        assertThat(door.left).isEqualTo(0)
        // It landed.
        assertThat(gate.onStatus(DocSaveStatus.SAVED)).isTrue()
        assertThat(door.left).isEqualTo(1)
        assertThat(gate.waiting).isFalse()
    }

    @Test
    fun `a write already in flight is waited out without asking twice`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.SAVING, door.flush, door.exit)
        assertThat(door.left).isEqualTo(0)
        gate.onStatus(DocSaveStatus.SAVED)
        assertThat(door.left).isEqualTo(1)
    }

    @Test
    fun `a conflict is not something waiting can fix`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.CONFLICT, door.flush, door.exit)
        assertThat(door.left).isEqualTo(1)
        assertThat(gate.waiting).isFalse()

        // And one raised while waiting releases the door too.
        val second = Door()
        gate.leave(DocSaveStatus.DIRTY, second.flush, second.exit)
        assertThat(gate.onStatus(DocSaveStatus.CONFLICT)).isTrue()
        assertThat(second.left).isEqualTo(1)
    }

    @Test
    fun `a failed write releases rather than traps`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.DIRTY, door.flush, door.exit)
        assertThat(gate.onStatus(DocSaveStatus.ERROR)).isTrue()
        assertThat(door.left).isEqualTo(1)
    }

    @Test
    fun `a page that stopped answering lets go after the wait`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.DIRTY, door.flush, door.exit)
        assertThat(gate.expire()).isTrue()
        assertThat(door.left).isEqualTo(1)
        assertThat(gate.waiting).isFalse()
        // Nothing is waiting any more, so a later tick does nothing at all.
        assertThat(gate.expire()).isFalse()
        assertThat(gate.onStatus(DocSaveStatus.SAVED)).isFalse()
        assertThat(door.left).isEqualTo(1)
    }

    @Test
    fun `the exit runs once, however many times back is pressed`() {
        val gate = DocExitGate()
        val door = Door()
        gate.leave(DocSaveStatus.DIRTY, door.flush, door.exit)
        gate.leave(DocSaveStatus.DIRTY, door.flush, door.exit)
        assertThat(door.flushes).isEqualTo(2)
        gate.onStatus(DocSaveStatus.SAVED)
        gate.onStatus(DocSaveStatus.SAVED)
        assertThat(door.left).isEqualTo(1)
    }

    @Test
    fun `every way out of the editor waits for the write`() {
        // The four that end with the editor gone or the document re-read from
        // the server. Two of them used to tear the WebView down mid-PATCH, and
        // two never even asked the page to write.
        assertThat(DocAction.entries.filter(::docActionNeedsFlush))
            .containsExactly(DocAction.READ, DocAction.LIST, DocAction.CHILDREN, DocAction.HISTORY)
    }

    @Test
    fun `what stays over a live editor does not wait`() {
        // Waiting here would be a pause with nothing behind it: these open a
        // sheet or a dialog above the page, which keeps typing and keeps saving.
        listOf(DocAction.COMMENTS, DocAction.LINKS, DocAction.TOC, DocAction.RENAME, DocAction.NESTED)
            .forEach { assertThat(docActionNeedsFlush(it)).isFalse() }
    }
}
