package website.msdnna.tessera.util

import java.util.Locale
import java.util.TimeZone

/** IANA time-zone ids (value == label), sorted. */
fun timezoneOptions(): List<Pair<String, String>> =
    TimeZone.getAvailableIDs().toSortedSet().map { it to it }

/**
 * ISO 3166-1 alpha-2 country codes → localized name, sorted by name.
 *
 * Sorted with a collator, not by `lowercase()`: a code-unit sort is not the
 * alphabet anywhere the names are not plain ASCII (see [naturalCollator]). In
 * Russian it puts «Тёркс и Кайкос» past the whole of «Т», between «Турция» and
 * «Уганда» — «ё» is U+0451, past «я» — and files every «о-в …»/«о-ва …» ahead of
 * «ОАЭ», because a hyphen precedes any letter by code unit. 22 of the 249 rows
 * land in a different place. The web sorts the same list with
 * `Intl.Collator` (`utils/localeOptions.js`), so the phone's picker and the
 * settings page in the browser disagreed about where to scroll for a country.
 *
 * The collator is built for the *interface* language, like the web's: unlike tag
 * or people names ([UserTextOrder]), these strings are catalogue text that is
 * already re-rendered in that language, so ordering them by it is not a reshuffle
 * of user data.
 */
fun countryOptions(language: String = "ru"): List<Pair<String, String>> {
    val loc = Locale(language)
    return Locale.getISOCountries()
        .map { code -> code to Locale("", code).getDisplayCountry(loc).ifBlank { code } }
        .sortedWith(compareBy(naturalCollator(loc)) { it.second })
}
