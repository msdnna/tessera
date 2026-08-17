package website.msdnna.tessera.data.model

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test
import website.msdnna.tessera.util.DocHeading
import website.msdnna.tessera.util.DocListRow
import website.msdnna.tessera.util.parseDocBlocks

/**
 * Contract test against **real** server output (#2735).
 *
 * [Document]'s field names were derived by reading the Go structs, and a
 * `@SerializedName` typo is invisible in a hand-written fixture: Gson simply
 * leaves the field at its default, and the screen renders an empty title
 * instead of failing. The payloads below are verbatim responses from
 * `GET /workspaces/:id/documents` and `GET /documents/:id`, captured against a
 * backend built from this branch.
 */
class DocumentJsonTest {
    private val gson = Gson()

    private val listRow = """
        {
          "id": "517aad6e-4a27-46a3-a7be-4034127ecebf",
          "workspace_id": "463beadb-3157-44e0-bae3-05eca7ed8fa5",
          "project_id": null,
          "parent_id": null,
          "author_id": "ffeef8b0-8a5c-4556-af72-4d700aa2b577",
          "title": "Регламент ночной смены",
          "slug": "reglament-nochnoy-smeny",
          "icon": "📘",
          "preview": "Регламент Обычный текст, жирный",
          "position": 65536,
          "created_at": "2026-08-16T01:01:24.728652+03:00",
          "updated_at": "2026-08-16T01:01:24.734639+03:00"
        }
    """.trimIndent()

    @Test
    fun `a list row maps onto every field`() {
        val doc = gson.fromJson(listRow, Document::class.java)
        assertThat(doc.id).isEqualTo("517aad6e-4a27-46a3-a7be-4034127ecebf")
        assertThat(doc.workspaceId).isEqualTo("463beadb-3157-44e0-bae3-05eca7ed8fa5")
        assertThat(doc.authorId).isEqualTo("ffeef8b0-8a5c-4556-af72-4d700aa2b577")
        assertThat(doc.title).isEqualTo("Регламент ночной смены")
        assertThat(doc.slug).isEqualTo("reglament-nochnoy-smeny")
        assertThat(doc.icon).isEqualTo("📘")
        assertThat(doc.preview).isEqualTo("Регламент Обычный текст, жирный")
        assertThat(doc.position).isEqualTo(65536.0)
        assertThat(doc.parentId).isNull()
        assertThat(doc.projectId).isNull()
        // The list query omits `content` entirely — the reader must treat that
        // as "not loaded yet", not as "empty document".
        assertThat(doc.content).isNull()
    }

    @Test
    fun `a detail response parses into blocks`() {
        val detail = """
            {
              "id": "517aad6e-4a27-46a3-a7be-4034127ecebf",
              "workspace_id": "463beadb-3157-44e0-bae3-05eca7ed8fa5",
              "parent_id": null,
              "title": "Регламент ночной смены",
              "icon": "📘",
              "preview": "Регламент",
              "position": 65536,
              "content": {
                "type": "doc",
                "content": [
                  {"type": "heading", "attrs": {"id": "h1", "level": 1},
                   "content": [{"text": "Регламент", "type": "text"}]},
                  {"type": "bulletList", "attrs": {"id": "ul"}, "content": [
                    {"type": "listItem", "content": [
                      {"type": "paragraph", "content": [{"text": "Первый пункт", "type": "text"}]},
                      {"type": "bulletList", "content": [
                        {"type": "listItem", "content": [
                          {"type": "paragraph", "content": [{"text": "Вложенный", "type": "text"}]}]}]}]}]}
                ]
              }
            }
        """.trimIndent()
        val doc = gson.fromJson(detail, Document::class.java)
        val blocks = parseDocBlocks(doc.content)

        val heading = blocks.first() as DocHeading
        assertThat(heading.level).isEqualTo(1)
        assertThat(heading.spans.single().text).isEqualTo("Регламент")

        val rows = blocks.filterIsInstance<DocListRow>()
        assertThat(rows.map { it.depth }).containsExactly(0, 1).inOrder()
        assertThat(rows.map { row -> row.spans.joinToString("") { it.text } })
            .containsExactly("Первый пункт", "Вложенный").inOrder()
    }
}
