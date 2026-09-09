package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.DocumentConverterStatus
import website.msdnna.tessera.data.model.DocumentTemplate

/**
 * Which road a picked file takes, what an export is called, and how the gallery
 * is put together (#2733, #2734 — §8 of #2894).
 *
 * The costly mistakes here are all silent. Sending a `.md` to the converter
 * makes the import fail on an install that has none; treating a PDF as an
 * office format hides an import that works; and dropping `html` from the export
 * list makes the cheapest export depend on the heaviest dependency. None of
 * that shows up until the sidecar is down, which is exactly when nobody is
 * looking.
 */
class DocFilesTest {
    @Test
    fun `each format takes its own road`() {
        assertThat(docImportRoute("заметка.md")).isEqualTo(DocImportRoute.LOCAL)
        assertThat(docImportRoute("body.JSON")).isEqualTo(DocImportRoute.LOCAL)
        assertThat(docImportRoute("договор.docx")).isEqualTo(DocImportRoute.OFFICE)
        assertThat(docImportRoute("scan.pdf")).isEqualTo(DocImportRoute.PDF)
        assertThat(docImportRoute("фото.png")).isEqualTo(DocImportRoute.UNSUPPORTED)
        assertThat(docImportRoute("noextension")).isEqualTo(DocImportRoute.UNSUPPORTED)
    }

    @Test
    fun `only office formats are gated on the converter`() {
        // PDF and the two locally-parsed formats keep working on an install
        // with no sidecar — that is the whole point of asking.
        assertThat(docImportNeedsConverter("договор.docx")).isTrue()
        assertThat(docImportNeedsConverter("scan.pdf")).isFalse()
        assertThat(docImportNeedsConverter("заметка.md")).isFalse()
    }

    @Test
    fun `the local ceiling is the tighter of the two`() {
        // A Markdown file expands when parsed, and the server caps the body at
        // 4 MiB — so it is refused before it is read, not after.
        assertThat(docImportLimit(DocImportRoute.LOCAL)).isEqualTo(MAX_DOC_LOCAL_BYTES)
        assertThat(docImportLimit(DocImportRoute.OFFICE)).isEqualTo(MAX_DOC_OFFICE_BYTES)
        assertThat(docImportLimit(DocImportRoute.PDF)).isEqualTo(MAX_DOC_OFFICE_BYTES)
        assertThat(MAX_DOC_LOCAL_BYTES).isLessThan(MAX_DOC_OFFICE_BYTES)
    }

    @Test
    fun `html is always exportable`() {
        assertThat(docExportFormats(null)).containsExactly("html")
        assertThat(docExportFormats(DocumentConverterStatus(available = false)))
            .containsExactly("html")
        assertThat(docExportFormats(DocumentConverterStatus(exportFormats = listOf("pdf", "docx"))))
            .containsExactly("pdf", "docx").inOrder()
    }

    @Test
    fun `export names survive Cyrillic and lose only what breaks a file system`() {
        assertThat(docExportFileName("Протокол 12/03", "pdf", "Документ"))
            .isEqualTo("Протокол 12_03.pdf")
        assertThat(docExportFileName("   ", "html", "Документ")).isEqualTo("Документ.html")
        assertThat(docExportFileName("a:b*c?d\"e<f>g|h", "docx", "Документ"))
            .isEqualTo("a_b_c_d_e_f_g_h.docx")
    }

    @Test
    fun `a markdown file is named by its first heading`() {
        val draft = parseDocFile("выгрузка.md", "# Настоящее имя\n\nтекст\n")
        assertThat(draft.title).isEqualTo("Настоящее имя")
        assertThat(draft.content.asJsonObject.get("type").asString).isEqualTo("doc")
    }

    @Test
    fun `a markdown file without a heading falls back to the file name`() {
        val draft = parseDocFile("выгрузка.md", "просто текст")
        assertThat(draft.title).isEqualTo("выгрузка")
    }

    @Test
    fun `a bare body and our own export envelope are both readable`() {
        val bare = parseDocFile("body.json", """{"type":"doc","content":[]}""")
        assertThat(bare.content.asJsonObject.get("type").asString).isEqualTo("doc")
        assertThat(bare.title).isEqualTo("body")

        // Refusing to read back the file the gallery itself exports would be a
        // trap — the envelope is what a user gets when they export a template.
        val enveloped = parseDocFile(
            "tpl.json",
            """{"title":"Шаблон","icon":"📋","content":{"type":"doc","content":[]}}""",
        )
        assertThat(enveloped.title).isEqualTo("Шаблон")
        assertThat(enveloped.icon).isEqualTo("📋")
        assertThat(enveloped.content.asJsonObject.get("type").asString).isEqualTo("doc")
    }

    @Test
    fun `json that is not a document is refused, not half-imported`() {
        assertThat(runCatching { parseDocFile("x.json", "не json вовсе") }.exceptionOrNull())
            .isInstanceOf(DocFileFormatException::class.java)
        assertThat(runCatching { parseDocFile("x.json", """{"hello":1}""") }.exceptionOrNull())
            .isInstanceOf(DocFileFormatException::class.java)
        assertThat(runCatching { parseDocFile("x.json", """{"content":{"type":"paragraph"}}""") }.exceptionOrNull())
            .isInstanceOf(DocFileFormatException::class.java)
    }

    @Test
    fun `the gallery puts the team's own templates first`() {
        val saved = listOf(
            DocumentTemplate(id = "t1", title = "Наш шаблон", authorName = "Аня"),
            DocumentTemplate(id = "t2", title = "Второй", preview = "первые\nстроки"),
        )
        val builtins = listOf(
            builtinTemplateCard(DocBuiltinTemplate.MEETING, "Протокол", "Участники"),
        )
        val cards = docTemplateGallery(saved, builtins)
        assertThat(cards.map { it.id }).containsExactly("t1", "t2", "builtin:meeting").inOrder()
        assertThat(cards[0].builtin).isFalse()
        assertThat(cards[0].authorName).isEqualTo("Аня")
        assertThat(cards[2].builtin).isTrue()
        assertThat(cards[2].builtinKey).isEqualTo("meeting")
        // A template with no description shows the server's preview instead of
        // an empty line under its name.
        assertThat(cards[1].description).isEqualTo("первые строки")
    }

    @Test
    fun `search looks at the description too`() {
        val cards = listOf(
            DocTemplateCard(id = "a", title = "Протокол", description = "повестка и решения", icon = ""),
            DocTemplateCard(id = "b", title = "Ретро", description = "что мешало", icon = ""),
        )
        assertThat(filterDocTemplates(cards, "  ").map { it.id }).containsExactly("a", "b")
        assertThat(filterDocTemplates(cards, "ПРОТО").map { it.id }).containsExactly("a")
        assertThat(filterDocTemplates(cards, "мешало").map { it.id }).containsExactly("b")
        assertThat(filterDocTemplates(cards, "нетакого")).isEmpty()
    }
}
