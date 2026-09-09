package website.msdnna.tessera.ui.components

import android.content.Context
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * Переключатель «не проверять сертификат» (#2896).
 *
 * Главное здесь — подпись, а не сам тумблер. Включённый режим снимает
 * единственную защиту от подмены соединения, и строка под ним — единственное
 * место, где об этом сказано. Сборка, в которой подпись не меняется, выглядит
 * ровно так же рабочей, поэтому оба текста проверяются отдельно.
 *
 * Экран пришпилен, как в остальных спеках: дефолтные 320×470px Robolectric
 * молча теряют тап ниже кромки.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class InsecureTlsRowTest {
    @get:Rule
    val compose = createComposeRule()

    private val res = ApplicationProvider.getApplicationContext<Context>().resources

    private fun mount(checked: Boolean, onCheckedChange: (Boolean) -> Unit = {}) {
        compose.setContent {
            TesseraTheme {
                InsecureTlsRow(
                    checked = checked,
                    onCheckedChange = onCheckedChange,
                    labelColor = Color.Black,
                    hintColor = Color.Gray,
                )
            }
        }
    }

    @Test
    fun `off state offers the fix without the warning`() {
        mount(checked = false)
        compose.onNodeWithText(res.getString(R.string.tls_insecure_label)).assertIsDisplayed()
        compose.onNodeWithText(res.getString(R.string.tls_insecure_hint_off)).assertIsDisplayed()
    }

    @Test
    fun `on state says the connection can be intercepted`() {
        mount(checked = true)
        compose.onNodeWithText(res.getString(R.string.tls_insecure_hint_on)).assertIsDisplayed()
    }

    @Test
    fun `the switch reports the flip`() {
        var got: Boolean? = null
        mount(checked = false, onCheckedChange = { got = it })
        compose.onNodeWithTag(TestTags.TLS_INSECURE_SWITCH).performClick()
        assertThat(got).isTrue()
    }
}
