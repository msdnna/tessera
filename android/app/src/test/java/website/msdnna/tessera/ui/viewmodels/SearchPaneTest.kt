package website.msdnna.tessera.ui.viewmodels

import com.google.gson.Gson
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.SearchResults
import website.msdnna.tessera.ui.UiText

/**
 * The search overlay's two silent failures (#2932).
 *
 * The first is the one this file exists for: the view model has always recorded
 * why a search failed, and nothing ever rendered it. A search that could not
 * reach the server produced a state with `error` set and `results` null, which
 * matched no branch of the overlay's `when` and fell through to the results
 * list — drawn with nothing in it. A 500, an expired session and a train tunnel
 * all looked like a blank panel: no message, no spinner, no "nothing found".
 *
 * The second is the page cap. The server answers with at most 25 hits of each
 * kind and now says so; a client that ignores the flag presents an arbitrary
 * slice as the whole answer.
 */
class SearchPaneTest {

    private fun state(
        query: String = "отчёт",
        loading: Boolean = false,
        error: UiText? = null,
        results: SearchResults? = null,
    ) = SearchUiState(query = query, loading = loading, error = error, results = results)

    @Test
    fun `a failed search has a face of its own`() {
        val failed = state(error = UiText.Raw("Нет соединения с сервером"))
        assertEquals(SearchPane.FAILED, searchPane(failed, hasHelpHits = false))
    }

    @Test
    fun `an empty answer is still an empty answer`() {
        // The negative control: a fix that routed every empty panel through
        // FAILED would put an error under a search that simply matched nothing.
        val nothing = state(results = Gson().fromJson("""{"tasks":[],"notes":[]}""", SearchResults::class.java))
        assertEquals(SearchPane.EMPTY, searchPane(nothing, hasHelpHits = false))
    }

    @Test
    fun `the other faces are unchanged`() {
        assertEquals(SearchPane.HINT, searchPane(state(query = "  "), hasHelpHits = false))
        assertEquals(SearchPane.LOADING, searchPane(state(loading = true), hasHelpHits = false))
        // Help hits are resolved locally and are already on screen, so they win
        // over both a pending answer and a failed one — the list carries the
        // failure as a row instead of replacing everything with it.
        assertEquals(SearchPane.RESULTS, searchPane(state(loading = true), hasHelpHits = true))
        assertEquals(
            SearchPane.RESULTS,
            searchPane(state(error = UiText.Raw("Что-то пошло не так")), hasHelpHits = true),
        )
    }

    @Test
    fun `the truncation flags are read off the wire`() {
        val json = """{"tasks":[],"notes":[],"has_more":{"tasks":true,"notes":false,"documents":true}}"""
        val res = Gson().fromJson(json, SearchResults::class.java)
        assertTrue(res.hasMore.tasks)
        assertFalse(res.hasMore.notes)
        assertTrue(res.hasMore.documents)
    }

    @Test
    fun `a server that does not send the flags is read as "nothing was cut off"`() {
        // Not a hypothetical: the phone updates on its own schedule and talks to
        // whatever the server happens to be running. A missing map must not put
        // "showing the first 25" under every list.
        val res = Gson().fromJson("""{"tasks":[],"notes":[]}""", SearchResults::class.java)
        assertFalse(res.hasMore.tasks)
        assertFalse(res.hasMore.notes)
    }
}
