package website.msdnna.tessera.ui.components

import android.content.Context
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
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
 * «Пропускать выходные» на форме срока против выбранных дней недели (#2932).
 *
 * Две настройки отвечают на один вопрос — в какие дни повторять, — и сервер
 * оставляет последнее слово за явным набором дней (`recur.normalized`,
 * backend/internal/recur/recur.go): иначе толчок субботы на понедельник переписывал
 * выбранный день и попутно съедал интервал правила. Web-редактор галочку в этом
 * случае просто не показывает (`skipWeekendsMoot`); здесь теперь так же — и правило,
 * собранное формой, флаг не несёт.
 *
 * Экран пришпилен, как в остальных спеках: дефолтные 320×470px Robolectric молча
 * теряют тап ниже кромки.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class DuePickerSkipWeekendsTest {
    @get:Rule
    val compose = createComposeRule()

    private val ctx = ApplicationProvider.getApplicationContext<Context>()
    private val res = ctx.resources

    private var applied: Recurrence? = null

    private fun mount(rule: Recurrence?) {
        AppContainer.init(ctx)
        compose.setContent {
            TesseraTheme {
                DueDateTimePicker(
                    initialIso = "2026-03-07T09:00:00Z",
                    initialStartIso = null,
                    initialRecurrence = rule,
                    columns = listOf(BoardColumn(id = "col-1", name = "К работе", position = 1.0)),
                    notifyEnabled = null,
                    notifyLead = null,
                    notifyRepeat = null,
                    onApply = { _, _, rec -> applied = rec },
                    onNotify = { _, _, _ -> },
                    onDismiss = {},
                )
            }
        }
    }

    private fun done() = compose.onNodeWithText(res.getString(R.string.common_done)).performScrollTo().performClick()

    private fun skipToggle() = compose.onAllNodesWithText(res.getString(R.string.due_picker_skip_weekends))

    /** Старое правило из базы: сохранено с обоими полями, приезжает на клиент как есть. */
    @Test
    fun `weekly with picked weekdays hides the toggle and drops the flag`() {
        mount(Recurrence(freq = "weekly", interval = 2, weekdays = listOf(6), skipWeekends = true))

        assertThat(skipToggle().fetchSemanticsNodes()).isEmpty()

        done()
        assertThat(applied?.weekdays).containsExactly(6)
        assertThat(applied?.interval).isEqualTo(2)
        assertThat(applied?.skipWeekends).isFalse()
    }

    /** Недельное правило БЕЗ явных дней противоречия не создаёт — галочка на месте. */
    @Test
    fun `weekly without weekdays keeps the toggle and the flag`() {
        mount(Recurrence(freq = "weekly", interval = 1, skipWeekends = true))

        compose.onNodeWithText(res.getString(R.string.due_picker_skip_weekends)).performScrollTo().assertIsDisplayed()

        done()
        assertThat(applied?.skipWeekends).isTrue()
    }
}
