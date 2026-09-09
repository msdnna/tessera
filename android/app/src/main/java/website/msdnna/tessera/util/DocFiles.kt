package website.msdnna.tessera.util

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import website.msdnna.tessera.data.model.DocumentConverterStatus
import website.msdnna.tessera.data.model.DocumentTemplate

// Files going in and out of the documents section (#2733, #2734 — §8 of #2894):
// which picked file goes down which road, what an export is called, and how the
// template gallery is put together.
//
// All of it is line-by-line the web's rules (`docImport.js`, `docOffice.js`,
// `docTemplates.js`) rather than a phone-specific variant — the two clients
// upload to the same endpoints and would otherwise disagree about which file the
// server will accept, which the user only finds out *after* the file picker.

/** Read on the phone, no converter needed — the same two the web parses itself. */
val DOC_LOCAL_EXTENSIONS = listOf(".md", ".markdown", ".json")

/** Sent to `POST /workspaces/:id/documents/import`. Kept in step with
 *  `docImportExts` in backend/handlers/document_convert.go. */
val DOC_OFFICE_EXTENSIONS = listOf(".doc", ".docx", ".odt", ".rtf", ".fodt", ".html", ".htm", ".txt")

/** Travels the import endpoint too, but is stored rather than converted — so it
 *  works on an install with no sidecar (see docPdf.js). */
const val DOC_PDF_EXTENSION = ".pdf"

/** Matches `MAX_IMPORT_BYTES`: the server caps a body at 4 MiB and Markdown
 *  expands when parsed, so a bigger file is refused before it is read. */
const val MAX_DOC_LOCAL_BYTES = 2L * 1024 * 1024

/** Matches `maxDocImportBytes` server-side. */
const val MAX_DOC_OFFICE_BYTES = 20L * 1024 * 1024

/** What happens to a file the user picked. */
enum class DocImportRoute {
    /** `.md` / `.json` — parsed here, then created and saved like any document. */
    LOCAL,

    /** An office format — uploaded, converted server-side, parsed by the editor. */
    OFFICE,

    /** A PDF — uploaded and stored as a block; never reaches the converter. */
    PDF,

    /** Nothing we can do with it. */
    UNSUPPORTED,
}

/** The road a picked file takes, decided by its name alone (that is all the
 *  picker gives us before the bytes are read). */
fun docImportRoute(fileName: String): DocImportRoute {
    val name = fileName.lowercase()
    return when {
        name.endsWith(DOC_PDF_EXTENSION) -> DocImportRoute.PDF
        DOC_LOCAL_EXTENSIONS.any { name.endsWith(it) } -> DocImportRoute.LOCAL
        DOC_OFFICE_EXTENSIONS.any { name.endsWith(it) } -> DocImportRoute.OFFICE
        else -> DocImportRoute.UNSUPPORTED
    }
}

/** The size ceiling for a file on [route], in bytes. */
fun docImportLimit(route: DocImportRoute): Long =
    if (route == DocImportRoute.LOCAL) MAX_DOC_LOCAL_BYTES else MAX_DOC_OFFICE_BYTES

/**
 * Whether the picked file needs the LibreOffice sidecar to be deployed.
 *
 * PDF and the two locally-parsed formats do not, which is the whole point of
 * asking: the import button stays useful on an install without a converter
 * instead of being hidden or failing after the file dialog.
 */
fun docImportNeedsConverter(fileName: String): Boolean =
    docImportRoute(fileName) == DocImportRoute.OFFICE

/** MIME types for the system picker. The wildcard is deliberate: providers
 *  disagree about `.docx`'s type, and a stricter filter greys out files the
 *  server would have accepted. */
val DOC_IMPORT_MIME_TYPES = arrayOf("*/*")

/**
 * The formats offered for export.
 *
 * `html` is always in the list even when the sidecar is down: it is rendered by
 * the backend itself, so the cheapest export never depends on the heaviest
 * dependency.
 */
fun docExportFormats(status: DocumentConverterStatus?): List<String> {
    val offered = status?.exportFormats.orEmpty().filter { it.isNotBlank() }
    return if (offered.isEmpty()) listOf("html") else offered
}

/** Human label for an export format — `EXPORT_LABELS` on the web. */
fun docExportLabel(format: String): String = when (format.lowercase()) {
    "pdf" -> "PDF"
    "docx" -> "Word (.docx)"
    "odt" -> "OpenDocument (.odt)"
    "html" -> "HTML"
    else -> format.uppercase()
}

/**
 * File name for an export.
 *
 * Only the characters that break a name on some platform are replaced —
 * Cyrillic and spaces are fine, and stripping them would mangle every title.
 * [fallback] covers a document whose title is blank: a file called `.pdf` is
 * invisible in a downloads folder.
 */
fun docExportFileName(title: String, format: String, fallback: String): String {
    val base = title.trim().ifEmpty { fallback }
    return base.replace(Regex("[\\\\/:*?\"<>|]"), "_") + "." + format
}

/** A draft read out of a picked `.md` / `.json` file. */
data class DocFileDraft(
    val title: String,
    val content: JsonElement,
    val icon: String = "",
    val description: String = "",
)

/** A file that is not a document body this app can read. */
class DocFileFormatException : Exception()

/**
 * Reads a `.md` or `.json` file into a draft.
 *
 * The JSON branch accepts both a bare body (`{"type":"doc"}`) and the envelope
 * the gallery exports (`{title, content}`) — refusing to read back our own
 * export file would be a trap.
 *
 * @throws DocFileFormatException when the bytes are not a body we can open
 */
fun parseDocFile(fileName: String, text: String): DocFileDraft {
    val dot = fileName.lastIndexOf('.')
    val fallbackTitle = if (dot > 0) fileName.substring(0, dot) else fileName
    if (!fileName.lowercase().endsWith(".json")) {
        return DocFileDraft(
            title = firstMarkdownHeading(text).ifBlank { fallbackTitle },
            content = markdownToDocJson(text),
        )
    }
    val parsed = runCatching { JsonParser.parseString(text) }.getOrNull()
        ?: throw DocFileFormatException()
    val root = parsed as? JsonObject ?: throw DocFileFormatException()
    val body = when {
        root.get("type")?.asStringOrNull() == "doc" -> root
        root.get("content") is JsonObject -> root.getAsJsonObject("content")
        else -> throw DocFileFormatException()
    }
    if (body.get("type")?.asStringOrNull() != "doc") throw DocFileFormatException()
    return DocFileDraft(
        title = root.get("title")?.asStringOrNull().orEmpty().ifBlank { fallbackTitle },
        content = body,
        icon = root.get("icon")?.asStringOrNull().orEmpty(),
        description = root.get("description")?.asStringOrNull().orEmpty(),
    )
}

private fun JsonElement.asStringOrNull(): String? =
    runCatching { if (isJsonPrimitive) asString else null }.getOrNull()

// ── The gallery ───────────────────────────────────────────────────────────────

/**
 * The starters that ship with the app.
 *
 * Constants rather than rows seeded per workspace, for the reason spelled out in
 * `docTemplates.js`: a seeded row ages independently in every workspace and
 * cannot be improved without a data migration. Titles, descriptions and bodies
 * live in the string resources — a skeleton is text the app writes for the
 * reader, so an English document must not open with Russian headings.
 */
enum class DocBuiltinTemplate(val key: String, val icon: String) {
    MEETING("meeting", "📋"),
    SPEC("spec", "📐"),
    RETRO("retro", "🔄"),
}

/** One tile of the gallery — a saved template or a built-in, told apart only by
 *  [builtin], since a card does not otherwise care where it came from. */
data class DocTemplateCard(
    val id: String,
    val title: String,
    val description: String,
    val icon: String,
    /** Set for a saved template; the built-ins say so in their own word. */
    val authorName: String = "",
    val builtin: Boolean = false,
    /** The built-in's key — what [DocBuiltinTemplate] the tile stands for. */
    val builtinKey: String = "",
)

/** Gallery card for a template saved in the workspace. */
fun templateCard(template: DocumentTemplate): DocTemplateCard = DocTemplateCard(
    id = template.id,
    title = template.title,
    description = template.description.ifBlank { template.preview.replace("\n", " ").trim() },
    icon = template.icon,
    authorName = template.authorName.orEmpty(),
)

/** Gallery card for a built-in, with its localized text handed in. */
fun builtinTemplateCard(
    template: DocBuiltinTemplate,
    title: String,
    description: String,
): DocTemplateCard = DocTemplateCard(
    id = "builtin:${template.key}",
    title = title,
    description = description,
    icon = template.icon,
    builtin = true,
    builtinKey = template.key,
)

/**
 * The gallery, saved templates first.
 *
 * The order is the web's and it is not alphabetical on purpose: the team's own
 * templates are what they came for, and pushing them below three fixed starters
 * would make the gallery look like it belongs to the app rather than to them.
 */
fun docTemplateGallery(
    saved: List<DocumentTemplate>,
    builtins: List<DocTemplateCard>,
): List<DocTemplateCard> = saved.map(::templateCard) + builtins

/** Filters the gallery by the search box — title and description, as the web. */
fun filterDocTemplates(cards: List<DocTemplateCard>, query: String): List<DocTemplateCard> {
    val q = query.trim().lowercase()
    if (q.isEmpty()) return cards
    return cards.filter { "${it.title} ${it.description}".lowercase().contains(q) }
}
