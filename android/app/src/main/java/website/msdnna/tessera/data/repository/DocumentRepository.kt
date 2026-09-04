package website.msdnna.tessera.data.repository

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.HttpException
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.CreateDocumentApprovalRequest
import website.msdnna.tessera.data.model.CreateDocumentCommentRequest
import website.msdnna.tessera.data.model.CreateDocumentRequest
import website.msdnna.tessera.data.model.CreateDocumentTaskLinkRequest
import website.msdnna.tessera.data.model.CreateDocumentTemplateRequest
import website.msdnna.tessera.data.model.CreateDocumentVersionRequest
import website.msdnna.tessera.data.model.DecideDocumentApprovalRequest
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.DocumentApproval
import website.msdnna.tessera.data.model.DocumentApprovalMode
import website.msdnna.tessera.data.model.DocumentApprovalStep
import website.msdnna.tessera.data.model.DocumentComment
import website.msdnna.tessera.data.model.DocumentContentSaved
import website.msdnna.tessera.data.model.DocumentConverterStatus
import website.msdnna.tessera.data.model.DocumentImportResult
import website.msdnna.tessera.data.model.DocumentTaskLink
import website.msdnna.tessera.data.model.DocumentTemplate
import website.msdnna.tessera.data.model.DocumentVersion
import website.msdnna.tessera.data.model.ResolveDocumentCommentRequest
import website.msdnna.tessera.data.model.TaskDocumentLink
import website.msdnna.tessera.data.model.UpdateDocumentCommentRequest
import website.msdnna.tessera.data.model.UpdateDocumentContentRequest
import website.msdnna.tessera.data.model.UpdateDocumentRequest
import website.msdnna.tessera.data.model.UpdateDocumentTemplateRequest

/**
 * The outcome of a content save.
 *
 * A stale [UpdateDocumentContentRequest.updatedAt] is answered with 409, which
 * Retrofit would otherwise raise as a generic [HttpException] — and "someone
 * else saved first" is not an error the caller should show as one. It is the
 * normal end of a race, so it gets a branch of its own carrying the timestamp
 * the server actually holds (#2894).
 */
sealed interface DocContentSave {
    data class Saved(val result: DocumentContentSaved) : DocContentSave

    /** [serverUpdatedAt] is the current server timestamp — refetch and merge from it. */
    data class Conflict(val serverUpdatedAt: String) : DocContentSave
}

/**
 * Documents (#2718). Read-only until #2894 — the editor now writes through here
 * too, so the surface mirrors the web `documents` API module rather than the two
 * calls the reader needed.
 */
class DocumentRepository {
    private val api get() = AppContainer.api()

    // ── Documents ─────────────────────────────────────────────────────────────
    suspend fun list(workspaceId: String, projectId: String? = null): List<Document> =
        api.documents(workspaceId, projectId).orEmpty()

    suspend fun get(id: String): Document = api.document(id)

    suspend fun bySlug(workspaceId: String, slug: String): Document = api.documentBySlug(workspaceId, slug)

    suspend fun create(
        workspaceId: String,
        title: String,
        icon: String = "",
        parentId: String? = null,
        projectId: String? = null,
        templateId: String? = null,
    ): Document = api.createDocument(
        workspaceId,
        CreateDocumentRequest(
            title = title,
            icon = icon,
            parentId = parentId,
            projectId = projectId,
            templateId = templateId,
        ),
    )

    suspend fun update(
        id: String,
        title: String? = null,
        icon: String? = null,
        parentId: String? = null,
        projectId: String? = null,
        position: Double? = null,
    ): Document = api.updateDocument(
        id,
        UpdateDocumentRequest(
            title = title,
            icon = icon,
            parentId = parentId,
            projectId = projectId,
            position = position,
        ),
    )

    /** [recursive] is required to delete a document that has children. */
    suspend fun delete(id: String, recursive: Boolean = false) =
        api.deleteDocument(id, if (recursive) true else null)

    suspend fun saveContent(
        id: String,
        content: JsonElement,
        updatedAt: String,
        connId: String = "",
    ): DocContentSave = try {
        DocContentSave.Saved(
            api.updateDocumentContent(id, UpdateDocumentContentRequest(content, updatedAt, connId)),
        )
    } catch (e: HttpException) {
        if (e.code() != HTTP_CONFLICT) throw e
        DocContentSave.Conflict(conflictTimestamp(e))
    }

    suspend fun uploadAsset(id: String, bytes: ByteArray, filename: String, mime: String?): String =
        api.uploadDocumentAsset(id, filePart(bytes, filename, mime)).url

    suspend fun uploadPdf(id: String, bytes: ByteArray, filename: String): JsonObject =
        api.uploadDocumentPdf(id, filePart(bytes, filename, "application/pdf"))

    /**
     * Pulls an asset into the cache and returns the file (#2894 §3).
     *
     * `PdfRenderer` reads through a [android.os.ParcelFileDescriptor] and does
     * random access over it, so a PDF has to become a real file before it can be
     * rendered at all — it cannot be handed a stream. The name is derived from
     * the URL rather than from the block's caption, which is the author's title
     * and may repeat across documents.
     *
     * A file already in the cache is reused: the asset is immutable (a new
     * upload gets a new name), and re-downloading a 12 MB scan on every scroll
     * back to the block is the difference between a reader and a progress bar.
     */
    suspend fun downloadAsset(cacheDir: java.io.File, src: String, cacheKey: String): java.io.File {
        val dir = java.io.File(cacheDir, "documents").apply { mkdirs() }
        val out = java.io.File(dir, cacheKey.replace(Regex("[^A-Za-z0-9._-]"), "_"))
        if (out.isFile && out.length() > 0) return out
        // Partial writes must not be mistaken for a cached file on the next open.
        val part = java.io.File(dir, out.name + ".part")
        api.downloadDocumentAsset(absoluteAssetUrl(src)).use { body ->
            body.byteStream().use { input -> part.outputStream().use { input.copyTo(it) } }
        }
        if (!part.renameTo(out)) {
            part.delete()
            // An IOException, so it reaches the user as «нет соединения» rather
            // than as a raw sentence (Errors.kt) — the cache is not something a
            // reader can act on.
            throw java.io.IOException("document asset cache write failed")
        }
        return out
    }

    /** The stored `src` is a path on our own origin; Retrofit's `@Url` needs it whole. */
    private fun absoluteAssetUrl(src: String): String = when {
        src.startsWith("http://") || src.startsWith("https://") -> src
        else -> website.msdnna.tessera.data.api.RetrofitClient.serverRoot + src
    }

    // ── Office import / export (#2733) ────────────────────────────────────────
    suspend fun converterStatus(): DocumentConverterStatus = api.documentConverterStatus()

    suspend fun import(workspaceId: String, bytes: ByteArray, filename: String, mime: String?): DocumentImportResult =
        api.importDocument(workspaceId, filePart(bytes, filename, mime))

    /** Reads the export whole: it is saved to a file or shared, never streamed on. */
    suspend fun export(id: String, format: String): ByteArray =
        api.exportDocument(id, format).use { it.bytes() }

    // ── Block comments (#2730) ────────────────────────────────────────────────
    suspend fun comments(id: String): List<DocumentComment> = api.documentComments(id).orEmpty()

    suspend fun addComment(
        id: String,
        body: String,
        blockId: String = "",
        parentId: String? = null,
        quote: String = "",
    ): DocumentComment = api.createDocumentComment(
        id,
        CreateDocumentCommentRequest(body = body, blockId = blockId, parentId = parentId, quote = quote),
    )

    suspend fun editComment(commentId: String, body: String): DocumentComment =
        api.updateDocumentComment(commentId, UpdateDocumentCommentRequest(body))

    suspend fun resolveComment(commentId: String, resolved: Boolean = true): DocumentComment =
        api.resolveDocumentComment(commentId, ResolveDocumentCommentRequest(resolved))

    suspend fun deleteComment(commentId: String) = api.deleteDocumentComment(commentId)

    // ── Version journal (#2731) ───────────────────────────────────────────────
    suspend fun versions(id: String): List<DocumentVersion> = api.documentVersions(id).orEmpty()

    suspend fun snapshot(id: String, label: String = ""): DocumentVersion =
        api.createDocumentVersion(id, CreateDocumentVersionRequest(label))

    suspend fun version(versionId: String): DocumentVersion = api.documentVersion(versionId)

    suspend fun restoreVersion(versionId: String): Document = api.restoreDocumentVersion(versionId)

    // ── Template gallery (#2734) ──────────────────────────────────────────────
    suspend fun templates(workspaceId: String): List<DocumentTemplate> =
        api.documentTemplates(workspaceId).orEmpty()

    suspend fun createTemplate(
        workspaceId: String,
        title: String,
        description: String = "",
        icon: String = "",
        content: JsonElement? = null,
        documentId: String? = null,
    ): DocumentTemplate = api.createDocumentTemplate(
        workspaceId,
        CreateDocumentTemplateRequest(
            title = title,
            description = description,
            icon = icon,
            content = content,
            documentId = documentId,
        ),
    )

    suspend fun template(templateId: String): DocumentTemplate = api.documentTemplate(templateId)

    suspend fun updateTemplate(
        templateId: String,
        title: String? = null,
        description: String? = null,
        icon: String? = null,
    ): DocumentTemplate =
        api.updateDocumentTemplate(templateId, UpdateDocumentTemplateRequest(title, description, icon))

    suspend fun deleteTemplate(templateId: String) = api.deleteDocumentTemplate(templateId)

    // ── Task links and approvals (#2732) ──────────────────────────────────────
    suspend fun taskLinks(id: String): List<DocumentTaskLink> = api.documentTaskLinks(id).orEmpty()

    suspend fun linkTask(id: String, taskId: String, blockId: String = "", quote: String = ""): DocumentTaskLink =
        api.createDocumentTaskLink(id, CreateDocumentTaskLinkRequest(taskId = taskId, blockId = blockId, quote = quote))

    suspend fun unlinkTask(linkId: String) = api.deleteDocumentTaskLink(linkId)

    /** The same link table from the task side — for the task modal's Документы tab. */
    suspend fun documentsOfTask(taskId: String): List<TaskDocumentLink> =
        api.taskDocumentLinks(taskId).orEmpty()

    suspend fun approvals(id: String): List<DocumentApproval> = api.documentApprovals(id).orEmpty()

    suspend fun createApproval(
        id: String,
        approvers: List<String>,
        title: String = "",
        mode: String = DocumentApprovalMode.SEQUENTIAL,
    ): DocumentApproval =
        api.createDocumentApproval(id, CreateDocumentApprovalRequest(approvers = approvers, title = title, mode = mode))

    suspend fun decideApproval(
        approvalId: String,
        decision: String,
        comment: String = "",
        signature: String = "",
    ): DocumentApprovalStep =
        api.decideDocumentApproval(approvalId, DecideDocumentApprovalRequest(decision, comment, signature))

    suspend fun cancelApproval(approvalId: String): DocumentApproval = api.cancelDocumentApproval(approvalId)

    private fun filePart(bytes: ByteArray, filename: String, mime: String?): MultipartBody.Part {
        val media = (mime ?: "application/octet-stream").toMediaTypeOrNull()
        return MultipartBody.Part.createFormData("file", filename, bytes.toRequestBody(media))
    }

    private fun conflictTimestamp(e: HttpException): String =
        parseDocConflictTimestamp(e.response()?.errorBody()?.string())

    private companion object {
        const val HTTP_CONFLICT = 409
    }
}

/**
 * Digs the server's timestamp out of a 409 body (`{"error":…,"updated_at":…}`).
 *
 * An empty string is a valid answer: it means "we could not learn the current
 * timestamp", and the caller refetches the document instead of trusting a value
 * it does not have. Parsing must not throw — a malformed error body would
 * otherwise turn a lost race into a crash, which is the one moment the editor
 * still holds text nobody else has.
 */
internal fun parseDocConflictTimestamp(body: String?): String = runCatching {
    JsonParser.parseString(body.orEmpty()).asJsonObject.get("updated_at").asString
}.getOrDefault("")
