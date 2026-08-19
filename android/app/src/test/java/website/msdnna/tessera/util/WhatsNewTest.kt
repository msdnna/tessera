package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The "what does this user still have coming" decision (#2766). Pure logic, so
 * the interesting cases — a skipped release, a partly-acked history, a fresh
 * sign-up — are asserted here rather than through the UI.
 */
class WhatsNewTest {
    private val spot = WhatsNewSpotlight("documents", "Документы", "Загляните")

    private val entries = listOf(
        WhatsNewEntry("0.70.0", "2026-08-20", "Новое", listOf("а")),
        WhatsNewEntry("0.69.0", "2026-08-17", "Документы", listOf("б"), spotlight = spot),
        WhatsNewEntry("0.68.0", "2026-08-12", "Пуши", listOf("в")),
    )

    // The build landed on 2026-08-01; the account predates it, so the brand-new
    // gate stays shut unless a test opens it on purpose.
    private val buildAt = 1_785_542_400_000L
    private val oldAccount = "2026-01-01T00:00:00Z"

    @Test
    fun `newer entries only, newest first`() {
        val plan = planWhatsNew(entries, setOf("whatsnew:0.68.0"), "0.70.0", oldAccount, buildAt)
        assertThat(plan.releases.map { it.version }).containsExactly("0.70.0", "0.69.0").inOrder()
    }

    @Test
    fun `a release newer than the running build is not shown`() {
        // The 0.70.0 entry is already in the source when the phone still runs 0.69.0.
        val plan = planWhatsNew(entries, setOf("whatsnew:0.68.0"), "0.69.0", oldAccount, buildAt)
        assertThat(plan.releases.map { it.version }).containsExactly("0.69.0")
    }

    @Test
    fun `no acks at all surfaces the whole curated list once`() {
        val plan = planWhatsNew(entries, emptySet(), "0.70.0", oldAccount, buildAt)
        assertThat(plan.releases).hasSize(3)
        assertThat(plan.baseline).isNull()
    }

    @Test
    fun `everything acked shows nothing`() {
        val acked = setOf("whatsnew:0.70.0", "spotlight:documents")
        val plan = planWhatsNew(entries, acked, "0.70.0", oldAccount, buildAt)
        assertThat(plan.releases).isEmpty()
        assertThat(plan.spotlights).isEmpty()
    }

    @Test
    fun `an ack from the web counts here too`() {
        // Keys are shared between clients: only the versions differ per component,
        // and a higher ack simply raises the baseline.
        val plan = planWhatsNew(entries, setOf("whatsnew:0.69.0"), "0.70.0", oldAccount, buildAt)
        assertThat(plan.releases.map { it.version }).containsExactly("0.70.0")
    }

    @Test
    fun `spotlights come from the shown releases and skip the acked ones`() {
        val fresh = planWhatsNew(entries, emptySet(), "0.70.0", oldAccount, buildAt)
        assertThat(fresh.spotlights.map { it.navKey }).containsExactly("documents")

        val dismissed = planWhatsNew(entries, setOf("spotlight:documents"), "0.70.0", oldAccount, buildAt)
        assertThat(dismissed.releases).hasSize(3) // the card still catches up…
        assertThat(dismissed.spotlights).isEmpty() // …but the hint stays dismissed
    }

    @Test
    fun `the same navKey queues once even from two releases`() {
        val twice = entries + WhatsNewEntry("0.67.0", "2026-08-01", "Раньше", listOf("г"), spotlight = spot)
        val plan = planWhatsNew(twice, emptySet(), "0.70.0", oldAccount, buildAt)
        assertThat(plan.spotlights).hasSize(1)
    }

    @Test
    fun `a brand-new account is baselined silently`() {
        // Registered after this build landed — it never updated into anything.
        val created = millisToUtcIso(buildAt + 60_000)
        val plan = planWhatsNew(entries, emptySet(), "0.70.0", created, buildAt)
        assertThat(plan.baseline).isEqualTo("whatsnew:0.70.0")
        assertThat(plan.releases).isEmpty()
    }

    @Test
    fun `signing up shortly before the install still counts as brand-new`() {
        // Registered on the web, then installed the app: nothing to catch up on,
        // and the two clocks involved need the slack anyway.
        assertThat(isBrandNewAccount(millisToUtcIso(buildAt - 5 * 60_000), buildAt)).isTrue()
    }

    @Test
    fun `an account older than the build sees the changelog`() {
        val created = millisToUtcIso(buildAt - 7 * 86_400_000L)
        val plan = planWhatsNew(entries, emptySet(), "0.70.0", created, buildAt)
        assertThat(plan.baseline).isNull()
        assertThat(plan.releases).isNotEmpty()
    }

    @Test
    fun `the brand-new gate only applies before the first ack`() {
        // A young account that has already dismissed something is a normal user:
        // its next release must still reach it.
        val created = millisToUtcIso(buildAt + 60_000)
        val plan = planWhatsNew(entries, setOf("whatsnew:0.69.0"), "0.70.0", created, buildAt)
        assertThat(plan.baseline).isNull()
        assertThat(plan.releases.map { it.version }).containsExactly("0.70.0")
    }

    @Test
    fun `an unparseable created_at shows rather than hides`() {
        assertThat(isBrandNewAccount(null, buildAt)).isFalse()
        assertThat(isBrandNewAccount("", buildAt)).isFalse()
        assertThat(isBrandNewAccount("не дата", buildAt)).isFalse()
    }

    @Test
    fun `an unknown install time hides the card instead of guessing`() {
        // No package info (Robolectric, or a package manager that refused): we
        // cannot prove the user updated into this build, so baseline and move on.
        val plan = planWhatsNew(entries, emptySet(), "0.70.0", oldAccount, 0L)
        assertThat(plan.baseline).isEqualTo("whatsnew:0.70.0")
        assertThat(plan.releases).isEmpty()
    }

    @Test
    fun `version compare is numeric, not lexicographic`() {
        assertThat(compareVersions("0.70.0", "0.9.0")).isGreaterThan(0)
        assertThat(compareVersions("0.9.0", "0.10.0")).isLessThan(0)
        assertThat(compareVersions("1.0", "1.0.0")).isEqualTo(0)
        assertThat(compareVersions("0.70.1", "0.70.0")).isGreaterThan(0)
    }
}
