package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.After
import org.junit.Before
import org.junit.Test
import retrofit2.HttpException
import retrofit2.Response
import website.msdnna.tessera.data.model.Invitation
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.repository.WorkspaceRepository

/**
 * Приглашение из диалога участников (#2932).
 *
 * Поле одно, а операций за ним две: зарегистрированного добавляем сразу
 * (`AddMember`), на «no user with that email» заводим приглашение по ссылке.
 * После любой из них диалог перечитывает списки — и вот это перечитывание
 * стояло голым: оба запроса вызывались прямо внутри `_state.update { }`, так что
 * мигнувшая сеть роняла не диалог, а процесс, хотя участник на сервере уже
 * добавлен, а приглашение уже создано. Ссылку при этом было не вернуть: при
 * выключенном SMTP она единственный носитель токена.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class MembersInviteTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<WorkspaceRepository>()

    private val owner = Member(userId = "u-owner", role = "owner", email = "o@e.com")
    private val roster = listOf(owner)
    private val joined = roster + Member(userId = "u-new", role = "member", email = "new@e.com")

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun http(code: Int, error: String): HttpException {
        val raw = okhttp3.Response.Builder()
            .code(code)
            .message("error")
            .protocol(okhttp3.Protocol.HTTP_1_1)
            .request(okhttp3.Request.Builder().url("http://localhost/").build())
            .build()
        val body = """{"error":"$error"}""".toResponseBody("application/json".toMediaType())
        return HttpException(Response.error<Any>(body, raw))
    }

    /** Открытый на менеджере диалог: дальше остаётся только звать invite(). */
    private fun TestScope.opened(): MembersViewModel {
        coEvery { repo.members("ws-1") } returns roster
        coEvery { repo.invitations("ws-1") } returns emptyList()
        val model = MembersViewModel(repo, readMeId = { "u-owner" })
        model.load("ws-1")
        advanceUntilIdle()
        return model
    }

    @Test
    fun `зарегистрированного добавляем сразу — списки перечитаны`() = runTest(dispatcher) {
        val model = opened()
        coEvery { repo.addMember("ws-1", "new@e.com", "member") } returns Unit
        coEvery { repo.members("ws-1") } returns joined

        model.invite("new@e.com", "member")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.members.map { it.userId }).containsExactly("u-owner", "u-new").inOrder()
        assertThat(s.busy).isFalse()
        assertThat(s.error).isNull()
        coVerify(exactly = 0) { repo.createInvitation(any(), any(), any()) }
    }

    @Test
    fun `сеть мигнула на перечитывании — ошибка в диалоге, а не падение`() = runTest(dispatcher) {
        val model = opened()
        coEvery { repo.addMember("ws-1", "new@e.com", "member") } returns Unit
        coEvery { repo.members("ws-1") } throws IOException("network")

        model.invite("new@e.com", "member")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.busy).isFalse()
        assertThat(s.error).isNotNull()
        // Ростер остаётся тем, что сервер отдал в прошлый раз: пустой список здесь
        // соврал бы, что в пространстве никого нет.
        assertThat(s.members.map { it.userId }).containsExactly("u-owner")
    }

    @Test
    fun `незнакомый адрес — заводим приглашение и показываем ссылку`() = runTest(dispatcher) {
        val model = opened()
        coEvery { repo.addMember("ws-1", "new@e.com", "member") } throws http(400, "no user with that email")
        coEvery { repo.createInvitation("ws-1", "new@e.com", "member") } returns
            Invitation(id = "i-1", email = "new@e.com", role = "member", link = "https://t/invite?token=raw")
        coEvery { repo.invitations("ws-1") } returns listOf(Invitation(id = "i-1", email = "new@e.com"))

        model.invite("new@e.com", "member")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.lastInviteLink).isEqualTo("https://t/invite?token=raw")
        assertThat(s.invitations.map { it.id }).containsExactly("i-1")
        assertThat(s.busy).isFalse()
        assertThat(s.error).isNull()
    }

    @Test
    fun `упавший список приглашений не съедает только что выданную ссылку`() = runTest(dispatcher) {
        val model = opened()
        coEvery { repo.addMember("ws-1", "new@e.com", "member") } throws http(400, "no user with that email")
        coEvery { repo.createInvitation("ws-1", "new@e.com", "member") } returns
            Invitation(id = "i-1", email = "new@e.com", role = "member", link = "https://t/invite?token=raw")
        coEvery { repo.invitations("ws-1") } throws IOException("network")

        model.invite("new@e.com", "member")
        advanceUntilIdle()

        val s = model.state.value
        // Приглашение создано, токен есть только здесь — отдать его обязаны.
        assertThat(s.lastInviteLink).isEqualTo("https://t/invite?token=raw")
        assertThat(s.busy).isFalse()
        assertThat(s.error).isNotNull()
    }

    @Test
    fun `чужой отказ не превращается в приглашение`() = runTest(dispatcher) {
        val model = opened()
        coEvery { repo.addMember("ws-1", "o@e.com", "member") } throws
            http(400, "cannot change your own role")

        model.invite("o@e.com", "member")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.busy).isFalse()
        assertThat(s.error).isNotNull()
        assertThat(s.lastInviteLink).isEmpty()
        coVerify(exactly = 0) { repo.createInvitation(any(), any(), any()) }
    }
}
