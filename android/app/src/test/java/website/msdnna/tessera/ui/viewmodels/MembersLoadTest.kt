package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
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
 * Загрузка диалога участников (#2932).
 *
 * Диалог открыт каждому — `ListMembers` стоит за `requireMember`, и кнопка
 * «Участники» показана всем. А `ListInvitations` — уже за `requireManager`:
 * рядовой участник получал на нём 403, исключение выбрасывало общий
 * `runCatching`, и вместе с ним терялся ростер, который сервер уже отдал первым
 * запросом. Веб чинится тем же изменением (`MembersModal.vue`, `allSettled`).
 */
@OptIn(ExperimentalCoroutinesApi::class)
class MembersLoadTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<WorkspaceRepository>()

    private val owner = Member(userId = "u-owner", role = "owner", email = "o@e.com")
    private val plain = Member(userId = "u-plain", role = "member", email = "p@e.com")
    private val roster = listOf(owner, plain)

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun httpError(code: Int, error: String): HttpException {
        val raw = okhttp3.Response.Builder()
            .code(code)
            .message(error)
            .protocol(okhttp3.Protocol.HTTP_1_1)
            .request(okhttp3.Request.Builder().url("http://localhost/").build())
            .build()
        val body = """{"error":"$error"}""".toResponseBody("application/json".toMediaType())
        return HttpException(Response.error<Any>(body, raw))
    }

    private fun forbidden(): HttpException = httpError(403, "forbidden")

    /** Отказ `AddMember`, по которому диалог уходит в ветку приглашения. */
    private fun noSuchUser(): HttpException = httpError(404, "no user with that email")

    private fun vm(meId: String) = MembersViewModel(repo, readMeId = { meId })

    @Test
    fun `рядовой участник видит ростер и не спрашивает приглашения`() = runTest(dispatcher) {
        coEvery { repo.members("ws-1") } returns roster
        val model = vm("u-plain")

        model.load("ws-1")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.members.map { it.userId }).containsExactly("u-owner", "u-plain").inOrder()
        assertThat(s.canManage).isFalse()
        assertThat(s.error).isNull()
        assertThat(s.loading).isFalse()
        // Запрос за приглашениями не уходит вовсе: сервер на него ответит 403.
        coVerify(exactly = 0) { repo.invitations(any()) }
    }

    @Test
    fun `403 на приглашениях не уносит уже полученный ростер`() = runTest(dispatcher) {
        // Даже если гвард разойдётся с сервером (роль в ростере устарела),
        // отказ на манагерском запросе не должен опустошать диалог.
        coEvery { repo.members("ws-1") } returns roster
        coEvery { repo.invitations("ws-1") } throws forbidden()
        val model = vm("u-owner")

        model.load("ws-1")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.members.map { it.userId }).containsExactly("u-owner", "u-plain").inOrder()
        assertThat(s.canManage).isTrue()
        assertThat(s.error).isNotNull()
    }

    @Test
    fun `менеджер получает и ростер, и приглашения`() = runTest(dispatcher) {
        coEvery { repo.members("ws-1") } returns roster
        coEvery { repo.invitations("ws-1") } returns listOf(Invitation(id = "i-1", email = "new@e.com", role = "member"))
        val model = vm("u-owner")

        model.load("ws-1")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.canManage).isTrue()
        assertThat(s.invitations.map { it.id }).containsExactly("i-1")
        assertThat(s.error).isNull()
    }

    @Test
    fun `упавший ростер — это ошибка, а не пустой список молча`() = runTest(dispatcher) {
        coEvery { repo.members("ws-1") } throws forbidden()
        val model = vm("u-plain")

        model.load("ws-1")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.loading).isFalse()
        assertThat(s.error).isNotNull()
        assertThat(s.members).isEmpty()
        coVerify(exactly = 0) { repo.invitations(any()) }
    }

    @Test
    fun `диалог на другом пространстве не показывает данные предыдущего`() = runTest(dispatcher) {
        // ViewModel привязан к активности, а не к окну: закрыли диалог,
        // переключили пространство, открыли снова — и в состоянии всё ещё лежат
        // ростер, приглашения и ссылка прошлого. Тут вдобавок падает ростер
        // второго, так что без сброса чужие данные не мелькают, а остаются.
        coEvery { repo.members("ws-1") } returns roster
        coEvery { repo.invitations("ws-1") } returns listOf(Invitation(id = "i-1", email = "new@e.com", role = "member"))
        coEvery { repo.addMember("ws-1", "new@e.com", "member") } throws noSuchUser()
        coEvery { repo.createInvitation("ws-1", "new@e.com", "member") } returns
            Invitation(id = "i-1", email = "new@e.com", role = "member", link = "https://t/invite?token=secret")
        coEvery { repo.members("ws-2") } throws forbidden()
        val model = vm("u-owner")

        model.load("ws-1")
        model.invite("new@e.com", "member")
        advanceUntilIdle()
        assertThat(model.state.value.invitations).isNotEmpty()
        assertThat(model.state.value.lastInviteLink).isNotEmpty()

        model.load("ws-2")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.members).isEmpty()
        assertThat(s.invitations).isEmpty()
        assertThat(s.canManage).isFalse()
        // Ссылка — носитель токена приглашения в ws-1; в диалоге ws-2 ей не место.
        assertThat(s.lastInviteLink).isEmpty()
        assertThat(s.error).isNotNull()
    }

    @Test
    fun `поздний ответ прошлого пространства не дописывается в состояние`() = runTest(dispatcher) {
        // Две загрузки в полёте разом: диалог переоткрыли на другом пространстве,
        // пока первый запрос ещё шёл.
        coEvery { repo.members("ws-1") } coAnswers {
            delay(50)
            roster
        }
        coEvery { repo.members("ws-2") } returns listOf(plain)
        val model = vm("u-plain")

        model.load("ws-1")
        advanceTimeBy(10)
        model.load("ws-2")
        advanceUntilIdle()

        assertThat(model.state.value.members.map { it.userId }).containsExactly("u-plain")
    }

    @Test
    fun `свой id доезжает до состояния — по нему запирается собственная строка`() = runTest(dispatcher) {
        coEvery { repo.members("ws-1") } returns roster
        coEvery { repo.invitations("ws-1") } returns emptyList()
        val model = vm("u-owner")

        model.load("ws-1")
        advanceUntilIdle()

        assertThat(model.state.value.meId).isEqualTo("u-owner")
    }
}
