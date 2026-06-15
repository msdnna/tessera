package website.msdnna.tessera.util

import java.util.Locale
import java.util.TimeZone

/** IANA time-zone ids (value == label), sorted. */
fun timezoneOptions(): List<Pair<String, String>> =
    TimeZone.getAvailableIDs().toSortedSet().map { it to it }

/** ISO 3166-1 alpha-2 country codes → localized name, sorted by name. */
fun countryOptions(language: String = "ru"): List<Pair<String, String>> {
    val loc = Locale(language)
    return Locale.getISOCountries()
        .map { code -> code to Locale("", code).getDisplayCountry(loc).ifBlank { code } }
        .sortedBy { it.second.lowercase(loc) }
}
