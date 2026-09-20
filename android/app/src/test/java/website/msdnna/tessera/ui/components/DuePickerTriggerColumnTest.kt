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
 * Мёртвая колонка-триггер на Android-форме (#2932).
 *
 * Сервер продвигает такое правило ровно в одном месте — переносе, чья колонка
 * дословно равна `trigger_column` (backend handlers/task_ops.go) — так что пустая
 * колонка не совпадёт ни с одним uuid, и удалённая с доски тоже. Обе дырки
 * открываются без единой ошибки: селект колонки открывается на плейсхолдере, а
 * «Готово» коммитит `trigger = "column"` как есть, после чего карточка рисует
 * бейдж повтора. Web это уже говорит вслух (`triggerColumnMissing` + предупреждение
 * в панели + пустая сетка) — Android обязан говорить то же.
 *
 * Экран пришпилен, как в остальных спеках: дефолтные 320×470px Robolectric молча
 * теряют тап ниже кромки.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class DuePickerTriggerColumnTest {
    @get:Rule
    val compose = createComposeRule()

    private val ctx = ApplicationProvider.getApplicationContext<Context>()
    private val res = ctx.resources

    private var applied: Triple<String?, String?, Recurrence?>? = null

    private fun mount(rule: Recurrence? = null) {
        AppContainer.init(ctx)
        compose.setContent {
            TesseraTheme {
                DueDateTimePicker(
                    initialIso = "2026-06-10T09:00:00Z",
                    initialStartIso = null,
                    initialRecurrence = rule,
                    columns = listOf(
                        BoardColumn(id = "col-1", name = "К работе", position = 1.0),
                        BoardColumn(id = "col-2", name = "В процессе", position = 2.0),
                    ),
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

    private fun unsetWarning() = compose.onNodeWithText(res.getString(R.string.due_picker_trigger_column_unset))
    private fun goneWarning() = compose.onNodeWithText(res.getString(R.string.due_picker_trigger_column_gone))

    @Test
    fun `picking the column trigger without a column is called out`() {
        mount(rule = Recurrence(freq = "daily", interval = 1))
        unsetWarning().assertDoesNotExist()

        // Селект события открывается на «по завершении»; переводим на «по переносу».
        // Пункт списка живёт в Popup — прокручивать там нечего, только тапать.
        compose.onNodeWithText(res.getString(R.string.recur_trigger_complete)).performScrollTo().performClick()
        compose.onNodeWithText(res.getString(R.string.recur_trigger_column)).performClick()

        unsetWarning().performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the dead rule is committed all the same — the warning is the only sign`() {
        mount(rule = Recurrence(freq = "daily", interval = 1, trigger = "column"))
        compose.onNodeWithText(res.getString(R.string.common_done)).performScrollTo().performClick()

        // Ровно то состояние, из-за которого нужно предупреждение: триггер есть,
        // колонки нет, правило уезжает на сервер живым на вид.
        assertThat(applied?.third?.trigger).isEqualTo("column")
        assertThat(applied?.third?.triggerColumn).isNull()
    }

    @Test
    fun `a column that has left the board is called out separately`() {
        mount(rule = Recurrence(freq = "daily", interval = 1, trigger = "column", triggerColumn = "col-gone"))
        goneWarning().performScrollTo().assertIsDisplayed()
        unsetWarning().assertDoesNotExist()
    }

    @Test
    fun `a reachable column gets no warning`() {
        mount(rule = Recurrence(freq = "daily", interval = 1, trigger = "column", triggerColumn = "col-2"))
        unsetWarning().assertDoesNotExist()
        goneWarning().assertDoesNotExist()
    }
}
