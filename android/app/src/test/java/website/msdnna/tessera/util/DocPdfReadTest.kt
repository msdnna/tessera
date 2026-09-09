package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Locale
import org.junit.Test

/**
 * The arithmetic of reading a PDF in place (#2733, §3 of #2894) — the part
 * `PdfRenderer` cannot be asked about in a unit test, and the part that decides
 * how much memory a page costs.
 */
class DocPdfReadTest {
    @Test
    fun `a byte count is scaled and punctuated in the reader's language`() {
        assertThat(docFileSize(512, Locale.forLanguageTag("ru")))
            .isEqualTo(DocFileSize("512", DocSizeUnit.BYTES))
        // The decimal separator is the locale's: writing the comma by hand is
        // right for Russian and wrong everywhere the point stays a point.
        assertThat(docFileSize(1536, Locale.forLanguageTag("ru"))?.text).isEqualTo("1,5")
        assertThat(docFileSize(1536, Locale.ENGLISH)?.text).isEqualTo("1.5")
        assertThat(docFileSize(12L * 1024 * 1024, Locale.ENGLISH))
            .isEqualTo(DocFileSize("12", DocSizeUnit.MB))
        assertThat(docFileSize(3L * 1024 * 1024 * 1024, Locale.ENGLISH)?.unit)
            .isEqualTo(DocSizeUnit.GB)
    }

    @Test
    fun `a size the block does not carry draws nothing`() {
        // «0 Б» next to a file name reads as a broken upload.
        assertThat(docFileSize(0)).isNull()
        assertThat(docFileSize(-1)).isNull()
    }

    @Test
    fun `a page number stays inside the document`() {
        assertThat(clampPdfPage(0, 10)).isEqualTo(1)
        assertThat(clampPdfPage(11, 10)).isEqualTo(10)
        assertThat(clampPdfPage(4, 10)).isEqualTo(4)
        // Page 0 of 0 is the kind of thing that gets shipped.
        assertThat(clampPdfPage(1, 0)).isEqualTo(1)
        assertThat(clampPdfPage(-3, 0)).isEqualTo(1)
    }

    @Test
    fun `a page is rendered at the width it is drawn at`() {
        // 595x842 pt is A4; at 1080 px wide the bitmap keeps the ratio.
        val (w, h) = pdfBitmapSize(595, 842, 1080)!!
        assertThat(w).isEqualTo(1080)
        assertThat(h).isEqualTo(1528)
    }

    @Test
    fun `an oversized page is capped instead of dying with an OOM`() {
        // PdfRenderer allocates the whole bitmap up front, and an A0 poster at
        // screen density is hundreds of megabytes.
        val (w, h) = pdfBitmapSize(2384, 3370, 8000, maxPx = 2400)!!
        assertThat(w).isAtMost(2400)
        assertThat(h).isAtMost(2400)
        // The long side is the one that hits the cap, and the ratio survives it.
        assertThat(h).isEqualTo(2400)
        assertThat(w).isEqualTo(1698)
    }

    @Test
    fun `a landscape page is capped on its width, which is the long side`() {
        // The portrait case above is capped by the height alone, so it would pass
        // even with the width left unbounded. A landscape scan is the one that
        // actually asks the container width to be clamped.
        val (w, h) = pdfBitmapSize(3370, 2384, 8000, maxPx = 2400)!!
        assertThat(w).isEqualTo(2400)
        assertThat(h).isEqualTo(1698)
    }

    @Test
    fun `a page with no usable size renders nothing rather than a zero bitmap`() {
        // Bitmap.createBitmap throws on a zero side, and a page that has not been
        // measured yet reports one.
        assertThat(pdfBitmapSize(0, 842, 1080)).isNull()
        assertThat(pdfBitmapSize(595, 0, 1080)).isNull()
        assertThat(pdfBitmapSize(595, 842, 0)).isNull()
    }
}
