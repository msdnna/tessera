package website.msdnna.tessera.ui.theme

import android.content.Context
import androidx.compose.material3.Text
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.AppLocale
import website.msdnna.tessera.util.priorityLabel
import website.msdnna.tessera.util.withLanguage

/**
 * Волна 18: подписи, которые раньше лежали `val`-списками в `ui/theme/Tokens.kt`.
 *
 * Ловушка тут не в переводе, а в моменте вычисления: список на верхнем уровне
 * собирается один раз при загрузке класса, и готовая строка застыла бы на языке
 * первого обращения — смена языка в профиле её бы не переехала. Поэтому оба
 * списка (`AccentThemes` и подписи приоритета) рендерятся дважды подряд в одной
 * JVM: второй язык обязан приехать из тех же самых объектов.
 *
 * Экран пинится размером: дефолтные 320x470 Robolectric'а обрезают композицию
 * снизу и дают ложно-красный assertIsDisplayed.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp")
class TokensLocaleTest {
    @get:Rule
    val compose = createComposeRule()

    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    /** Тот же список `AccentThemes`, две локали подряд — подпись едет из `nameRes`. */
    @Test
    fun `accent names follow the profile language, not the class load language`() {
        val language = mutableStateOf("ru")
        compose.setContent {
            AppLocale(language = language.value) { Text(stringResource(accentByKey("teal").nameRes)) }
        }
        compose.onNodeWithText("Бирюзовый").assertIsDisplayed()

        compose.runOnIdle { language.value = "en" }
        compose.onNodeWithText("Teal").assertIsDisplayed()
    }

    /** Подписи приоритета внутри композиции (пилюля карточки, меню, композер доски). */
    @Test
    fun `priority labels follow the profile language in composition`() {
        val language = mutableStateOf("ru")
        compose.setContent {
            AppLocale(language = language.value) {
                Text(stringArrayResource(R.array.task_priority_labels)[URGENT])
            }
        }
        compose.onNodeWithText("Срочный").assertIsDisplayed()

        compose.runOnIdle { language.value = "en" }
        compose.onNodeWithText("Urgent").assertIsDisplayed()
    }

    /** И вне композиции — журнал синхронизации и разбор конфликтов форматируют
     *  значение поля `priority` там, где `Resources` приходится передавать руками. */
    @Test
    fun `priority labels outside compose follow the profile language too`() {
        assertThat(priorityLabel(appContext.withLanguage("en").resources, URGENT)).isEqualTo("Urgent")
        assertThat(priorityLabel(appContext.withLanguage("ru").resources, URGENT)).isEqualTo("Срочный")
    }

    /** Индекс приходит с сервера: чужое значение не должно ронять форматирование. */
    @Test
    fun `an out-of-range priority has no label`() {
        val res = appContext.withLanguage("ru").resources
        assertThat(priorityLabel(res, -1)).isNull()
        assertThat(priorityLabel(res, URGENT + 1)).isNull()
    }

    /** Подписи и цвета приоритета адресуются одним индексом — разъехавшаяся длина
     *  дала бы «Срочный» бирюзовым цветом низкого. */
    @Test
    fun `priority labels and colors share their index`() {
        val labels = appContext.withLanguage("ru").resources.getStringArray(R.array.task_priority_labels)
        assertThat(labels).hasLength(PriorityColors.size)
    }

    private companion object {
        /** Последний уровень приоритета (0..4) — он же самый заметный в интерфейсе. */
        const val URGENT = 4
    }
}
