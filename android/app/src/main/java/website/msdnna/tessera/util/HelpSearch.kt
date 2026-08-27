package website.msdnna.tessera.util

import java.text.Collator
import java.util.Locale
import website.msdnna.tessera.data.model.HelpContent

/**
 * Search over the help index — a port of the web `utils/helpSearch.js` (#2795).
 *
 * The corpus is a handful of short articles shipped inside the APK, so this runs
 * entirely on the device: no endpoint, works offline. Prefix matching is what
 * makes it usable — «доск» has to find «Доски и задачи», because a reader types
 * the stem and stops.
 *
 * Ranking is field-weighted, not frequency-based: in a manual, a word in the
 * title says far more about relevance than the same word buried in a paragraph.
 *
 * Kept in step with the web deliberately, tokenizer included: a word that finds
 * an article in the browser and finds nothing in the app reads as a broken
 * search, not as two implementations.
 */
private object Weight {
    const val TITLE = 8
    const val KEYWORDS = 4
    const val HEADING = 3
    const val TEXT = 1
}

/** A token is a run of letters/digits — Unicode-aware, so Cyrillic is not split
 *  apart and «e2e» stays one token. Hyphenated words break into parts on
 *  purpose: searching «код» should reach «QR-код». */
private val TOKEN_RE = Regex("[\\p{L}\\p{N}]+")

fun tokenizeHelp(s: String?): List<String> =
    TOKEN_RE.findAll((s ?: "").lowercase()).map { it.value }.toList()

/** One search result: the article, its score, and the fragment that matched. */
data class HelpHit(
    val slug: String,
    val score: Int,
    val title: String,
    val category: String,
    val excerpt: String,
)

/**
 * Precomputes token → {slug → score} once per index; [search] is what the UI
 * calls on every keystroke. The articles are already collapsed to one language
 * and platform ([HelpContent]), so the corpus, the titles and the [collator]
 * that orders ties all belong to the reader's language (#2809).
 */
class HelpSearcher(articles: List<HelpContent>, lang: String = "ru") {
    private val postings = HashMap<String, MutableMap<String, Int>>()
    private val bySlug = LinkedHashMap<String, HelpContent>()
    private val tokens: List<String>
    private val collator: Collator = Collator.getInstance(Locale.forLanguageTag(lang))

    init {
        for (a in articles) {
            bySlug[a.slug] = a
            for (t in tokenizeHelp(a.title)) add(t, a.slug, Weight.TITLE)
            // Indexed from the text this client renders in this language: a word
            // findable in the browser must be findable in the app, and one the
            // reader never sees on screen must not be.
            for (kw in a.keywords) for (t in tokenizeHelp(kw)) add(t, a.slug, Weight.KEYWORDS)
            for (h in a.headings) for (t in tokenizeHelp(h.text)) add(t, a.slug, Weight.HEADING)
            // The body is scored once per distinct token: repeating a word ten
            // times in one article should not outrank an article that is
            // actually about it.
            for (t in tokenizeHelp(a.text).toSet()) add(t, a.slug, Weight.TEXT)
        }
        // Sorted once so a prefix lookup scans a contiguous range instead of
        // testing every token in the corpus on each keystroke.
        tokens = postings.keys.sorted()
    }

    private fun add(token: String, slug: String, weight: Int) {
        val entry = postings.getOrPut(token) { HashMap() }
        entry[slug] = (entry[slug] ?: 0) + weight
    }

    /** Merged postings of every token starting with [prefix] — an exact hit
     *  counts double, so «доска» beats «досках» when both are present. */
    private fun matchPrefix(prefix: String): Map<String, Int> {
        val hits = HashMap<String, Int>()
        var lo = 0
        var hi = tokens.size
        while (lo < hi) {
            val mid = (lo + hi) ushr 1
            if (tokens[mid] < prefix) lo = mid + 1 else hi = mid
        }
        var i = lo
        while (i < tokens.size && tokens[i].startsWith(prefix)) {
            val exact = if (tokens[i] == prefix) 2 else 1
            for ((slug, score) in postings.getValue(tokens[i])) {
                hits[slug] = maxOf(hits[slug] ?: 0, score * exact)
            }
            i++
        }
        return hits
    }

    fun search(query: String?, limit: Int = DEFAULT_LIMIT): List<HelpHit> {
        val terms = tokenizeHelp(query)
        if (terms.isEmpty()) return emptyList()
        // Every term must match somewhere in the article (AND): with a corpus
        // this small, OR-matching turns a two-word query into "everything,
        // ranked".
        var acc: Map<String, Int>? = null
        for (term in terms) {
            val hits = matchPrefix(term)
            if (hits.isEmpty()) return emptyList()
            val previous = acc
            if (previous == null) {
                acc = hits
                continue
            }
            val next = HashMap<String, Int>()
            for ((slug, score) in hits) {
                val before = previous[slug] ?: continue
                next[slug] = before + score
            }
            if (next.isEmpty()) return emptyList()
            acc = next
        }
        val matched = acc ?: return emptyList()
        return matched.mapNotNull { (slug, score) ->
            val a = bySlug[slug] ?: return@mapNotNull null
            HelpHit(slug, score, a.title, a.category, helpExcerpt(a.text, terms))
        }.sortedWith(
            compareByDescending<HelpHit> { it.score }.thenComparator { x, y ->
                collator.compare(x.title, y.title)
            },
        ).take(limit)
    }

    private companion object {
        const val DEFAULT_LIMIT = 20
    }
}

private const val EXCERPT_LEAD = 40
private const val EXCERPT_LEN = 160

/**
 * Lifts the neighbourhood of the first matching term out of the flattened body,
 * so a result row shows *why* it matched. Returns the opening of the article
 * when the match is only in the title or keywords.
 */
fun helpExcerpt(text: String?, terms: List<String>): String {
    val body = text ?: ""
    if (body.isEmpty()) return ""
    var at = -1
    for (term in terms) {
        val found = body.indexOf(term)
        if (found >= 0 && (at < 0 || found < at)) at = found
    }
    if (at < 0) {
        return body.take(EXCERPT_LEN).trim() + if (body.length > EXCERPT_LEN) "…" else ""
    }
    // Back up to a word boundary so the snippet does not start mid-word.
    var start = maxOf(0, at - EXCERPT_LEAD)
    if (start > 0) {
        val space = body.indexOf(' ', start)
        if (space in 0 until at) start = space + 1
    }
    val end = minOf(body.length, start + EXCERPT_LEN)
    return (if (start > 0) "…" else "") + body.substring(start, end).trim() + if (end < body.length) "…" else ""
}
