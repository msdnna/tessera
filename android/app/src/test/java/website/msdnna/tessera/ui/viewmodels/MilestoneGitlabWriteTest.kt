package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
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
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Milestone
import website.msdnna.tessera.data.repository.MilestoneRepository
import website.msdnna.tessera.ui.UiText

/**
 * GitLab-этапы в обе стороны (#3010): этап из GitLab правится и закрывается на
 * телефоне, только когда привязка пишет в GitLab (`gl_writable`); отказ GitLab
 * («не синхронизировано») и конфликт показываются в окне этапов, «Повторить»
 * шлёт запись снова.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class MilestoneGitlabWriteTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<MilestoneRepository>(relaxed = true)

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun ms(json: String): Milestone = Gson().fromJson(json, Milestone::class.java)

    private val readOnly = """{"id":"m1","title":"S","gl_global_id":"gid://gitlab/Milestone/4","gl_writable":false}"""
    private val writable = """{"id":"m1","title":"S","gl_global_id":"gid://gitlab/Milestone/4","gl_writable":true}"""

    @Test
    fun `состояние записи читается из списка`() {
        assertThat(ms("""{"id":"n1","title":"N"}""").canEdit).isTrue()
        assertThat(ms(readOnly).canEdit).isFalse()
        assertThat(ms(writable).canEdit).isTrue()
        val failed = ms("""{"id":"m1","gl_global_id":"g","gl_push_error":"http 500"}""")
        assertThat(failed.unsynced).isTrue()
        assertThat(ms(writable).unsynced).isFalse()
    }

    @Test
    fun `этап без записи в GitLab не закрывается`() = runTest(dispatcher) {
        coEvery { repo.list("p1") } returns listOf(ms(readOnly))
        val vm = MilestoneViewModel(repo).also { it.openManager("p1", "P", glCapable = true) }
        advanceUntilIdle()
        vm.toggleState(ms(readOnly))
        advanceUntilIdle()
        coVerify(exactly = 0) { repo.update(any(), any(), any(), any(), any(), any()) }
    }

    @Test
    fun `отказ GitLab показывается, Повторить шлёт запись снова`() = runTest(dispatcher) {
        coEvery { repo.list("p1") } returns listOf(ms(writable))
        coEvery { repo.update("m1", any(), any(), any(), any(), "closed") } returns
            ms("""{"id":"m1","state":"closed","gl_sync":{"status":"failed","error":"http 500"}}""")
        coEvery { repo.retryGitlab("m1") } returns ms("""{"id":"m1","gl_sync":{"status":"ok"}}""")
        val vm = MilestoneViewModel(repo).also { it.openManager("p1", "P", glCapable = true) }
        advanceUntilIdle()

        vm.toggleState(ms(writable))
        advanceUntilIdle()
        assertThat(vm.manager.value?.error)
            .isEqualTo(UiText.Res(R.string.milestones_gitlab_failed, listOf("http 500")))

        vm.retryGitlab("m1")
        advanceUntilIdle()
        coVerify(exactly = 1) { repo.retryGitlab("m1") }
        assertThat(vm.manager.value?.error).isNull()
    }

    @Test
    fun `конфликт отправляет в окно конфликтов веб-версии`() = runTest(dispatcher) {
        coEvery { repo.list("p1") } returns listOf(ms(writable))
        coEvery { repo.update(any(), any(), any(), any(), any(), any()) } returns
            ms("""{"id":"m1","gl_sync":{"status":"conflict"}}""")
        val vm = MilestoneViewModel(repo).also { it.openManager("p1", "P", glCapable = true) }
        advanceUntilIdle()
        vm.updateMilestone("m1", "S", "Цели", null, null, "active")
        advanceUntilIdle()
        assertThat(vm.manager.value?.error).isEqualTo(UiText.Res(R.string.milestones_gitlab_conflict))
    }
}
