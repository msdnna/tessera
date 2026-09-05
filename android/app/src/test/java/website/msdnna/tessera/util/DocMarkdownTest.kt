package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonObject
import kotlin.random.Random
import org.junit.Test

/**
 * Markdown → ProseMirror, the one writer of document JSON the phone owns
 * (§8 of #2894).
 *
 * What it is really guarding is the shape the *server* validates and the reader
 * parses back: a node type it does not know, a missing `attrs.id`, or a task
 * list flattened into bullets all produce a document that looks fine in the
 * gallery and is wrong the moment it is opened. The built-in templates go
 * through this on every use, so a regression here ships three broken starters.
 */
class DocMarkdownTest {
    /** Deterministic ids — the one part of the output that cannot be asserted
     *  on, and the one that would otherwise make every expectation unstable. */
    private fun doc(md: String): JsonObject = markdownToDocJson(md, Random(7))

    private fun blocks(md: String) = doc(md).getAsJsonArray("content").map { it.asJsonObject }

    private fun typeOf(node: JsonObject) = node.get("type").asString

    private fun textOf(node: JsonObject): String {
        val content = node.getAsJsonArray("content") ?: return ""
        return content.joinToString("") { child ->
            val obj = child.asJsonObject
            if (typeOf(obj) == "text") obj.get("text").asString else textOf(obj)
        }
    }

    @Test
    fun `headings keep their level`() {
        val out = blocks("# Один\n\n### Три\n")
        assertThat(out.map(::typeOf)).containsExactly("heading", "heading").inOrder()
        assertThat(out[0].getAsJsonObject("attrs").get("level").asInt).isEqualTo(1)
        assertThat(out[1].getAsJsonObject("attrs").get("level").asInt).isEqualTo(3)
        assertThat(textOf(out[0])).isEqualTo("Один")
    }

    @Test
    fun `every block carries a unique id`() {
        val out = blocks("# Заголовок\n\nАбзац\n\n- пункт\n\n---\n")
        val ids = out.map { it.getAsJsonObject("attrs").get("id").asString }
        assertThat(ids).hasSize(4)
        assertThat(ids.toSet()).hasSize(4)
        assertThat(ids.all { it.length == 16 }).isTrue()
    }

    @Test
    fun `a checklist stays a checklist`() {
        // The trap: `- [ ]` also matches the plain bullet marker, and a
        // checklist rendered as bullets loses every box in the template.
        val out = blocks("- [ ] не сделано\n- [x] сделано\n")
        assertThat(out).hasSize(1)
        assertThat(typeOf(out[0])).isEqualTo("taskList")
        val items = out[0].getAsJsonArray("content").map { it.asJsonObject }
        assertThat(items.map { typeOf(it) }).containsExactly("taskItem", "taskItem")
        assertThat(items[0].getAsJsonObject("attrs").get("checked").asBoolean).isFalse()
        assertThat(items[1].getAsJsonObject("attrs").get("checked").asBoolean).isTrue()
        assertThat(textOf(items[0])).isEqualTo("не сделано")
    }

    @Test
    fun `bullet and ordered lists are told apart`() {
        val bullet = blocks("- раз\n- два\n")
        val ordered = blocks("1. раз\n2. два\n")
        assertThat(typeOf(bullet[0])).isEqualTo("bulletList")
        assertThat(typeOf(ordered[0])).isEqualTo("orderedList")
        assertThat(ordered[0].getAsJsonArray("content")).hasSize(2)
    }

    @Test
    fun `a table becomes a table with a header row`() {
        val out = blocks("| Вопрос | Кто отвечает |\n|---|---|\n| раз | два |\n")
        assertThat(out).hasSize(1)
        assertThat(typeOf(out[0])).isEqualTo("table")
        val rows = out[0].getAsJsonArray("content").map { it.asJsonObject }
        assertThat(rows).hasSize(2)
        val header = rows[0].getAsJsonArray("content").map { it.asJsonObject }
        val body = rows[1].getAsJsonArray("content").map { it.asJsonObject }
        assertThat(header.map(::typeOf)).containsExactly("tableHeader", "tableHeader")
        assertThat(body.map(::typeOf)).containsExactly("tableCell", "tableCell")
        assertThat(textOf(header[0])).isEqualTo("Вопрос")
    }

    @Test
    fun `a pipe line without a divider under it is not a table`() {
        // Otherwise a sentence that happens to start with a pipe eats the lines
        // after it and the paragraph disappears.
        val out = blocks("| просто текст\n")
        assertThat(typeOf(out[0])).isEqualTo("paragraph")
    }

    @Test
    fun `inline marks survive`() {
        val out = blocks("**Дата:** и *курсив* и `код` и [ссылка](https://t.local)\n")
        val spans = out[0].getAsJsonArray("content").map { it.asJsonObject }
        fun markNames(node: JsonObject): List<String> =
            node.getAsJsonArray("marks")?.map { it.asJsonObject.get("type").asString }.orEmpty()

        assertThat(markNames(spans[0])).containsExactly("bold")
        assertThat(spans[0].get("text").asString).isEqualTo("Дата:")
        val link = spans.first { it.get("text").asString == "ссылка" }
        assertThat(markNames(link)).containsExactly("link")
        assertThat(link.getAsJsonArray("marks")[0].asJsonObject.getAsJsonObject("attrs").get("href").asString)
            .isEqualTo("https://t.local")
        assertThat(spans.first { it.get("text").asString == "код" }.let(::markNames))
            .containsExactly("code")
    }

    @Test
    fun `a code span wins over emphasis inside it`() {
        val out = blocks("текст `a**b**c` конец\n")
        val spans = out[0].getAsJsonArray("content").map { it.asJsonObject }
        val code = spans.first { it.getAsJsonArray("marks") != null }
        assertThat(code.get("text").asString).isEqualTo("a**b**c")
    }

    @Test
    fun `a fenced block keeps its language and its line breaks`() {
        val out = blocks("```kotlin\nval a = 1\nval b = 2\n```\n")
        assertThat(typeOf(out[0])).isEqualTo("codeBlock")
        assertThat(out[0].getAsJsonObject("attrs").get("language").asString).isEqualTo("kotlin")
        assertThat(textOf(out[0])).isEqualTo("val a = 1\nval b = 2")
    }

    @Test
    fun `wrapped prose is one paragraph and a blank line ends it`() {
        val out = blocks("первая строка\nвторая строка\n\nдругой абзац\n")
        assertThat(out.map(::typeOf)).containsExactly("paragraph", "paragraph").inOrder()
        assertThat(textOf(out[0])).isEqualTo("первая строка вторая строка")
    }

    @Test
    fun `an empty file is still a document`() {
        val out = doc("")
        assertThat(out.get("type").asString).isEqualTo("doc")
        assertThat(out.getAsJsonArray("content")).hasSize(1)
    }

    @Test
    fun `blockquotes hold their paragraphs`() {
        val out = blocks("> цитата\n> продолжение\n")
        assertThat(typeOf(out[0])).isEqualTo("blockquote")
        assertThat(textOf(out[0])).isEqualTo("цитата продолжение")
    }

    @Test
    fun `first heading names an imported file`() {
        assertThat(firstMarkdownHeading("текст\n\n## Настоящее имя\n\nещё\n"))
            .isEqualTo("Настоящее имя")
        assertThat(firstMarkdownHeading("совсем без заголовков")).isEmpty()
        // A lone `#` is not a heading — it is a line with a hash on it, and
        // naming a document "" would make the server refuse the create.
        assertThat(firstMarkdownHeading("#\n")).isEmpty()
    }

    @Test
    fun `built-in template bodies parse into the blocks they promise`() {
        // The shape of the shipped starters, in one place: three headings, a
        // checklist and a table are what «Техническое задание» *is*, and a
        // silently flattened one would still create a document.
        val spec = blocks(
            "# ТЗ\n\n## Объём\n\n- \n\n## Критерии\n\n- [ ] \n\n## Риски\n\n" +
                "| Вопрос | Кто |\n|---|---|\n|  |  |\n",
        )
        assertThat(spec.map(::typeOf)).containsExactly(
            "heading", "heading", "bulletList", "heading", "taskList", "heading", "table",
        ).inOrder()
    }
}
