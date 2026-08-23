package website.msdnna.tessera.update

import website.msdnna.tessera.R
import website.msdnna.tessera.util.WhatsNewEntry
import website.msdnna.tessera.util.WhatsNewSpotlight

/**
 * Curated, user-facing "What's New" highlights for the **Android** app — newest
 * first, keyed by the `android/VERSION` they shipped in.
 *
 * Deliberately NOT the raw `android/CHANGELOG.md` (developer-facing and noisy):
 * only what is worth interrupting someone with after an update. A couple of short
 * bullets per release, visible features only, no Markdown — the card renders the
 * strings as-is.
 *
 * The texts live in `res/values` (and `values-en`) under `whatsnew_v<version>_` and
 * arrive here as ids — this list is a top-level `val`, so a ready string would be
 * built once at class load and stay in the language of that moment. A new entry
 * needs its pair of keys in **both** locales (the parity test enforces it).
 *
 * The web keeps its own list (`frontend/src/data/whatsNew.js`) because the two
 * components version independently: the same feature ships under different
 * numbers, and a shared list would show the wrong ones on one of the clients.
 * Add an entry here when an Android release ships something a user should notice
 * (the bump step is the place to remember — see the tessera-ship skill).
 *
 * `spotlight` queues a one-shot arrow at a sidebar item after the card is
 * dismissed; `navKey` must match a sidebar nav key (`activeNav`) — "documents",
 * "milestones", "notes", "reminders", "home", "admin" — or the hint is skipped.
 */
val WhatsNewEntries: List<WhatsNewEntry> = listOf(
    WhatsNewEntry(
        version = "0.70.0",
        date = "2026-08-20",
        titleRes = R.string.whatsnew_v0_70_0_title,
        itemsRes = R.array.whatsnew_v0_70_0_items,
    ),
    WhatsNewEntry(
        version = "0.69.0",
        date = "2026-08-17",
        titleRes = R.string.whatsnew_v0_69_0_title,
        itemsRes = R.array.whatsnew_v0_69_0_items,
        spotlight = WhatsNewSpotlight(
            navKey = "documents",
            titleRes = R.string.whatsnew_spotlight_documents_title,
            bodyRes = R.string.whatsnew_spotlight_documents_body,
        ),
    ),
    WhatsNewEntry(
        version = "0.68.0",
        date = "2026-08-12",
        titleRes = R.string.whatsnew_v0_68_0_title,
        itemsRes = R.array.whatsnew_v0_68_0_items,
    ),
)
