package website.msdnna.tessera.ui.components

import android.content.Context
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
 * Правило по списку дат (freq=custom) на Android-форме (#2932).
 *
 * Список дат — это ВСЯ серия, а срок задачи говорит, где серия стоит сейчас:
 * `nextCustom` (backend/internal/recur) берёт первую дату СТРОГО ПОЗЖЕ срока, так
 * что пройденные даты списка задача уже не отыграет. Пикер же выводил срок из
 * самого списка — минимальная дата, — и потому на середине серии показывал (и
 * отдавал в «Готово») первую дату списка вместо настоящего срока задачи. Открыть
 * форму ради упреждения уведомления было достаточно, чтобы отмотать серию в начало
 * и заставить её заново отыграть все уже пройденные повторения. У web-редактора
 * источник истины — свой `localDue`, а `dates[0]` подставляется только по клику по
 * дню; здесь теперь так же.
 *
 * Экран пришпилен, как в остальных спеках: дефолтные 320×470px Robolectric молча
 * теряют тап ниже кромки.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class DuePickerCustomDatesTest {
    @get:Rule
    val compose = createComposeRule()

    private val ctx = ApplicationProvider.getApplicationContext<Context>()
    private val res = ctx.resources

    private var applied: Triple<String?, String?, Recurrence?>? = null

    private fun mount(dueIso: String?, rule: Recurrence?) {
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

    private fun done() = compose.onNodeWithText(res.getString(R.string.common_done)).performScrollTo().performClick()

    /** Серия 1/8/15/22 марта, задача стоит на 15-м — два повторения уже позади. */
    private fun midSeries() = Recurrence(
        freq = "custom",
        interval = 1,
        dates = listOf("2026-03-01", "2026-03-08", "2026-03-15", "2026-03-22"),
    )

    @Test
    fun `open and confirm in the middle of a series keeps the task's own due date`() {
        mount(dueIso = "2026-03-15T12:00:00Z", rule = midSeries())
        done()

        // Ни одного касания календаря — срок обязан уехать ровно тот, что пришёл.
        assertThat(applied?.first).startsWith("2026-03-15")
        assertThat(applied?.third?.dates).isEqualTo(midSeries().dates)
    }

    @Test
    fun `picking a day still anchors the due to the earliest date on the list`() {
        // Свежая серия: список начинается со срока. Клик по 11 марта делает его
        // самым ранним — якорь серии переезжает туда (как в web-редакторе).
        mount(dueIso = "2026-03-15T12:00:00Z", rule = Recurrence(freq = "custom", interval = 1, dates = listOf("2026-03-15", "2026-03-22")))
        // В сетке марта 2026 (23 фев … 5 апр) число 11 встречается ровно один раз.
        compose.onNodeWithText("11").performScrollTo().performClick()
        done()

        assertThat(applied?.first).startsWith("2026-03-11")
        assertThat(applied?.third?.dates).containsExactly("2026-03-11", "2026-03-15", "2026-03-22")
    }
}
