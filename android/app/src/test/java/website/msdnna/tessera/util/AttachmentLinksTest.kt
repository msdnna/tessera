package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import org.junit.Test

/**
 * #2932: файл, приложенный внутрь описания или комментария, на телефоне открыть
 * было нечем. Редактор вставляет его ссылкой
 * `[имя](/api/attachments/<uuid>/download)`, а маршрут скачивания стоит в
 * защищённой группе — браузер, которому WebView отдавал такой переход, может
 * получить только 401.
 *
 * Разбор ссылки и имени файла — чистые функции, поэтому и проверяются здесь, без
 * WebView и сети.
 */
class AttachmentLinksTest {
    @Test
    fun `root-relative link from the editor is recognised`() {
        val id = "0f3c7f1e-6b2a-4c5d-9e8f-1a2b3c4d5e6f"
        assertThat(attachmentIdFromHref("/api/attachments/$id/download")).isEqualTo(id)
    }

    @Test
    fun `absolute link is recognised too`() {
        // JS документа разворачивает корневые href в адрес сервера, так что до
        // Kotlin доезжает уже абсолютный URL.
        val id = "0F3C7F1E-6B2A-4C5D-9E8F-1A2B3C4D5E6F"
        assertThat(attachmentIdFromHref("https://tessera.example/api/attachments/$id/download"))
            .isEqualTo(id)
    }

    @Test
    fun `ordinary links are left alone`() {
        listOf(
            null,
            "",
            "https://example.com/docs",
            "/api/uploads/0f3c7f1e-6b2a-4c5d-9e8f-1a2b3c4d5e6f.png",
            "/api/tasks/0f3c7f1e-6b2a-4c5d-9e8f-1a2b3c4d5e6f",
            // Не uuid: ссылка на что-то другое, а не строка вложения.
            "/api/attachments/12/download",
            "/api/attachments/0f3c7f1e-6b2a-4c5d-9e8f-1a2b3c4d5e6f",
        ).forEach {
            assertWithMessage("ссылка ошибочно принята за файл задачи: $it")
                .that(attachmentIdFromHref(it)).isNull()
        }
    }

    // ── имя файла из ответа сервера ──────────────────────────────────────────

    @Test
    fun `russian name comes from the filename star parameter`() {
        // Ровно то, что пишет backend/handlers/content_disposition.go: ASCII-фолбэк
        // из подчёркиваний плюс RFC 8187 ext-value.
        val header = "attachment; filename=\"_____ __ ______.pdf\"; " +
            "filename*=UTF-8''%D0%9E%D1%82%D1%87%D1%91%D1%82.pdf"
        assertThat(filenameFromContentDisposition(header)).isEqualTo("Отчёт.pdf")
    }

    @Test
    fun `ascii name is used when there is no star parameter`() {
        assertThat(filenameFromContentDisposition("attachment; filename=\"report.pdf\""))
            .isEqualTo("report.pdf")
    }

    @Test
    fun `plus in a name is a plus and not a space`() {
        // `+` — attr-char, сервер его не кодирует; URLDecoder без подготовки
        // прочитал бы его как пробел формы.
        assertThat(filenameFromContentDisposition("attachment; filename*=UTF-8''v1+2.txt"))
            .isEqualTo("v1+2.txt")
    }

    @Test
    fun `a nameless header yields nothing`() {
        // Сервер отвечает голым "attachment", когда имя пустое.
        listOf(null, "", "attachment").forEach {
            assertWithMessage("имя выдумано из заголовка: $it")
                .that(filenameFromContentDisposition(it)).isNull()
        }
    }

    // ── имя файла в кэше ─────────────────────────────────────────────────────

    @Test
    fun `a name never becomes a path`() {
        assertThat(safeDownloadName("../../etc/passwd", "fallback")).isEqualTo(".._.._etc_passwd")
        assertThat(safeDownloadName("""dir\file.txt""", "fallback")).isEqualTo("dir_file.txt")
        assertThat(safeDownloadName("..", "fallback")).isEqualTo("fallback")
    }

    @Test
    fun `an empty name falls back to the row id`() {
        listOf(null, "", "   ").forEach {
            assertThat(safeDownloadName(it, "att-id")).isEqualTo("att-id")
        }
    }
}
