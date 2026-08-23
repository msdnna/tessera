package website.msdnna.tessera.ui

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
import website.msdnna.tessera.util.withLanguage

/**
 * Язык интерфейса едет из профиля, а не из системной локали устройства — здесь
 * проверяется именно этот механизм: подмена `LocalResources` в [AppLocale] и
 * контекст [withLanguage] для строк вне Compose.
 *
 * Экран пинится размером: дефолтные 320x470 Robolectric'а обрезают композицию
 * снизу, и узел «есть, но за кромкой» дал бы ложно-красный assertIsDisplayed.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp")
class AppLocaleTest {
    @get:Rule
    val compose = createComposeRule()

    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    @Test
    fun `english profile renders english strings`() {
        compose.setContent {
            AppLocale(language = "en") { Text(stringResource(R.string.auth_submit_login)) }
        }
        compose.onNodeWithText("Sign in").assertIsDisplayed()
    }

    @Test
    fun `an unknown language falls back to russian`() {
        compose.setContent {
            AppLocale(language = "de") { Text(stringResource(R.string.auth_submit_login)) }
        }
        compose.onNodeWithText("Войти").assertIsDisplayed()
    }

    /** Смена языка — обычная рекомпозиция: Activity не пересоздаётся, состояние живёт. */
    @Test
    fun `switching the language relabels a live composition`() {
        val language = mutableStateOf("ru")
        compose.setContent {
            AppLocale(language = language.value) { Text(stringResource(R.string.settings_language)) }
        }
        compose.onNodeWithText("Язык").assertIsDisplayed()

        compose.runOnIdle { language.value = "en" }
        compose.onNodeWithText("Language").assertIsDisplayed()
    }

    @Test
    fun `strings outside compose follow the profile language too`() {
        assertThat(appContext.withLanguage("en").getString(R.string.auth_submit_login)).isEqualTo("Sign in")
        assertThat(appContext.withLanguage("ru").getString(R.string.auth_submit_login)).isEqualTo("Войти")
        assertThat(appContext.withLanguage("fr").getString(R.string.auth_submit_login)).isEqualTo("Войти")
    }

    /** Массивы читают тот же [LocalResources], что и `stringResource`, — проверено, а не
     *  предположено: календарь и ось таймлайна берут названия месяцев именно так. */
    @Test
    fun `string arrays follow the profile language`() {
        compose.setContent {
            AppLocale(language = "en") { Text(stringArrayResource(R.array.calendar_months)[0]) }
        }
        compose.onNodeWithText("January").assertIsDisplayed()
    }

    /** Русская форма склоняется по количеству *после «из»* — иначе «1 из 3 подзадачи». */
    @Test
    fun `russian plurals pick the form by the total, not the shown count`() {
        val ru = appContext.withLanguage("ru").resources
        assertThat(ru.getQuantityString(R.plurals.task_subtasks_filtered, 21, 1, 21)).startsWith("1 из 21 подзадачи")
        assertThat(ru.getQuantityString(R.plurals.task_subtasks_filtered, 3, 1, 3)).startsWith("1 из 3 подзадач")
        assertThat(ru.getQuantityString(R.plurals.task_subtasks_filtered, 5, 2, 5)).startsWith("2 из 5 подзадач")

        val en = appContext.withLanguage("en").resources
        assertThat(en.getQuantityString(R.plurals.task_subtasks_filtered, 1, 1, 1)).startsWith("1 of 1 subtask —")
        assertThat(en.getQuantityString(R.plurals.task_subtasks_filtered, 4, 2, 4)).startsWith("2 of 4 subtasks")
    }
}
