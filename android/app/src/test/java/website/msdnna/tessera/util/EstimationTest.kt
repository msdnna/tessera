package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.EstimationConfig
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.Workspace

@RunWith(RobolectricTestRunner::class)
class EstimationTest {
    /** Ресурсы на конкретном языке: подписи берутся из профиля, а не из локали устройства. */
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val ru: Resources get() = res("ru")

    private val time = EstimationConfig(unit = "time", hoursPerDay = 8.0, daysPerWeek = 5.0)
    private val fib = EstimationConfig(unit = "points", pointsScale = "fibonacci")
    private val tshirt = EstimationConfig(unit = "points", pointsScale = "tshirt")
    private val linear = EstimationConfig(unit = "points", pointsScale = "linear")
    private val custom = EstimationConfig(unit = "custom", customLabel = "тикетов")

    // ── resolve ──────────────────────────────────────────────────────────────
    @Test
    fun `resolve prefers project then workspace then default`() {
        val ws = Workspace(estimation = fib)
        val prj = Project(estimation = tshirt)
        assertThat(Estimation.resolve(prj, ws)).isEqualTo(tshirt)
        assertThat(Estimation.resolve(null, ws)).isEqualTo(fib)
        assertThat(Estimation.resolve(null, null)).isEqualTo(Estimation.DEFAULT)
        assertThat(Estimation.resolve(Project(estimation = null), ws)).isEqualTo(fib)
    }

    // ── parse: time ──────────────────────────────────────────────────────────
    @Test
    fun `parse null blank whitespace yields null`() {
        assertThat(Estimation.parse(null, time)).isNull()
        assertThat(Estimation.parse("", time)).isNull()
        assertThat(Estimation.parse("   ", time)).isNull()
    }

    @Test
    fun `parse bare number is hours`() {
        assertThat(Estimation.parse("2", time)).isEqualTo(120.0)
    }

    @Test
    fun `parse mixed english tokens to minutes`() {
        // 1w=5*8*60=2400, 2d=2*480=960, 3h=180, 30m=30 → 3570
        assertThat(Estimation.parse("1w 2d 3h 30m", time)).isEqualTo(3570.0)
    }

    @Test
    fun `parse russian tokens`() {
        assertThat(Estimation.parse("3д 4ч", time)).isEqualTo(3.0 * 480 + 240)
        assertThat(Estimation.parse("1н", time)).isEqualTo(2400.0)
        assertThat(Estimation.parse("90м", time)).isEqualTo(90.0)
    }

    @Test
    fun `parse decimals and comma`() {
        assertThat(Estimation.parse("1.5h", time)).isEqualTo(90.0)
        assertThat(Estimation.parse("1,5ч", time)).isEqualTo(90.0)
    }

    @Test
    fun `parse unrecognised text yields null`() {
        assertThat(Estimation.parse("abc", time)).isNull()
    }

    @Test
    fun `parse honours custom hours per day`() {
        val cfg = EstimationConfig(unit = "time", hoursPerDay = 6.0, daysPerWeek = 5.0)
        assertThat(Estimation.parse("1d", cfg)).isEqualTo(360.0)
    }

    // ── parse: points / custom ───────────────────────────────────────────────
    @Test
    fun `parse points takes positive number`() {
        assertThat(Estimation.parse("5", fib)).isEqualTo(5.0)
        assertThat(Estimation.parse("0", fib)).isNull()
        assertThat(Estimation.parse("-3", fib)).isNull()
    }

    @Test
    fun `parse tshirt label case-insensitive`() {
        assertThat(Estimation.parse("M", tshirt)).isEqualTo(3.0)
        assertThat(Estimation.parse("xxl", tshirt)).isEqualTo(13.0)
        assertThat(Estimation.parse("7", tshirt)).isEqualTo(7.0) // numeric still accepted
        assertThat(Estimation.parse("huge", tshirt)).isNull()
    }

    @Test
    fun `parse custom takes number`() {
        assertThat(Estimation.parse("8", custom)).isEqualTo(8.0)
    }

    // ── format ───────────────────────────────────────────────────────────────
    @Test
    fun `format null or non-positive is empty`() {
        assertThat(Estimation.format(ru, null, time)).isEmpty()
        assertThat(Estimation.format(ru, 0.0, time)).isEmpty()
        assertThat(Estimation.format(ru, -1.0, time)).isEmpty()
    }

    @Test
    fun `format time compresses to working units`() {
        // 30h with 8h day = 3d 6h → but per week roll: 30h=1800m; mpw=2400, mpd=480
        assertThat(Estimation.format(ru, 1800.0, time)).isEqualTo("3д 6ч")
        assertThat(Estimation.format(ru, 90.0, time)).isEqualTo("1ч 30м")
        assertThat(Estimation.format(ru, 2400.0, time)).isEqualTo("1н")
    }

    /** Суффиксы единиц — не просто перевод: на английском их читает обратно [Estimation.parse]. */
    @Test
    fun `format time speaks the profile language`() {
        val en = res("en")
        assertThat(Estimation.format(en, 1800.0, time)).isEqualTo("3d 6h")
        assertThat(Estimation.format(en, 2400.0, time)).isEqualTo("1w")
        assertThat(Estimation.parse(Estimation.format(en, 1800.0, time), time)).isEqualTo(1800.0)
    }

    @Test
    fun `format tshirt maps back to label`() {
        assertThat(Estimation.format(ru, 5.0, tshirt)).isEqualTo("L")
        assertThat(Estimation.format(ru, 4.0, tshirt)).isEqualTo("4") // no exact label
    }

    @Test
    fun `format points has SP suffix`() {
        assertThat(Estimation.format(ru, 5.0, fib)).isEqualTo("5 SP")
        assertThat(Estimation.format(ru, 2.5, fib)).isEqualTo("2.5 SP")
    }

    @Test
    fun `format custom uses label`() {
        assertThat(Estimation.format(ru, 3.0, custom)).isEqualTo("3 тикетов")
        // Своя единица — пользовательский текст: он остаётся как введён на любом языке.
        assertThat(Estimation.format(res("en"), 3.0, custom)).isEqualTo("3 тикетов")
        assertThat(Estimation.format(ru, 3.0, EstimationConfig(unit = "custom", customLabel = ""))).isEqualTo("3")
    }

    // ── scaleOptions ─────────────────────────────────────────────────────────
    @Test
    fun `scaleOptions per scale`() {
        assertThat(Estimation.scaleOptions(time)).isEmpty()
        assertThat(Estimation.scaleOptions(fib).map { it.second }).containsExactly(
            1.0, 2.0, 3.0, 5.0, 8.0, 13.0, 21.0,
        ).inOrder()
        assertThat(Estimation.scaleOptions(linear)).hasSize(10)
        assertThat(Estimation.scaleOptions(tshirt).first()).isEqualTo("XS" to 1.0)
    }

    // ── unitName / placeholder ───────────────────────────────────────────────
    @Test
    fun `unitName per unit`() {
        assertThat(Estimation.unitName(ru, time)).isEqualTo("Время")
        assertThat(Estimation.unitName(ru, fib)).isEqualTo("Стори-поинты")
        assertThat(Estimation.unitName(ru, custom)).isEqualTo("тикетов")
        assertThat(Estimation.unitName(ru, EstimationConfig(unit = "custom", customLabel = " ")))
            .isEqualTo("Единицы")
        assertThat(Estimation.unitName(res("en"), fib)).isEqualTo("Story points")
    }

    @Test
    fun `placeholder per unit`() {
        assertThat(Estimation.placeholder(ru, fib)).isEqualTo("напр. 5")
        assertThat(Estimation.placeholder(ru, custom)).isEqualTo("напр. 8")
        assertThat(Estimation.placeholder(ru, time)).contains("3д 4ч")
        assertThat(Estimation.placeholder(res("en"), time)).contains("3d 4h")
    }

    // ── toDays ───────────────────────────────────────────────────────────────
    @Test
    fun `toDays only for time`() {
        // one working week (2400m) → 7 calendar days
        assertThat(Estimation.toDays(2400.0, time)).isEqualTo(7.0)
        assertThat(Estimation.toDays(5.0, fib)).isNull()
        assertThat(Estimation.toDays(null, time)).isNull()
        assertThat(Estimation.toDays(0.0, time)).isNull()
    }

    // ── sum ──────────────────────────────────────────────────────────────────
    @Test
    fun `sum ignores null and non-positive`() {
        assertThat(Estimation.sum(listOf(10.0, null, -5.0, 20.0))).isEqualTo(30.0)
        assertThat(Estimation.sum(listOf(null, 0.0, -1.0))).isNull()
        assertThat(Estimation.sum(emptyList())).isNull()
    }
}
