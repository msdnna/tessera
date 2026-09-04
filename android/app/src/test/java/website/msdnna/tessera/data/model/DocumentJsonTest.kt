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

    // ── #2894: the write side of the module ───────────────────────────────────
    // Same rule as above and the same reason: these payloads are verbatim
    // responses from a backend built off this branch (:8094, scratch database
    // `tessera_night_2894`), not hand-written shapes. A `@SerializedName` typo in
    // a field the server *does* send is otherwise invisible — Gson leaves the
    // property at its default and the panel renders an empty row.

    @Test
    fun `a content save answers with the new timestamp, not the document`() {
        val json = """
            {
              "id": "56f8967a-c570-4d7f-9a70-52072d423efb",
              "preview": "Регламент Первый абзац",
              "updated_at": "2026-09-05T00:28:25.716274+03:00"
            }
        """.trimIndent()
        val saved = gson.fromJson(json, DocumentContentSaved::class.java)
        assertThat(saved.id).isEqualTo("56f8967a-c570-4d7f-9a70-52072d423efb")
        assertThat(saved.updatedAt).isEqualTo("2026-09-05T00:28:25.716274+03:00")
        assertThat(saved.preview).isEqualTo("Регламент Первый абзац")
    }

    @Test
    fun `a comment list carries the thread shape and the joined author`() {
        val json = """
            [
              {
                "id": "814a4a4e-fe2a-42f4-a3c0-163593ed8a1f",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "block_id": "p1",
                "parent_id": null,
                "author_id": "1c747613-5d09-4e61-9277-941310024aa1",
                "body": "Тут нужен пример",
                "quote": "Первый абзац",
                "resolved_at": "2026-09-05T00:28:25.790608+03:00",
                "resolved_by": "1c747613-5d09-4e61-9277-941310024aa1",
                "created_at": "2026-09-05T00:28:25.75273+03:00",
                "updated_at": "2026-09-05T00:28:25.791693+03:00",
                "author_name": "Док Тестовый",
                "author_email": "doc2894@test.local"
              },
              {
                "id": "7382f526-d65a-4a0e-bca3-cf437ca7258d",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "block_id": "p1",
                "parent_id": "814a4a4e-fe2a-42f4-a3c0-163593ed8a1f",
                "author_id": "1c747613-5d09-4e61-9277-941310024aa1",
                "body": "Согласен",
                "quote": "",
                "resolved_at": null,
                "resolved_by": null,
                "created_at": "2026-09-05T00:28:25.781978+03:00",
                "updated_at": "2026-09-05T00:28:25.781978+03:00",
                "author_name": "Док Тестовый",
                "author_email": "doc2894@test.local"
              }
            ]
        """.trimIndent()
        val comments = gson.fromJson(json, Array<DocumentComment>::class.java).toList()

        val root = comments.first()
        assertThat(root.isRoot).isTrue()
        assertThat(root.isResolved).isTrue()
        assertThat(root.blockId).isEqualTo("p1")
        assertThat(root.quote).isEqualTo("Первый абзац")
        assertThat(root.authorName).isEqualTo("Док Тестовый")
        assertThat(root.authorEmail).isEqualTo("doc2894@test.local")

        val reply = comments.last()
        assertThat(reply.isRoot).isFalse()
        assertThat(reply.parentId).isEqualTo(root.id)
        // The server rewrites a reply's anchor to the root's block, whatever the
        // client sent — the panel relies on that to group a thread by block.
        assertThat(reply.blockId).isEqualTo("p1")
        assertThat(reply.isResolved).isFalse()
    }

    @Test
    fun `the version journal arrives newest first and without bodies`() {
        val json = """
            [
              {
                "id": "440dfe21-b2dd-43e3-874d-ff9fd9a0ab9f",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "revision": 2,
                "author_id": "1c747613-5d09-4e61-9277-941310024aa1",
                "title": "Регламент ночной смены",
                "preview": "Регламент Первый абзац",
                "label": "Перед правкой",
                "manual": true,
                "created_at": "2026-09-05T00:28:25.804816+03:00",
                "updated_at": "2026-09-05T00:28:25.804816+03:00",
                "author_name": "Док Тестовый",
                "author_email": "doc2894@test.local"
              },
              {
                "id": "92129200-64ef-4f00-aed3-23254b34c1b9",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "revision": 1,
                "author_id": "1c747613-5d09-4e61-9277-941310024aa1",
                "title": "Регламент ночной смены",
                "preview": "Регламент Первый абзац",
                "label": "",
                "manual": false,
                "created_at": "2026-09-05T00:28:25.726915+03:00",
                "updated_at": "2026-09-05T00:28:25.726915+03:00",
                "author_name": "Док Тестовый",
                "author_email": "doc2894@test.local"
              }
            ]
        """.trimIndent()
        val versions = gson.fromJson(json, Array<DocumentVersion>::class.java).toList()

        assertThat(versions.map { it.revision }).containsExactly(2, 1).inOrder()
        assertThat(versions.first().manual).isTrue()
        assertThat(versions.first().label).isEqualTo("Перед правкой")
        assertThat(versions.last().manual).isFalse()
        // No bodies in the list — the history sheet must fetch one to preview it,
        // not render `content` it never received.
        assertThat(versions.all { it.content == null }).isTrue()
    }

    @Test
    fun `a template row carries a preview and the gallery has no body`() {
        val json = """
            [
              {
                "id": "6925c527-a3ae-40fb-bc98-f128fb619d06",
                "workspace_id": "9af62e0f-9f01-4dad-9f83-4714f3217f09",
                "author_id": "1c747613-5d09-4e61-9277-941310024aa1",
                "title": "Протокол совещания",
                "description": "Шапка и повестка",
                "icon": "📝",
                "preview": "Регламент Первый абзац",
                "created_at": "2026-09-05T00:28:25.840145+03:00",
                "updated_at": "2026-09-05T00:28:25.840145+03:00",
                "author_name": "Док Тестовый"
              }
            ]
        """.trimIndent()
        val template = gson.fromJson(json, Array<DocumentTemplate>::class.java).single()
        assertThat(template.title).isEqualTo("Протокол совещания")
        assertThat(template.description).isEqualTo("Шапка и повестка")
        assertThat(template.icon).isEqualTo("📝")
        assertThat(template.preview).isEqualTo("Регламент Первый абзац")
        assertThat(template.authorName).isEqualTo("Док Тестовый")
        assertThat(template.content).isNull()
    }

    @Test
    fun `a task link joins in enough of the task to render a row`() {
        val json = """
            [
              {
                "id": "16c7c28c-7c9b-4a75-99fd-cf9ea7255acc",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "task_id": "f803f8d0-8db4-4dec-9e9e-6915ae91194b",
                "block_id": "p1",
                "quote": "Первый абзац",
                "created_by": "1c747613-5d09-4e61-9277-941310024aa1",
                "created_at": "2026-09-05T00:29:12.854933+03:00",
                "task_title": "Свести регламент",
                "task_board_id": "40b0ac4b-0c64-4e53-8b05-20c68410fd71",
                "task_priority": 3,
                "task_completed_at": null
              }
            ]
        """.trimIndent()
        val link = gson.fromJson(json, Array<DocumentTaskLink>::class.java).single()
        assertThat(link.taskTitle).isEqualTo("Свести регламент")
        assertThat(link.taskPriority).isEqualTo(3)
        // The board id is what a tap on the row navigates with.
        assertThat(link.taskBoardId).isEqualTo("40b0ac4b-0c64-4e53-8b05-20c68410fd71")
        assertThat(link.taskCompletedAt).isNull()
    }

    @Test
    fun `the same link read from the task side names the document`() {
        val json = """
            [
              {
                "id": "49d1d4a1-fad8-4103-b509-ba02d6ef3000",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "task_id": "9a2e7652-3338-4c95-bffe-619ec5171b64",
                "block_id": "p1",
                "quote": "Первый абзац",
                "created_by": "1c747613-5d09-4e61-9277-941310024aa1",
                "created_at": "2026-09-05T00:29:16.795028+03:00",
                "document_title": "Регламент ночной смены",
                "document_icon": "📘",
                "document_slug": "reglament-nochnoy-smeny",
                "document_workspace_id": "9af62e0f-9f01-4dad-9f83-4714f3217f09"
              }
            ]
        """.trimIndent()
        val link = gson.fromJson(json, Array<TaskDocumentLink>::class.java).single()
        assertThat(link.documentTitle).isEqualTo("Регламент ночной смены")
        assertThat(link.documentIcon).isEqualTo("📘")
        assertThat(link.documentSlug).isEqualTo("reglament-nochnoy-smeny")
        assertThat(link.documentWorkspaceId).isEqualTo("9af62e0f-9f01-4dad-9f83-4714f3217f09")
    }

    @Test
    fun `an approval nests its route and pins a revision`() {
        val json = """
            [
              {
                "id": "1997a604-281a-4e68-b836-15d9c20aa504",
                "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
                "version_id": "b1da41ab-8a14-4adb-be46-5283131f1b09",
                "title": "Утвердить регламент",
                "status": "approved",
                "mode": "sequential",
                "created_by": "1c747613-5d09-4e61-9277-941310024aa1",
                "created_at": "2026-09-05T00:29:16.833382+03:00",
                "closed_at": "2026-09-05T00:29:16.874292+03:00",
                "version_revision": 3,
                "created_by_name": "Док Тестовый",
                "steps": [
                  {
                    "id": "a9b9758c-e969-4239-b590-3566b22728ca",
                    "approval_id": "1997a604-281a-4e68-b836-15d9c20aa504",
                    "approver_id": "1c747613-5d09-4e61-9277-941310024aa1",
                    "approver_name": "Док Тестовый",
                    "position": 0,
                    "status": "approved",
                    "comment": "Ок",
                    "signature": "Док Тестовый",
                    "decided_at": "2026-09-05T00:29:16.87084+03:00",
                    "approver_email": "doc2894@test.local"
                  }
                ]
              }
            ]
        """.trimIndent()
        val approval = gson.fromJson(json, Array<DocumentApproval>::class.java).single()
        assertThat(approval.status).isEqualTo(DocumentApprovalStatus.APPROVED)
        assertThat(approval.mode).isEqualTo(DocumentApprovalMode.SEQUENTIAL)
        assertThat(approval.isOpen).isFalse()
        // The revision is what says *which text* the route is about; an id would
        // not answer that, so a rename of the joined column must fail here.
        assertThat(approval.versionRevision).isEqualTo(3)
        assertThat(approval.createdByName).isEqualTo("Док Тестовый")

        val step = approval.steps.single()
        assertThat(step.approverName).isEqualTo("Док Тестовый")
        assertThat(step.status).isEqualTo(DocumentApprovalStatus.APPROVED)
        assertThat(step.signature).isEqualTo("Док Тестовый")
        assertThat(step.comment).isEqualTo("Ок")
        assertThat(step.decidedAt).isNotNull()
    }

    @Test
    fun `creating an approval answers with a bare row, and it still parses`() {
        // Create and cancel return the row without the joined revision or the
        // route; the model defaults rather than requiring them, and this is the
        // fixture that keeps that true.
        val json = """
            {
              "id": "1997a604-281a-4e68-b836-15d9c20aa504",
              "document_id": "56f8967a-c570-4d7f-9a70-52072d423efb",
              "version_id": "b1da41ab-8a14-4adb-be46-5283131f1b09",
              "title": "Утвердить регламент",
              "status": "pending",
              "mode": "sequential",
              "created_by": "1c747613-5d09-4e61-9277-941310024aa1",
              "created_at": "2026-09-05T00:29:16.833382+03:00",
              "closed_at": null
            }
        """.trimIndent()
        val approval = gson.fromJson(json, DocumentApproval::class.java)
        assertThat(approval.isOpen).isTrue()
        assertThat(approval.closedAt).isNull()
        assertThat(approval.steps).isEmpty()
        assertThat(approval.versionRevision).isEqualTo(0)
    }

    @Test
    fun `deciding answers with the step, and it lacks the joined email`() {
        val json = """
            {
              "id": "a9b9758c-e969-4239-b590-3566b22728ca",
              "approval_id": "1997a604-281a-4e68-b836-15d9c20aa504",
              "approver_id": "1c747613-5d09-4e61-9277-941310024aa1",
              "approver_name": "Док Тестовый",
              "position": 0,
              "status": "approved",
              "comment": "Ок",
              "signature": "Док Тестовый",
              "decided_at": "2026-09-05T00:29:16.87084+03:00"
            }
        """.trimIndent()
        val step = gson.fromJson(json, DocumentApprovalStep::class.java)
        assertThat(step.status).isEqualTo(DocumentApprovalStatus.APPROVED)
        assertThat(step.approverEmail).isNull()
    }

    @Test
    fun `converter status still names the native formats when it is off`() {
        // The install under test has no sidecar. That branch matters more than the
        // happy one: PDF import needs no converter, so a client that hid import on
        // `available:false` would hide a feature that works.
        val json = """
            {
              "available": false,
              "native_formats": [".pdf"],
              "reason": "сервис конвертации недоступен"
            }
        """.trimIndent()
        val status = gson.fromJson(json, DocumentConverterStatus::class.java)
        assertThat(status.available).isFalse()
        assertThat(status.nativeFormats).containsExactly(".pdf")
        assertThat(status.reason).isEqualTo("сервис конвертации недоступен")
        assertThat(status.importFormats).isEmpty()
    }
}
