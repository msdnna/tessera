package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonParser
import org.junit.Test

/**
 * Page geometry as the reader sees it (#2821, #2827, §3 of #2894).
 *
 * The rules here are shared with the server (`checkDocPage`) and the web client
 * (`docPage.js`), and a reader that disagrees with either draws a document its
 * author never wrote — so they are pinned rather than trusted.
 */
class DocPageTest {
    private fun page(json: String) = parseDocPage(JsonParser.parseString(json))

    @Test
    fun `a document without a geometry gets the default A4 sheet`() {
        // ProseMirror serialises the unset attribute as `page: null` on every
        // document that has not been through the page dialog, so this is the
        // normal case rather than an error one.
        assertThat(parseDocPage(null)).isEqualTo(DEFAULT_DOC_PAGE)
        assertThat(page("""{"type":"doc","content":[]}""")).isEqualTo(DEFAULT_DOC_PAGE)
        assertThat(page("""{"type":"doc","attrs":{"page":null}}""")).isEqualTo(DEFAULT_DOC_PAGE)
    }

    @Test
    fun `a stored geometry is read whole`() {
        val landscape = page(
            """{"type":"doc","attrs":{"page":{"w":297,"h":210,"ml":13,"mr":13,"mt":25,"mb":25}}}""",
        )
        assertThat(landscape).isEqualTo(DocPage(297.0, 210.0, 13.0, 13.0, 25.0, 25.0))
        assertThat(isDocLandscape(landscape)).isTrue()
        assertThat(docSizeKey(landscape)).isEqualTo("a4")
        assertThat(docContentWidthMm(landscape)).isEqualTo(271.0)
    }

    @Test
    fun `an unusable geometry falls back instead of reaching the layout`() {
        // A side outside the bounds is a misparsed unit (inches read as
        // millimetres), and margins that meet leave nothing to draw in.
        val cases = listOf(
            """{"w":10,"h":297,"ml":20,"mr":20,"mt":20,"mb":20}""",
            """{"w":5000,"h":297,"ml":20,"mr":20,"mt":20,"mb":20}""",
            """{"w":210,"h":297,"ml":120,"mr":120,"mt":20,"mb":20}""",
            """{"w":210,"h":297,"ml":-5,"mr":20,"mt":20,"mb":20}""",
            // Missing member: a partial object is not a geometry.
            """{"w":210,"h":297,"ml":20,"mr":20,"mt":20}""",
            """{"w":"210","h":"297","ml":20,"mr":20,"mt":20,"mb":20}""",
        )
        for (case in cases) {
            assertThat(page("""{"type":"doc","attrs":{"page":$case}}"""))
                .isEqualTo(DEFAULT_DOC_PAGE)
        }
    }

    @Test
    fun `an imported sheet is recognised despite twip rounding`() {
        // Twips do not divide evenly into millimetres, so an imported A4 is
        // 209.9 mm as often as 210 — matching exactly would label it «свой размер».
        assertThat(docSizeKey(DocPage(209.9, 297.05, 20.0, 20.0, 20.0, 20.0))).isEqualTo("a4")
        assertThat(docSizeKey(DocPage(215.9, 279.4, 20.0, 20.0, 20.0, 20.0))).isEqualTo("letter")
        assertThat(docSizeKey(DocPage(180.0, 240.0, 20.0, 20.0, 20.0, 20.0))).isEmpty()
    }

    @Test
    fun `section breaks switch the geometry for what follows them`() {
        val blocks = parseDocBlocks(
            JsonParser.parseString(
                """
                {"type":"doc","attrs":{"page":{"w":210,"h":297,"ml":20,"mr":20,"mt":20,"mb":20}},
                 "content":[
                  {"type":"paragraph","attrs":{"id":"a"},"content":[{"type":"text","text":"до"}]},
                  {"type":"sectionBreak","attrs":{"id":"br",
                    "page":{"w":297,"h":210,"ml":13,"mr":13,"mt":13,"mb":13}}},
                  {"type":"paragraph","attrs":{"id":"b"},"content":[{"type":"text","text":"после"}]}
                 ]}
                """.trimIndent(),
            ),
        )
        val pages = docSectionPages(blocks, DocPage(210.0, 297.0, 20.0, 20.0, 20.0, 20.0))
        assertThat(pages).hasSize(3)
        assertThat(pages[0].w).isEqualTo(210.0)
        // The break itself belongs to the section it opens: its caption says what
        // comes next, and drawing it at the old width would label the wrong band.
        assertThat(pages[1].w).isEqualTo(297.0)
        assertThat(pages[2].w).isEqualTo(297.0)
    }

    @Test
    fun `margins reach the phone as a share of the sheet, clamped`() {
        val available = 390.0
        val narrow = DocPage(210.0, 297.0, 13.0, 13.0, 13.0, 13.0)
        val normal = DocPage(210.0, 297.0, 25.0, 25.0, 25.0, 25.0)
        val wide = DocPage(210.0, 297.0, 50.0, 50.0, 25.0, 25.0)

        // 20 mm on a 74 mm phone would be half the text column; as a fraction the
        // document's own proportion survives.
        assertThat(docSideInsetDp(narrow, available)).isWithin(0.01).of(24.14)
        assertThat(docSideInsetDp(normal, available)).isWithin(0.01).of(32.0)
        // Wide margins clamp rather than leaving a ribbon of text.
        assertThat(docSideInsetDp(wide, available)).isEqualTo(32.0)
        // …and a sheet with almost none still keeps the reader off the bezel.
        assertThat(docSideInsetDp(DocPage(210.0, 297.0, 0.0, 0.0, 0.0, 0.0), available)).isEqualTo(12.0)
        // A width that has not been measured yet must not divide by zero.
        assertThat(docSideInsetDp(normal, 0.0)).isEqualTo(12.0)
    }
}
