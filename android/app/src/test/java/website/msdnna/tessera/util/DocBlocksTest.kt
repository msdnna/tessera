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
}
