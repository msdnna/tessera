package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.Milestone

/**
 * Окно этапа собирается из ресурсов вместе с самими датами (#2803, волна 11) —
 * отсюда Robolectric и проверка обеих локалей. Раньше эти проверки жили в
 * `MiscUtilTest`, где ресурсов нет.
 */
@RunWith(RobolectricTestRunner::class)
class MilestonesTest {
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val ru: Resources get() = res("ru")
    private val en: Resources get() = res("en")

    @Test
    fun `range both sides`() {
        assertThat(Milestones.range(ru, "2026-06-01T00:00:00Z", "2026-06-30T00:00:00Z"))
            .isEqualTo("1 июн. 2026 г. – 30 июн. 2026 г.")
        assertThat(Milestones.range(en, "2026-06-01T00:00:00Z", "2026-06-30T00:00:00Z"))
            .isEqualTo("Jun 1, 2026 – Jun 30, 2026")
    }

    @Test
    fun `range due only and start only`() {
        assertThat(Milestones.range(ru, null, "2026-06-30")).isEqualTo("до 30 июн. 2026 г.")
        assertThat(Milestones.range(ru, "2026-06-01", null)).isEqualTo("с 1 июн. 2026 г.")
        assertThat(Milestones.range(en, null, "2026-06-30")).isEqualTo("until Jun 30, 2026")
        assertThat(Milestones.range(en, "2026-06-01", null)).isEqualTo("from Jun 1, 2026")
    }

    @Test
    fun `range neither is empty`() {
        assertThat(Milestones.range(ru, null, null)).isEmpty()
        assertThat(Milestones.range(ru, "", "")).isEmpty()
    }

    @Test
    fun `range from milestone and null milestone`() {
        assertThat(Milestones.range(ru, null as Milestone?)).isEmpty()
        val m = Milestone(startDate = "2026-06-01", dueDate = "2026-06-30")
        assertThat(Milestones.range(ru, m)).isEqualTo("1 июн. 2026 г. – 30 июн. 2026 г.")
    }
}
