package website.msdnna.tessera.ui.components

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.dp
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.model.Task

/**
 * Границы колонок под дроп живут ровно столько, сколько сама колонка (#2932).
 *
 * `dropColumn` регистрирует окно-Rect колонки, а `resolveDrop` выбирает цель по
 * ПЕРВОМУ зарегистрированному прямоугольнику, который накрыл палец. Пока Rect
 * удалённой колонки оставался в мапе, соседка, съехавшая на её место, ловила
 * чужие дропы: карточка уходила в несуществующую колонку, сервер отвечал 400
 * «column does not belong to this board» — и так на каждый дроп, пока не уйдёшь
 * с доски. Подзадаче доставалось сильнее: `dropTask` сначала откепляет её от
 * родителя и только потом двигает, так что «неудачный» дроп всё равно выдёргивал
 * её наверх.
 *
 * Поэтому здесь проверяется не мапа сама по себе, а весь путь: колонка ушла из
 * композиции → её границы сняты → палец над её бывшим местом резолвится в ту
 * колонку, которая там теперь стоит.
 *
 * Экран пришпилен, как в остальных спеках: дефолтные 320×470px Robolectric
 * не вместили бы три дорожки.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class BoardDropBoundsTest {
    @get:Rule
    val compose = createComposeRule()

    private val state = BoardDragState()
    private val lanes = mutableStateOf(listOf("a", "b", "c"))

    private fun mount() {
        compose.setContent {
            Row {
                lanes.value.forEach { id ->
                    key(id) { Spacer(Modifier.size(100.dp).dropColumn(state, id)) }
                }
            }
        }
        compose.waitForIdle()
    }

    /** Ставит палец в центр [rect] с поднятой (top-level) карточкой в руке. */
    private fun grabOver(x: Float, y: Float) {
        state.start(Task(id = "t-1", columnId = "a", title = "карточка"), Offset.Zero, Offset.Zero)
        state.pointer = Offset(x, y)
    }

    @Test
    fun `a column that leaves the board takes its drop bounds with it`() {
        mount()
        assertThat(state.columnBounds.keys).containsExactly("a", "b", "c")
        val gone = state.columnBounds.getValue("b")

        compose.runOnIdle { lanes.value = listOf("a", "c") }
        compose.waitForIdle()

        assertThat(state.columnBounds.keys).containsExactly("a", "c")
        // «c» действительно заняла освободившееся место — иначе проверка ниже
        // ничего бы не стоила.
        assertThat(state.columnBounds.getValue("c").left).isEqualTo(gone.left)
    }

    @Test
    fun `a drop over the freed slot lands in the column that took it`() {
        mount()
        val gone = state.columnBounds.getValue("b")

        compose.runOnIdle { lanes.value = listOf("a", "c") }
        compose.waitForIdle()
        grabOver(gone.center.x, gone.center.y)

        val drop = state.resolveDrop({ emptyList() }, { emptyList() })
        assertThat(drop).isEqualTo(Drop.ToColumn("c", null, null))
    }
}
