package website.msdnna.tessera.ui.components

import android.content.Context
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Recurrence
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * Повтор без срока на Android-форме (#2932).
 *
 * Правило отсчитывается от срока и больше ни от чего: `recurAdvance` обрывается на
 * задаче с пустым due_date раньше, чем сравнивает триггер, а воркер расписания
 * фильтрует `due_date IS NOT NULL`. У web-редактора выбор частоты сам ставит дату,
 * и туда попадают только через «очистить срок»; здесь чип частоты не ставит ничего,
 * так что задача без срока уезжает на сервер с живым на вид правилом сразу. Кроме
 * подписи об этом сказать негде — календарь просто остаётся неразмеченным, а
 * карточка рисует бейдж повтора.
 *
 * Экран пришпилен, как в остальных спеках: дефолтные 320×470px Robolectric молча
 * теряют тап ниже кромки.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class DuePickerNoDueTest {
    @get:Rule
    val compose = createComposeRule()

    private val ctx = ApplicationProvider.getApplicationContext<Context>()
    private val res = ctx.resources

    private var applied: Triple<String?, String?, Recurrence?>? = null

    private fun mount(dueIso: String?, rule: Recurrence? = null) {
        AppContainer.init(ctx)
        compose.setContent {
            TesseraTheme {
                DueDateTimePicker(
                    initialIso = dueIso,
                    initialStartIso = null,
                    initialRecurrence = rule,
                    columns = listOf(BoardColumn(id = "col-1", name = "К работе", position = 1.0)),
                    notifyEnabled = null,
                    notifyLead = null,
                    notifyRepeat = null,
                    onApply = { due, start, rec -> applied = Triple(due, start, rec) },
                    onNotify = { _, _, _ -> },
                    onDismiss = {},
                )
            }
        }
    }

    private fun hint() = compose.onNodeWithText(res.getString(R.string.due_picker_no_due))

    @Test
    fun `frequency picked on a task without a due date is called out`() {
        mount(dueIso = null)
        hint().assertDoesNotExist()

        compose.onNodeWithText(res.getString(R.string.recur_freq_daily)).performScrollTo().performClick()

        hint().performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the dead rule is committed all the same — the hint is the only warning`() {
        mount(dueIso = null)
        compose.onNodeWithText(res.getString(R.string.recur_freq_daily)).performScrollTo().performClick()
        compose.onNodeWithText(res.getString(R.string.common_done)).performScrollTo().performClick()

        // Ровно то состояние, из-за которого нужна подпись: срок пуст, правило живое.
        assertThat(applied?.first).isNull()
        assertThat(applied?.third?.freq).isEqualTo("daily")
    }

    @Test
    fun `a dated task gets no such hint`() {
        mount(dueIso = "2026-06-10T09:00:00Z")
        compose.onNodeWithText(res.getString(R.string.recur_freq_daily)).performScrollTo().performClick()
        hint().assertDoesNotExist()
    }

    @Test
    fun `an already stored rule shows the hint the moment the task opens undated`() {
        // Через API/другой клиент правило без срока уже могло осесть в базе —
        // форма должна объявлять его мёртвым ещё до единого касания.
        mount(dueIso = null, rule = Recurrence(freq = "weekly", interval = 1))
        hint().performScrollTo().assertIsDisplayed()
    }
}
