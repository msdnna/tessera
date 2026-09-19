package website.msdnna.tessera.ui.screens.documents

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import website.msdnna.tessera.util.DocSaveStatus

/**
 * Holds a way out of the document editor until autosave has settled (#2932).
 *
 * The editor is a WebView, and the WebView is destroyed the moment `editing`
 * turns false — `AndroidView`'s `onRelease` calls `destroy()`, which takes the
 * page, its scripts and any request in flight with it. Asking the page to flush
 * on the way out is therefore not enough: `DocEditorController.save()` only
 * *posts* `window.tesseraEmbed.save()` at the page, and the PATCH it starts is
 * still in the air a frame later, when the view is torn down. The last words
 * typed were lost exactly as if nothing had been asked at all — and two of the
 * four doors out (the list, and switching to another document) never asked.
 *
 * So the exit waits for the page to report back. Every door hands its action to
 * [leave]; it runs at once when there is nothing to write, and otherwise after
 * the status says the write is done.
 *
 * The wait is bounded by whoever drives it (see [expire]): a page that stopped
 * answering — a crashed WebView, a bridge that never loaded — must not trap the
 * user inside a document.
 */
class DocExitGate {
    private var pending by mutableStateOf<(() -> Unit)?>(null)

    /** Whether an exit is waiting for the page. Compose-observable. */
    val waiting: Boolean
        get() = pending != null

    /**
     * Leaves the editor, writing first if anything is queued.
     *
     * @param status what the page last reported
     * @param flush asks the page to write now (the bridge's `save`)
     * @param exit what leaving actually does — dropping the editor, switching
     *   documents, opening the journal
     */
    fun leave(status: DocSaveStatus, flush: () -> Unit, exit: () -> Unit) {
        if (settled(status)) {
            pending = null
            exit()
            return
        }
        pending = exit
        flush()
    }

    /**
     * A fresh status from the page. Runs the waiting exit once the write is
     * done; returns whether it did.
     */
    fun onStatus(status: DocSaveStatus): Boolean {
        if (!settled(status)) return false
        return release()
    }

    /**
     * The wait ran out. Leaves anyway: the text is no more saved for being kept
     * on screen, and a door that never opens is worse than a lost paragraph.
     */
    fun expire(): Boolean = release()

    private fun release(): Boolean {
        val exit = pending ?: return false
        pending = null
        exit()
        return true
    }

    /**
     * Whether waiting any longer could still put the text on the server.
     *
     * Only the two working states are worth waiting out. A conflict has stopped
     * autosave for good (the server holds someone else's version), and an error
     * leaves the content queued for a retry that this page will not live to
     * make — neither resolves by standing still, and both are already spelled
     * out in the bar above the text.
     */
    private fun settled(status: DocSaveStatus): Boolean = when (status) {
        DocSaveStatus.DIRTY, DocSaveStatus.SAVING -> false
        DocSaveStatus.SAVED, DocSaveStatus.CONFLICT, DocSaveStatus.ERROR -> true
    }
}

/**
 * How long an exit waits for the page before leaving regardless. Long enough
 * for a debounce (800 ms) plus a slow PATCH over a phone's connection, short
 * enough that a dead bridge does not read as a frozen app.
 */
const val DOC_EXIT_WAIT_MS = 5000L

/**
 * Whether a menu action has to wait for the write before it runs (#2932).
 *
 * The list is decided here rather than branch by branch in the screen, because
 * the answer is the same question every time — *does this leave the text behind
 * me?* — and the screen's `when` is where it got answered wrong twice already.
 * A new action added to [DocAction] lands on the safe side only if its author
 * thinks about this function, and the test below is what makes them.
 *
 * Two shapes qualify. One is the door: [DocAction.READ], [DocAction.LIST] and
 * [DocAction.CHILDREN] all end with the WebView released — and released takes
 * `destroy()` with it, along with any PATCH still in the air. The other is
 * reading the document from the server while the newest paragraph is still on
 * the phone: [DocAction.HISTORY] compares against the journal, and a journal
 * fetched a keystroke early is a rollback target missing the last sentence.
 *
 * Everything else either stays over a live editor (comments, links, the
 * outline) or only opens a dialog (rename, remove, a nested document) — nothing
 * is read back and nothing is torn down, so there is nothing to wait for.
 */
fun docActionNeedsFlush(action: DocAction): Boolean = when (action) {
    DocAction.READ, DocAction.LIST, DocAction.CHILDREN, DocAction.HISTORY -> true

    DocAction.EDIT, DocAction.COMMENTS, DocAction.LINKS, DocAction.TOC,
    DocAction.NESTED, DocAction.RENAME, DocAction.REMOVE,
    -> false
}
