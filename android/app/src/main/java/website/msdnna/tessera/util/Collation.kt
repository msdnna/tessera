package website.msdnna.tessera.util

import java.math.BigInteger
import java.text.Collator
import java.util.Locale

/*
 * Ordering of user-entered text — tag names, namespace labels, member names.
 *
 * The web sorts all of these with one collator (`frontend/src/utils/format.js`
 * `collator()`), built as `Intl.Collator(lang, { numeric: true, sensitivity: 'base' })`.
 * Android mirrors those lists but reached for `java.text.Collator` directly, and
 * that class has no notion of `numeric` at all: it compares "10" against "2" letter
 * by letter, so a board grouped by the «спринт» namespace put «спринт/10» before
 * «спринт/2» on the phone and after it in the browser. The web comment that
 * introduced the flag names this exact case, so the two were never meant to differ.
 *
 * Two call sites had no collator whatsoever and sorted by `String.lowercase()`,
 * i.e. by UTF-16 code unit. That fails on Cyrillic «ё» (U+0451), which sits past
 * «я» (U+044F): a tag namespace «ёмкость» or a colleague named «Ёлкина» dropped to
 * the very bottom of a list that is otherwise alphabetical.
 */

/**
 * Locale for ordering user data. Deliberately not the interface language: tag and
 * people names stay Russian on an English UI, and the order of a list must not
 * reshuffle when the language toggle is flipped (see [buildTagGroups]).
 */
private val COLLATION_LOCALE: Locale = Locale.forLanguageTag("ru")

/**
 * The comparator every list of user-entered text is sorted with. Case-insensitive
 * (web `sensitivity: 'base'`) and digit-aware (web `numeric: true`).
 */
val UserTextOrder: Comparator<String> = naturalCollator(COLLATION_LOCALE)

/**
 * A collator for [locale] wrapped so that runs of digits compare by value rather
 * than character by character — «этап 2» before «этап 10».
 */
fun naturalCollator(
    locale: Locale = COLLATION_LOCALE,
    strength: Int = Collator.SECONDARY,
): Comparator<String> {
    val collator = Collator.getInstance(locale).apply { this.strength = strength }
    return Comparator { a, b -> compareNatural(collator, a, b) }
}

/** ASCII digits only: `Char.isDigit()` is true for Arabic-Indic digits too, and
 *  [BigInteger] cannot parse those — a tag written in them would crash the sort. */
private fun Char.isAsciiDigit(): Boolean = this in '0'..'9'

/** Splits a string into alternating digit / non-digit runs ("этап 10" → ["этап ", "10"]). */
private fun runs(s: String): List<String> {
    val out = ArrayList<String>()
    var i = 0
    while (i < s.length) {
        val digit = s[i].isAsciiDigit()
        var j = i + 1
        while (j < s.length && s[j].isAsciiDigit() == digit) j++
        out.add(s.substring(i, j))
        i = j
    }
    return out
}

private fun compareNatural(collator: Collator, a: String, b: String): Int {
    val ra = runs(a)
    val rb = runs(b)
    for (k in 0 until minOf(ra.size, rb.size)) {
        val x = ra[k]
        val y = rb[k]
        val c = if (x[0].isAsciiDigit() && y[0].isAsciiDigit()) {
            BigInteger(x).compareTo(BigInteger(y))
        } else {
            collator.compare(x, y)
        }
        if (c != 0) return c
    }
    // Every shared run matched: either one string is a prefix of the other, or the
    // only difference is leading zeros ("этап 02" vs "этап 2"). The plain collator
    // settles both, and keeps the order total so a sort stays deterministic.
    return collator.compare(a, b)
}
