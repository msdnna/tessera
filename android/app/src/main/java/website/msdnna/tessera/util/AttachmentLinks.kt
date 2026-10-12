package website.msdnna.tessera.util

/**
 * Ссылки на файлы задачи внутри описаний и комментариев.
 *
 * Их вставляет редактор, когда в текст кладут не картинку: web
 * `MarkdownEditor.vue` пишет `[имя](/api/attachments/<uuid>/download)`, и такой
 * же markdown приезжает на телефон. Следовать такой ссылке нельзя нигде:
 * `GET /attachments/:id/download` живёт в защищённой группе роутера, а токен
 * держится только в памяти клиента — браузеру его никто не отдаст, и переход
 * может закончиться лишь 401. Поэтому на web клик перехватывает
 * `RichContent.vue` (`attachmentIdFromHref` + `saveAttachment`), а здесь — тот
 * же разбор для `RichContent`.
 *
 * Префикс origin необязателен: разметка хранится с корневым `/api/…`, но JS в
 * документе разворачивает такие href в абсолютный адрес сервера, так что до
 * `shouldOverrideUrlLoading` доезжает уже полный URL.
 */
private val ATTACHMENT_HREF_RE = Regex("""/api/attachments/([0-9a-fA-F-]{36})/download""")

/** Id вложения, на которое показывает ссылка, или null для обычной ссылки. */
fun attachmentIdFromHref(href: String?): String? =
    ATTACHMENT_HREF_RE.find(href.orEmpty())?.groupValues?.get(1)

/**
 * Имя файла из заголовка `Content-Disposition` ответа сервера.
 *
 * Сервер зовёт его единственным источником имени (`handlers/content_disposition.go`):
 * в самой ссылке имени нет — в пути лежит uuid строки, — а текст ссылки пишет
 * человек и он может быть каким угодно («вот лог», «см. файл»). Разбираются обе
 * формы, которые пишет бэкенд: `filename*=UTF-8''<percent-encoded>` для имён с
 * не-ASCII (по ней и идут русские имена) и ASCII-фолбэк `filename="…"`.
 */
fun filenameFromContentDisposition(header: String?): String? {
    val raw = header.orEmpty()
    if (raw.isBlank()) return null
    Regex("""filename\*\s*=\s*UTF-8''([^;]+)""", RegexOption.IGNORE_CASE).find(raw)?.let { m ->
        // `+` — это attr-char, сервер оставляет его в имени как есть, а
        // URLDecoder читает плюс как пробел (форма, не ext-value): файл
        // «отчёт v1+2.pdf» иначе приезжает с пробелом вместо плюса.
        val encoded = m.groupValues[1].trim().replace("+", "%2B")
        val decoded = runCatching {
            java.net.URLDecoder.decode(encoded, Charsets.UTF_8.name())
        }.getOrNull()
        if (!decoded.isNullOrBlank()) return decoded
    }
    // Не raw-строка: она кончалась бы четырьмя кавычками подряд — читается плохо
    // и сканер храповика литералов на этом теряет границу строки.
    Regex("filename\\s*=\\s*\"([^\"]*)\"", RegexOption.IGNORE_CASE).find(raw)?.let { m ->
        val name = m.groupValues[1].trim()
        if (name.isNotBlank()) return name
    }
    return null
}

/**
 * Имя, под которым скачанный файл ложится в кэш.
 *
 * Имя приходит от сервера, то есть из того, что когда-то загрузил человек, и
 * путём быть не должно: разделители схлопываются, как в
 * `TaskRepository.downloadAttachment`, а «..» целиком не имя, а шаг вверх по
 * дереву. Пустое имя заменяется на [fallback] — id строки: файл без имени
 * система открыть не даст.
 */
fun safeDownloadName(raw: String?, fallback: String): String {
    val name = raw.orEmpty().trim().replace(Regex("""[/\\]"""), "_")
    return if (name.isBlank() || name == "." || name == "..") fallback else name
}
