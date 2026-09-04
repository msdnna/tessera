package website.msdnna.tessera.util

import com.google.gson.JsonElement
import com.google.gson.JsonObject

// Page geometry of a document, as the web editor stores it (#2821, #2827) — the
// Kotlin half of `frontend/src/utils/docPage.js`, kept to the part a reader
// needs: what the sheet is, and how wide its printable column is relative to it.
//
// Millimetres throughout, because that is the unit the attribute is written in.
// The phone never draws a millimetre: a 210 mm sheet on a 390 dp screen is not a
// sheet, it is a scroll. What survives the trip to a phone is the *proportion* —
// see [docSideInsetDp].

/** The six numbers a geometry consists of, in storage order. */
data class DocPage(
    val w: Double,
    val h: Double,
    val ml: Double,
    val mr: Double,
    val mt: Double,
    val mb: Double,
)

/** A4 with 20 mm margins — what a document nobody opened the page dialog on has. */
val DEFAULT_DOC_PAGE = DocPage(210.0, 297.0, 20.0, 20.0, 20.0, 20.0)

// Bounds shared with the server (office.MinSide/MaxSide) and the web client. A
// value outside them is not a page, it is a misparsed unit.
const val DOC_MIN_SIDE = 50.0
const val DOC_MAX_SIDE = 2000.0

/** A named sheet size, portrait side up. */
data class DocPageSize(val key: String, val w: Double, val h: Double)

val DOC_PAGE_SIZES = listOf(
    DocPageSize("a4", 210.0, 297.0),
    DocPageSize("a5", 148.0, 210.0),
    DocPageSize("a3", 297.0, 420.0),
    DocPageSize("letter", 215.9, 279.4),
    DocPageSize("legal", 215.9, 355.6),
)

/**
 * Whether a value is a usable geometry — the same rules the Go validator
 * (`checkDocPage`) and the web client enforce. Margins that meet leave nothing
 * to print in, so they are not a page either.
 */
fun isDocPage(page: DocPage?): Boolean {
    if (page == null) return false
    val numbers = listOf(page.w, page.h, page.ml, page.mr, page.mt, page.mb)
    if (numbers.any { it.isNaN() || it.isInfinite() }) return false
    if (page.w < DOC_MIN_SIDE || page.w > DOC_MAX_SIDE) return false
    if (page.h < DOC_MIN_SIDE || page.h > DOC_MAX_SIDE) return false
    if (page.ml < 0 || page.mr < 0 || page.mt < 0 || page.mb < 0) return false
    return page.ml + page.mr < page.w && page.mt + page.mb < page.h
}

/** The geometry to lay out with: the given one if usable, the default otherwise. */
fun normalizeDocPage(page: DocPage?): DocPage = if (isDocPage(page)) page!! else DEFAULT_DOC_PAGE

/**
 * Reads `attrs.page` off a node. ProseMirror serialises the unset attribute as
 * `page: null` on every document that has not been through the page dialog, so
 * an absent geometry is the norm rather than an error.
 */
fun docPageAttr(node: JsonElement?): DocPage? {
    val attrs = (node as? JsonObject)?.get("attrs") as? JsonObject ?: return null
    val page = attrs.get("page") as? JsonObject ?: return null
    val read = { key: String ->
        page.get(key)?.takeIf { it.isJsonPrimitive }?.let { runCatching { it.asDouble }.getOrNull() }
    }
    val values = listOf("w", "h", "ml", "mr", "mt", "mb").map { read(it) ?: return null }
    val candidate = DocPage(values[0], values[1], values[2], values[3], values[4], values[5])
    return if (isDocPage(candidate)) candidate else null
}

/** The geometry of the document itself — everything up to the first section break. */
fun parseDocPage(content: JsonElement?): DocPage = normalizeDocPage(docPageAttr(content))

/** Wider than tall. The one question the section caption asks. */
fun isDocLandscape(page: DocPage): Boolean = page.w > page.h

/**
 * Which named size the geometry is, ignoring orientation; '' for one that is not
 * a preset. Compared with a tolerance because an imported A4 is 209.9 mm as
 * often as 210 — twips do not divide evenly into millimetres.
 */
fun docSizeKey(page: DocPage): String {
    val short = minOf(page.w, page.h)
    val long = maxOf(page.w, page.h)
    val near = { a: Double, b: Double -> kotlin.math.abs(a - b) < 0.5 }
    return DOC_PAGE_SIZES.firstOrNull { near(it.w, short) && near(it.h, long) }?.key.orEmpty()
}

/** The printable column in millimetres — the sheet minus its side margins. */
fun docContentWidthMm(page: DocPage): Double = page.w - page.ml - page.mr

/**
 * The geometry every block is laid out in, one entry per block.
 *
 * A document with no section break has one geometry — the one it always had —
 * and every entry is it. A break switches the geometry for everything after it,
 * and counts as the first block of the section it opens: it is where the new
 * geometry starts, and drawing its caption at the old margins would put an
 * «Альбомная, A4» label on a portrait band.
 */
fun docSectionPages(blocks: List<DocBlock>, docPage: DocPage): List<DocPage> {
    var page = docPage
    return blocks.map { block ->
        if (block is DocSectionBreak) page = block.page
        page
    }
}

/**
 * The reader's horizontal inset, in dp, for a document with this geometry.
 *
 * The margin is applied as a *fraction of the sheet*, not as millimetres: a
 * phone is 74 mm of glass, so 20 mm margins taken literally would eat half of
 * the text column and 50 mm ones would leave a ribbon. As a fraction the
 * document's own proportion survives — wide margins still read wider than
 * narrow ones — and the clamp keeps the extreme end usable.
 *
 * Both margins are averaged into one inset because the reader is symmetric: an
 * asymmetric sheet (a bound report with a 30 mm left and 15 mm right margin) is
 * a printing decision, and reproducing it on a phone would look like a layout
 * bug rather than like the document.
 *
 * @param page normalised geometry
 * @param availableDp the width the reader has to draw in
 */
fun docSideInsetDp(page: DocPage, availableDp: Double, min: Double = 12.0, max: Double = 32.0): Double {
    if (availableDp <= 0 || page.w <= 0) return min
    val fraction = ((page.ml + page.mr) / 2.0) / page.w
    return (fraction * availableDp).coerceIn(min, max)
}
