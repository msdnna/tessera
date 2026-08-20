package website.msdnna.tessera.util

/*
 * "What's New" + spotlight bookkeeping — the pure half of the feature (web
 * `stores/whatsNew.js`), kept out of the ViewModel so it is testable on the JVM.
 *
 * Both clients write into the SAME per-user acknowledgement keys, so a release
 * dismissed on the web is not shown again on the phone and vice versa — the keys
 * are the contract, not the UI:
 *   whatsnew:<version>  — the changelog card for a release was dismissed
 *   spotlight:<navKey>  — the hint pointing at a sidebar item was dismissed
 *
 * Versions are Android's own (`android/VERSION`), independent of the web's, so
 * the *entries* differ between clients even though the key format doesn't.
 */

/** A one-shot hint pointing at a sidebar nav item ([navKey] matches `activeNav`). */
data class WhatsNewSpotlight(val navKey: String, val title: String, val body: String)

/** One curated release highlight. [items] are plain sentences — the Android card
 *  renders them as a bullet list verbatim (no Markdown pass, unlike the web). */
data class WhatsNewEntry(
    val version: String,
    val date: String,
    val title: String,
    val items: List<String>,
    val spotlight: WhatsNewSpotlight? = null,
)

const val WHATSNEW_PREFIX = "whatsnew:"
const val SPOTLIGHT_PREFIX = "spotlight:"

/**
 * What to surface this session. [baseline] is set instead of the two lists when
 * there is nothing to catch up on but the user still needs a starting point
 * written down (a brand-new account) — ack it and show nothing.
 */
data class WhatsNewPlan(
    val releases: List<WhatsNewEntry> = emptyList(),
    val spotlights: List<WhatsNewSpotlight> = emptyList(),
    val baseline: String? = null,
)

/** Numeric semver compare for plain `x.y.z` strings; >0 when [a] is newer. */
fun compareVersions(a: String, b: String): Int {
    val pa = a.split('.')
    val pb = b.split('.')
    for (i in 0 until maxOf(pa.size, pb.size)) {
        val d = (pa.getOrNull(i)?.trim()?.toIntOrNull() ?: 0) - (pb.getOrNull(i)?.trim()?.toIntOrNull() ?: 0)
        if (d != 0) return d
    }
    return 0
}

/**
 * Decides which release cards and spotlights this user still has coming.
 *
 * The baseline is the highest already-acknowledged `whatsnew:` version — anything
 * newer than that and not newer than the running build is shown, newest first. A
 * user with no acks at all has baseline `0.0.0`, so a first run surfaces the whole
 * curated list once (and clearing the acks makes it show again — testable by hand).
 *
 * [buildAtMillis] is when THIS build landed on the device (`lastUpdateTime`), the
 * Android stand-in for the web bundle's build date: an account created after that
 * moment never updated *into* anything, so the changelog would be noise. Such an
 * account is baselined silently instead.
 */
fun planWhatsNew(
    entries: List<WhatsNewEntry>,
    acked: Set<String>,
    currentVersion: String,
    accountCreatedAt: String?,
    buildAtMillis: Long,
): WhatsNewPlan {
    val ackedVersions = acked.filter { it.startsWith(WHATSNEW_PREFIX) }.map { it.removePrefix(WHATSNEW_PREFIX) }
    if (ackedVersions.isEmpty() && isBrandNewAccount(accountCreatedAt, buildAtMillis)) {
        return WhatsNewPlan(baseline = WHATSNEW_PREFIX + currentVersion)
    }
    val highest = ackedVersions.fold("0.0.0") { max, v -> if (compareVersions(v, max) > 0) v else max }
    val releases = entries
        .filter { compareVersions(it.version, highest) > 0 && compareVersions(it.version, currentVersion) <= 0 }
        .sortedWith { a, b -> compareVersions(b.version, a.version) }
    // Spotlights of those releases, newest first, de-duplicated by navKey: two
    // releases touching the same section must not queue the same arrow twice.
    val seen = mutableSetOf<String>()
    val spotlights = releases
        .mapNotNull { it.spotlight }
        .filter { !acked.contains(SPOTLIGHT_PREFIX + it.navKey) && seen.add(it.navKey) }
    return WhatsNewPlan(releases = releases, spotlights = spotlights)
}

/** Signing up right around the install counts as brand-new: someone who registers
 *  minutes before or after putting the app on their phone has nothing to catch up
 *  on, and the clocks involved (server vs device) need slack anyway. */
private const val SignupGraceMillis = 60L * 60L * 1000L

/**
 * True when the account is younger than this build's arrival (± the grace above),
 * i.e. it never updated *into* the running version.
 *
 * An unknown install time answers "yes" — we cannot prove the user updated into
 * anything, and the cost of guessing wrong sits on the wrong side otherwise: a
 * full-screen card thrown at someone who just signed up (and, in the Robolectric
 * e2e tier, a scrim over the shell that eats the spec's taps). The baseline is
 * still written, so the *next* release reaches them normally.
 */
fun isBrandNewAccount(accountCreatedAt: String?, buildAtMillis: Long): Boolean {
    if (buildAtMillis <= 0L) return true
    val created = parseInstantMillis(accountCreatedAt) ?: return false
    return created >= buildAtMillis - SignupGraceMillis
}
