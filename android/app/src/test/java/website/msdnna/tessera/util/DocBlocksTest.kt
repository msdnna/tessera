package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonParser
import org.junit.Test

/**
 * The document reader's whole correctness budget lives here: [parseDocBlocks] is
 * the only non-Compose part of #2735, and the rendering it feeds is a direct
 * function of what it returns.
 */
class DocBlocksTest {
    private fun parse(json: String) = parseDocBlocks(JsonParser.parseString(json))

    @Test
    fun `empty and malformed bodies yield no blocks`() {
        assertThat(parseDocBlocks(null)).isEmpty()
        assertThat(parse("""{"type":"doc","content":[]}""")).isEmpty()
        assertThat(parse("""{"type":"doc"}""")).isEmpty()
        // A body that is not an object at all (a stray array or string) must not
        // throw — the reader would otherwise die on one bad document.
        assertThat(parse("""[]""")).isEmpty()
        assertThat(parse(""""nope"""")).isEmpty()
    }

    @Test
    fun `paragraph keeps its block id, alignment and indent`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"paragraph","attrs":{"id":"blk-1","textAlign":"center","indent":2},
               "content":[{"type":"text","text":"привет"}]}
            ]}
            """.trimIndent(),
        )
        val p = blocks.single() as DocParagraph
        assertThat(p.id).isEqualTo("blk-1")
        assertThat(p.align).isEqualTo("center")
        assertThat(p.indent).isEqualTo(2)
        assertThat(p.spans.single().text).isEqualTo("привет")
    }

    @Test
    fun `blocks without an id still get unique keys`() {
        // Documents written before BlockId existed carry no attrs.id. A shared
        // key would collapse them into one row in a LazyColumn.
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"paragraph","content":[{"type":"text","text":"a"}]},
              {"type":"paragraph","content":[{"type":"text","text":"b"}]}
            ]}
            """.trimIndent(),
        )
        assertThat(blocks.map { it.id }.toSet()).hasSize(2)
    }

    @Test
    fun `heading level is read and clamped`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"H"}]},
              {"type":"heading","attrs":{"level":99},"content":[{"type":"text","text":"X"}]},
              {"type":"heading","content":[{"type":"text","text":"Y"}]}
            ]}
            """.trimIndent(),
        )
        assertThat(blocks.map { (it as DocHeading).level }).containsExactly(2, 6, 1).inOrder()
    }

    @Test
    fun `marks land on their span and adjacent identical runs merge`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"paragraph","content":[
              {"type":"text","text":"bold","marks":[{"type":"bold"}]},
              {"type":"text","text":"er","marks":[{"type":"bold"}]},
              {"type":"text","text":" plain"},
              {"type":"text","text":"link","marks":[{"type":"link","attrs":{"href":"https://x.test"}}]}
            ]}]}
            """.trimIndent(),
        )
        val spans = (blocks.single() as DocParagraph).spans
        assertThat(spans).hasSize(3)
        assertThat(spans[0]).isEqualTo(DocSpan("bolder", bold = true))
        assertThat(spans[1].text).isEqualTo(" plain")
        assertThat(spans[2].href).isEqualTo("https://x.test")
    }

    @Test
    fun `every supported mark is carried`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"paragraph","content":[
              {"type":"text","text":"x","marks":[
                {"type":"italic"},{"type":"underline"},{"type":"strike"},{"type":"code"},
                {"type":"textStyle","attrs":{"color":"#ff0000"}},{"type":"unknownMark"}
              ]}
            ]}]}
            """.trimIndent(),
        )
        val span = (blocks.single() as DocParagraph).spans.single()
        assertThat(span.italic).isTrue()
        assertThat(span.underline).isTrue()
        assertThat(span.strike).isTrue()
        assertThat(span.code).isTrue()
        assertThat(span.color).isEqualTo("#ff0000")
    }

    @Test
    fun `hard break becomes a newline span`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"paragraph","content":[
              {"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}
            ]}]}
            """.trimIndent(),
        )
        assertThat((blocks.single() as DocParagraph).spans.joinToString("") { it.text }).isEqualTo("a\nb")
    }

    @Test
    fun `ordered list numbering honours start and restarts inside a nested list`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"orderedList","attrs":{"start":3},"content":[
              {"type":"listItem","content":[
                {"type":"paragraph","content":[{"type":"text","text":"first"}]},
                {"type":"orderedList","content":[
                  {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"nested"}]}]}
                ]}
              ]},
              {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"second"}]}]}
            ]}]}
            """.trimIndent(),
        )
        val rows = blocks.filterIsInstance<DocListRow>()
        assertThat(rows.map { it.marker }).containsExactly("3.", "1.", "4.").inOrder()
        assertThat(rows.map { it.depth }).containsExactly(0, 1, 0).inOrder()
        assertThat(rows.map { it.spans.joinToString("") { s -> s.text } })
            .containsExactly("first", "nested", "second").inOrder()
    }

    @Test
    fun `bullet and task lists carry their own markers`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"bulletList","content":[
                {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}
              ]},
              {"type":"taskList","content":[
                {"type":"taskItem","attrs":{"checked":true},
                 "content":[{"type":"paragraph","content":[{"type":"text","text":"done"}]}]},
                {"type":"taskItem","content":[{"type":"paragraph","content":[{"type":"text","text":"todo"}]}]}
              ]}
            ]}
            """.trimIndent(),
        )
        val rows = blocks.filterIsInstance<DocListRow>()
        assertThat(rows[0].marker).isEqualTo("•")
        assertThat(rows[0].checked).isNull()
        assertThat(rows[1].checked).isTrue()
        // An unchecked task item omits the attribute entirely; it must read as
        // false, not as "this is a bullet".
        assertThat(rows[2].checked).isFalse()
        assertThat(rows[2].marker).isEmpty()
    }

    @Test
    fun `code block keeps its text verbatim and its language`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"kotlin"},
              "content":[{"type":"text","text":"fun main() {\n    println(1)\n}"}]}]}
            """.trimIndent(),
        )
        val code = blocks.single() as DocCode
        assertThat(code.language).isEqualTo("kotlin")
        assertThat(code.text).isEqualTo("fun main() {\n    println(1)\n}")
    }

    @Test
    fun `blockquote splits into one row per child paragraph`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"blockquote","content":[
              {"type":"paragraph","content":[{"type":"text","text":"one"}]},
              {"type":"paragraph","content":[{"type":"text","text":"two"}]}
            ]}]}
            """.trimIndent(),
        )
        val quotes = blocks.filterIsInstance<DocQuote>()
        assertThat(quotes.map { it.spans.joinToString("") { s -> s.text } })
            .containsExactly("one", "two").inOrder()
        assertThat(quotes.map { it.id }.toSet()).hasSize(2)
    }

    @Test
    fun `image and divider survive`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"image","attrs":{"src":"/api/documents/asset?doc=1","alt":"схема"}},
              {"type":"horizontalRule"}
            ]}
            """.trimIndent(),
        )
        val image = blocks[0] as DocImage
        assertThat(image.src).isEqualTo("/api/documents/asset?doc=1")
        assertThat(image.alt).isEqualTo("схема")
        assertThat(blocks[1]).isInstanceOf(DocDivider::class.java)
    }

    @Test
    fun `table rows keep header cells apart from body cells`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"table","content":[
              {"type":"tableRow","content":[
                {"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"К"}]}]}
              ]},
              {"type":"tableRow","content":[
                {"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"в"}]}]}
              ]}
            ]}]}
            """.trimIndent(),
        )
        val table = blocks.single() as DocTable
        assertThat(table.rows).hasSize(2)
        assertThat(table.rows[0].cells.single().header).isTrue()
        assertThat(table.rows[1].cells.single().header).isFalse()
        assertThat(table.rows[1].cells.single().spans.single().text).isEqualTo("в")
    }

    @Test
    fun `an unknown node contributes its children instead of swallowing them`() {
        // The phone ships behind the web client, so it will meet node types it
        // predates. Dropping the subtree with the node would silently delete
        // text the user wrote.
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"callout","content":[
              {"type":"paragraph","content":[{"type":"text","text":"внутри"}]}
            ]}]}
            """.trimIndent(),
        )
        assertThat((blocks.single() as DocParagraph).spans.single().text).isEqualTo("внутри")
    }

    @Test
    fun `docPlainText flattens the body`() {
        val text = docPlainText(
            JsonParser.parseString(
                """
                {"type":"doc","content":[
                  {"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Заголовок"}]},
                  {"type":"paragraph","content":[{"type":"text","text":"текст"}]}
                ]}
                """.trimIndent(),
            ),
        )
        assertThat(text).isEqualTo("Заголовок\nтекст")
    }

    // ── §3 of #2894: the blocks and attributes the reader used to drop ────────

    @Test
    fun `a PDF block survives instead of leaving an empty document`() {
        // An imported PDF is a document whose whole body is one pdfEmbed node.
        // Skipping it (an atom has no children to fall back on) rendered the
        // document as blank — the reader said «Документ пуст» about a 12 MB scan.
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"pdfEmbed","attrs":{
              "id":"pdf-1","src":"/api/documents/asset?doc=1&n=a.pdf&sig=x",
              "name":"Договор.pdf","size":1258291}}]}
            """.trimIndent(),
        )
        val pdf = blocks.single() as DocPdf
        assertThat(pdf.id).isEqualTo("pdf-1")
        assertThat(pdf.src).isEqualTo("/api/documents/asset?doc=1&n=a.pdf&sig=x")
        assertThat(pdf.name).isEqualTo("Договор.pdf")
        assertThat(pdf.size).isEqualTo(1258291L)
    }

    @Test
    fun `a section break keeps its geometry, and keeps the boundary without one`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"sectionBreak","attrs":{"id":"br-1",
                "page":{"w":297,"h":210,"ml":13,"mr":13,"mt":13,"mb":13}}},
              {"type":"sectionBreak","attrs":{"id":"br-2"}}
            ]}
            """.trimIndent(),
        )
        val first = blocks[0] as DocSectionBreak
        assertThat(first.page).isEqualTo(DocPage(297.0, 210.0, 13.0, 13.0, 13.0, 13.0))
        // A break whose geometry did not survive a paste is still a break:
        // losing it would silently merge two sections of the document.
        assertThat((blocks[1] as DocSectionBreak).page).isEqualTo(DEFAULT_DOC_PAGE)
    }

    @Test
    fun `textStyle carries the font and size beside the colour`() {
        val blocks = parse(
            """
            {"type":"doc","content":[{"type":"paragraph","content":[
              {"type":"text","text":"крупно","marks":[{"type":"textStyle","attrs":{
                "fontSize":"24px","fontFamily":"Georgia, \"Times New Roman\", serif","color":"#1f4e79"}}]}
            ]}]}
            """.trimIndent(),
        )
        val span = (blocks.single() as DocParagraph).spans.single()
        assertThat(span.fontSize).isEqualTo("24px")
        assertThat(docFontSizeSp(span.fontSize)).isEqualTo(24f)
        assertThat(docFontKind(span.fontFamily)).isEqualTo(DocFontKind.SERIF)
        assertThat(span.color).isEqualTo("#1f4e79")
    }

    @Test
    fun `line spacing and indentation reach every styled block`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"paragraph","attrs":{"id":"p","lineHeight":"1.5","indent":2},
               "content":[{"type":"text","text":"а"}]},
              {"type":"heading","attrs":{"id":"h","level":2,"lineHeight":"2","indent":1},
               "content":[{"type":"text","text":"б"}]},
              {"type":"blockquote","attrs":{"id":"q","lineHeight":"1.15","indent":3},
               "content":[{"type":"paragraph","content":[{"type":"text","text":"в"}]}]}
            ]}
            """.trimIndent(),
        )
        assertThat((blocks[0] as DocParagraph).lineHeight).isEqualTo(1.5f)
        assertThat((blocks[1] as DocHeading).lineHeight).isEqualTo(2f)
        assertThat((blocks[1] as DocHeading).indent).isEqualTo(1)
        // Spacing is an attribute of the quote, not of the paragraphs inside it,
        // so every row it flattens into carries the quote's own values.
        assertThat((blocks[2] as DocQuote).lineHeight).isEqualTo(1.15f)
        assertThat((blocks[2] as DocQuote).indent).isEqualTo(3)
    }

    @Test
    fun `a spacing or size that is not a multiplier is dropped, not guessed at`() {
        val blocks = parse(
            """
            {"type":"doc","content":[
              {"type":"paragraph","attrs":{"id":"a","lineHeight":"20px"},"content":[{"type":"text","text":"а"}]},
              {"type":"paragraph","attrs":{"id":"b","lineHeight":"40"},"content":[{"type":"text","text":"б"}]}
            ]}
            """.trimIndent(),
        )
        // 20px read as a multiplier would scroll one paragraph off the screen.
        assertThat((blocks[0] as DocParagraph).lineHeight).isNull()
        assertThat((blocks[1] as DocParagraph).lineHeight).isNull()
        assertThat(docFontSizeSp("640px")).isNull()
        assertThat(docFontSizeSp("large")).isNull()
        assertThat(docFontSizeSp(null)).isNull()
    }

    @Test
    fun `a font stack is classified by its generic family`() {
        // An imported document names fonts this device does not have, but the
        // stack still ends in the family the author chose.
        assertThat(docFontKind("ui-monospace, SFMono-Regular, Menlo, monospace"))
            .isEqualTo(DocFontKind.MONO)
        // "sans-serif" contains "serif" — the wider match has to win.
        assertThat(docFontKind("system-ui, -apple-system, Segoe UI, Roboto, sans-serif"))
            .isEqualTo(DocFontKind.SANS)
        assertThat(docFontKind("Cambria, Georgia, serif")).isEqualTo(DocFontKind.SERIF)
        assertThat(docFontKind("")).isEqualTo(DocFontKind.DEFAULT)
        assertThat(docFontKind(null)).isEqualTo(DocFontKind.DEFAULT)
    }

    @Test
    fun `docPlainText names a PDF rather than calling the document empty`() {
        val text = docPlainText(
            JsonParser.parseString(
                """{"type":"doc","content":[{"type":"pdfEmbed","attrs":{"name":"Смета.pdf","src":"/x"}}]}""",
            ),
        )
        assertThat(text).isEqualTo("Смета.pdf")
    }
}
