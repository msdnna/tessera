package website.msdnna.tessera.e2e

import com.google.common.truth.Truth.assertThat
import kotlinx.coroutines.runBlocking
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.repository.AuthRepository
import website.msdnna.tessera.data.repository.BoardRepository

/**
 * Proves the harness itself before any spec leans on it: the seeded session is
 * accepted by the app's own Retrofit stack, the seeded board reads back through
 * the app's repositories, and [E2eRule] really does isolate one test from the
 * next. A UI spec failing is only meaningful once this passes.
 */
@RunWith(RobolectricTestRunner::class)
class HarnessE2eTest {
    @get:Rule
    val e2e = E2eRule()

    @Test
    fun `seeded session authenticates the app's own api client`() = runBlocking {
        val user = AuthRepository().verify()

        assertThat(user.email).isEqualTo(e2e.account.email)
        assertThat(AppContainer.serverUrl).isEqualTo(E2eBackend.serverUrl)
    }

    // Explicit `Unit`: an assertion that returns a value (Truth's `containsExactly`
    // hands back `Ordered`) would make this an invalid JUnit4 test method.
    @Test
    fun `seeded board reads back through the app's repositories`(): Unit = runBlocking {
        val repo = BoardRepository()
        E2eBackend.createTask(e2e.fixture, "harness task")

        val columns = repo.columns(e2e.fixture.board.id)
        val tasks = repo.tasks(e2e.fixture.board.id)

        assertThat(columns.map { it.id }).containsExactlyElementsIn(e2e.fixture.columns.map { it.id })
        assertThat(tasks.map { it.title }).containsExactly("harness task")
    }

    @Test
    fun `each test gets its own account and an empty board`() = runBlocking {
        // Deliberately overlaps the previous test: if the rule leaked either the
        // session or the seeded data, this board would not be empty.
        assertThat(BoardRepository().tasks(e2e.fixture.board.id)).isEmpty()
        assertThat(AuthRepository().verify().email).isEqualTo(e2e.account.email)
    }
}
