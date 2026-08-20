package website.msdnna.tessera.util

/*
 * "What's New" + spotlight bookkeeping — the pure half of the feature (web
 * `stores/whatsNew.js`), kept out of the ViewModel so it is testable on the JVM.
 *
 * Both clients share ONE per-user acknowledgement store, so the keys are the
 * contract, not the UI:
 *   whatsnew:android:<version>  — a release card was dismissed on THIS client
 *   spotlight:<navKey>          — the hint pointing at a sidebar item was dismissed
 *
 * The changelog key is namespaced per client because versions are each
 * component's own (`android/VERSION` vs the web's), independent numbers living in
 * one comparison space: a shared `whatsnew:<version>` let the web's far-higher
 * versions raise this client's baseline above every Android release, hiding the
 * card for good. Legacy un-namespaced keys are still honoured, but only for a
 * version we actually ship (see [whatsNewVersion]) — so the web's numbers, once
 * written under the bare prefix, no longer leak in. Spotlights stay shared: a
 * `navKey` names the same sidebar feature on both clients, so dismissing the
 * arrow on one is meant to settle it on the other.
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

/** This client's changelog namespace — writes and primary reads go here. */
const val WHATSNEW_ANDROID_PREFIX = WHATSNEW_PREFIX + "android:"
const val SPOTLIGHT_PREFIX = "spotlight:"

/**
 * The release version an ack [key] vouches for on Android, or null if it does not
 * count here. This client's own `whatsnew:android:<v>` keys always count; a legacy
 * bare `whatsnew:<v>` counts only for a version we actually ship ([ownVersions]),
 * so the web's independent numbers — written under the same bare prefix before the
 * split — no longer raise our baseline. Any other client's namespace is ignored.
 */
fun whatsNewVersion(key: String, ownVersions: Set<String>): String? {
    if (key.startsWith(WHATSNEW_ANDROID_PREFIX)) return key.removePrefix(WHATSNEW_ANDROID_PREFIX)
    if (!key.startsWith(WHATSNEW_PREFIX)) return null
    val rest = key.removePrefix(WHATSNEW_PREFIX)
    if (rest.contains(':')) return null // another client's namespace, e.g. whatsnew:web:*
    return rest.takeIf { it in ownVersions } // legacy bare key — only for our own releases
}

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
 * The baseline is the highest already-acknowledged version that counts for this
 * client ([whatsNewVersion]) — anything newer than that and not newer than the
 * running build is shown, newest first. A user with no such acks has baseline
 * `0.0.0`, so a first run surfaces the whole curated list once (and clearing the
 * acks makes it show again — testable by hand).
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
    val ownVersions = entries.mapTo(mutableSetOf()) { it.version }
    val ackedVersions = acked.mapNotNull { whatsNewVersion(it, ownVersions) }
    if (ackedVersions.isEmpty() && isBrandNewAccount(accountCreatedAt, buildAtMillis)) {
        return WhatsNewPlan(baseline = WHATSNEW_ANDROID_PREFIX + currentVersion)
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
