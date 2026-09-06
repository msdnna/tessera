package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import com.google.gson.JsonObject
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Before
import org.junit.Test
import website.msdnna.tessera.data.model.GitlabSyncAction
import website.msdnna.tessera.data.model.GitlabSyncActionsPage
import website.msdnna.tessera.data.model.GitlabSyncRun
import website.msdnna.tessera.data.repository.GitlabRepository

/**
 * The journal screen's paging and lazy diffs (#2918). Since #2616 the actions
 * endpoint answers in keyset pages of 500 and no longer ships the before/after
 * blob in the list, so a long run has to be streamed through the cursor and a
 * row's diff fetched when it is opened. The wire shapes themselves are pinned by
 * `GitlabJournalWireTest`; here the repo is stubbed to test the flow.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class GitlabJournalPagingTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<GitlabRepository>()
    private val run = GitlabSyncRun(id = "run-1", kind = "pull")

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun action(seq: Int, hasDetail: Boolean = false) =
        GitlabSyncAction(id = "act-$seq", seq = seq, op = "update", summary = "action $seq", hasDetail = hasDetail)

    private fun diff(after: String): JsonObject =
        Gson().fromJson("""{"fields": {"title": {"before": "a", "after": "$after"}}}""", JsonObject::class.java)

    @Test
    fun `a run longer than one page streams in through the cursor`() = runTest(dispatcher) {
        coEvery { repo.syncActions("ws-1", "run-1", null) } returns
            GitlabSyncActionsPage(items = listOf(action(0), action(1)), hasMore = true, nextAfterSeq = 1)
        coEvery { repo.syncActions("ws-1", "run-1", 1) } returns
            GitlabSyncActionsPage(items = listOf(action(2)), hasMore = false, nextAfterSeq = null)
        val vm = GitlabJournalViewModel(repo)

        vm.toggleRun("ws-1", run)
        advanceUntilIdle()

        val first = vm.state.value.actionsByRun.getValue("run-1")
        assertThat(first.items.map { it.seq }).containsExactly(0, 1).inOrder()
        assertThat(first.hasMore).isTrue()

        vm.loadMoreActions("ws-1", run)
        advanceUntilIdle()

        val all = vm.state.value.actionsByRun.getValue("run-1")
        // Appended, not replaced — the second page must not drop the first.
        assertThat(all.items.map { it.seq }).containsExactly(0, 1, 2).inOrder()
        assertThat(all.hasMore).isFalse()
        assertThat(all.nextSeq).isNull()
        coVerify(exactly = 1) { repo.syncActions("ws-1", "run-1", 1) }
    }

    @Test
    fun `the last page offers nothing more to load`() = runTest(dispatcher) {
        coEvery { repo.syncActions("ws-1", "run-1", null) } returns
            GitlabSyncActionsPage(items = listOf(action(0)), hasMore = false, nextAfterSeq = null)
        val vm = GitlabJournalViewModel(repo)

        vm.toggleRun("ws-1", run)
        advanceUntilIdle()
        vm.loadMoreActions("ws-1", run)
        advanceUntilIdle()

        coVerify(exactly = 1) { repo.syncActions(any(), any(), any()) }
    }

    @Test
    fun `opening a row fetches its diff and caches it`() = runTest(dispatcher) {
        coEvery { repo.syncActions("ws-1", "run-1", null) } returns
            GitlabSyncActionsPage(items = listOf(action(0, hasDetail = true)), hasMore = false, nextAfterSeq = null)
        coEvery { repo.syncActionDetail("ws-1", "run-1", "act-0") } returns diff("b")
        val vm = GitlabJournalViewModel(repo)

        vm.toggleRun("ws-1", run)
        advanceUntilIdle()
        val row = vm.state.value.actionsByRun.getValue("run-1").items.single()
        assertThat(row.detail).isNull()

        vm.select("ws-1", run, row)
        advanceUntilIdle()

        val shown = vm.state.value.selected!!.second
        assertThat(vm.state.value.loadingDetail).isFalse()
        assertThat(shown.detail!!.getAsJsonObject("fields").getAsJsonObject("title").get("after").asString)
            .isEqualTo("b")

        // Cached back onto the list row, so reopening it costs no second call.
        assertThat(vm.state.value.actionsByRun.getValue("run-1").items.single().detail).isNotNull()
        vm.closeDetail()
        vm.select("ws-1", run, vm.state.value.actionsByRun.getValue("run-1").items.single())
        advanceUntilIdle()
        coVerify(exactly = 1) { repo.syncActionDetail(any(), any(), any()) }
    }

    @Test
    fun `a row with no diff does not hit the detail endpoint`() = runTest(dispatcher) {
        coEvery { repo.syncActions("ws-1", "run-1", null) } returns
            GitlabSyncActionsPage(items = listOf(action(0)), hasMore = false, nextAfterSeq = null)
        val vm = GitlabJournalViewModel(repo)

        vm.toggleRun("ws-1", run)
        advanceUntilIdle()
        vm.select("ws-1", run, vm.state.value.actionsByRun.getValue("run-1").items.single())
        advanceUntilIdle()

        assertThat(vm.state.value.selected).isNotNull()
        assertThat(vm.state.value.loadingDetail).isFalse()
        coVerify(exactly = 0) { repo.syncActionDetail(any(), any(), any()) }
    }

    /** A late diff must not overwrite the dialog if the user already moved on. */
    @Test
    fun `a diff arriving after the user switched rows does not repaint the dialog`() = runTest(dispatcher) {
        coEvery { repo.syncActions("ws-1", "run-1", null) } returns
            GitlabSyncActionsPage(
                items = listOf(action(0, hasDetail = true), action(1, hasDetail = true)),
                hasMore = false, nextAfterSeq = null,
            )
        coEvery { repo.syncActionDetail("ws-1", "run-1", "act-0") } returns diff("first")
        coEvery { repo.syncActionDetail("ws-1", "run-1", "act-1") } returns diff("second")
        val vm = GitlabJournalViewModel(repo)

        vm.toggleRun("ws-1", run)
        advanceUntilIdle()
        val rows = vm.state.value.actionsByRun.getValue("run-1").items

        // Both selects are issued before either detail call is allowed to resume.
        vm.select("ws-1", run, rows[0])
        vm.select("ws-1", run, rows[1])
        advanceUntilIdle()

        val shown = vm.state.value.selected!!.second
        assertThat(shown.id).isEqualTo("act-1")
        assertThat(shown.detail!!.getAsJsonObject("fields").getAsJsonObject("title").get("after").asString)
            .isEqualTo("second")
        // The first row's diff still lands in the cache — only the dialog is guarded.
        assertThat(vm.state.value.actionsByRun.getValue("run-1").items[0].detail).isNotNull()
    }
}
