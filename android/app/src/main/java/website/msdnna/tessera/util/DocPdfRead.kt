package website.msdnna.tessera.util

import java.text.NumberFormat
import java.util.Locale

// Reading a PDF that lives inside a document (#2733) — the arithmetic half.
//
// Unlike every other import format a PDF is not converted into blocks: run one
// through LibreOffice and you get a page of absolutely positioned text frames
// with no paragraphs and no reading order. So the file is stored as a document
// asset and the body holds one `pdfEmbed` block pointing at it.
//
// The web client draws it with pdf.js; the phone has `android.graphics.pdf`,
// which renders a page into a bitmap and nothing else — no page window, no
// scale, no page counter. Those decisions are here, away from PdfRenderer,
// because they are the part that can be wrong in a way tests can see.

/** Which unit a byte count was scaled into — the reader translates it. */
enum class DocSizeUnit { BYTES, KB, MB, GB }

/** A byte count as a number in the reader's language plus the unit to label it. */
data class DocFileSize(val text: String, val unit: DocSizeUnit)

private const val STEP = 1024.0

/**
 * Human-readable byte count.
 *
 * Both halves are locale business: the unit comes from the string catalogue and
 * the decimal separator from [NumberFormat] — writing the comma by hand is right
 * for Russian and wrong everywhere the point stays a point. Whole bytes never
 * want a fraction digit; anything scaled reads better with one.
 *
 * @return null for a size the block does not carry (0 or absent), so the caller
 *   draws no size at all rather than «0 Б»
 */
fun docFileSize(bytes: Long, locale: Locale = Locale.getDefault()): DocFileSize? {
    if (bytes <= 0) return null
    var value = bytes.toDouble()
    var unit = 0
    val units = DocSizeUnit.entries
    while (value >= STEP && unit < units.size - 1) {
        value /= STEP
        unit += 1
    }
    val format = NumberFormat.getInstance(locale).apply {
        maximumFractionDigits = if (unit == 0) 0 else 1
    }
    return DocFileSize(format.format(value), units[unit])
}

/**
 * Keeps a page number inside the document.
 *
 * Returns 1 for an empty or unknown document rather than 0: the viewer shows
 * «стр. N из M», and page 0 of 0 is the kind of thing that gets shipped.
 */
fun clampPdfPage(page: Int, total: Int): Int {
    val count = maxOf(1, total)
    return page.coerceIn(1, count)
}

/**
 * The bitmap to render one page into, in pixels.
 *
 * Rendered at the width it is drawn at rather than at the page's own size: a
 * PDF page is 595 pt wide and a phone is over a thousand pixels, so rendering at
 * source size and letting the ImageView scale up is how a scan turns to mush.
 *
 * The cap is not decoration. `PdfRenderer` allocates the whole bitmap up front —
 * an A0 poster at screen density is hundreds of megabytes and dies with an OOM
 * that takes the app with it, so the long side is bounded and the page is drawn
 * slightly softer instead.
 *
 * @param pageWidth page width in PDF points
 * @param pageHeight page height in PDF points
 * @param widthPx the width the page is drawn at
 * @param maxPx bound on either side of the bitmap
 * @return width to height in pixels, or null when the page has no usable size
 */
fun pdfBitmapSize(
    pageWidth: Int,
    pageHeight: Int,
    widthPx: Int,
    maxPx: Int = MAX_PDF_BITMAP_PX,
): Pair<Int, Int>? {
    if (pageWidth <= 0 || pageHeight <= 0 || widthPx <= 0 || maxPx <= 0) return null
    val ratio = pageHeight.toDouble() / pageWidth.toDouble()
    var w = minOf(widthPx, maxPx)
    var h = maxOf(1, Math.round(w * ratio).toInt())
    if (h > maxPx) {
        h = maxPx
        w = maxOf(1, Math.round(h / ratio).toInt())
    }
    return w to h
}

/** Bound on a rendered page's long side — see [pdfBitmapSize]. */
const val MAX_PDF_BITMAP_PX = 2400
