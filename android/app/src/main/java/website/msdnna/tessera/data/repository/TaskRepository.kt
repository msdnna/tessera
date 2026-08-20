package website.msdnna.tessera.data.repository

import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.AddRelationRequest
import website.msdnna.tessera.data.model.Attachment
import website.msdnna.tessera.data.model.CommandPreview
import website.msdnna.tessera.data.model.Comment
import website.msdnna.tessera.data.model.CommentResult
import website.msdnna.tessera.data.model.CreateCommentRequest
import website.msdnna.tessera.data.model.PreviewCommandsRequest
import website.msdnna.tessera.data.model.Relation
import website.msdnna.tessera.data.model.TaskDetail
import website.msdnna.tessera.data.model.TaskEvent
import website.msdnna.tessera.data.model.UpdateCommentRequest

/**
 * Reads a single task's rich detail (comments, relations, attachments, journal)
 * and performs the modal's collab mutations. The card-level edits (priority,
 * due, tags, …) stay on [BoardRepository], which the modal reuses via the board
 * view-model.
 */
class TaskRepository {
    private val api get() = AppContainer.api()

    suspend fun detail(taskId: String): TaskDetail = api.task(taskId)

    suspend fun comments(taskId: String): List<Comment> = api.comments(taskId).orEmpty()
    suspend fun addComment(
        taskId: String,
        body: String,
        mentions: List<String>,
        parentId: String? = null,
    ): CommentResult = api.createComment(taskId, CreateCommentRequest(body, mentions, parentId))

    /** Dry-runs the draft against the backend's own parser: what each `/`-command
     *  in it would do, without executing anything. */
    suspend fun previewCommands(taskId: String, body: String): CommandPreview =
        api.previewCommands(taskId, PreviewCommandsRequest(body))
    suspend fun editComment(commentId: String, body: String): Comment =
        api.updateComment(commentId, UpdateCommentRequest(body))
    suspend fun deleteComment(commentId: String) = api.deleteComment(commentId)

    suspend fun relations(taskId: String): List<Relation> = api.relations(taskId).orEmpty()
    suspend fun addRelation(taskId: String, number: Long, kind: String) =
        api.addRelation(taskId, AddRelationRequest(number, kind))
    suspend fun deleteRelation(relationId: String) = api.deleteRelation(relationId)

    suspend fun attachments(taskId: String): List<Attachment> = api.attachments(taskId).orEmpty()
    suspend fun uploadAttachment(taskId: String, bytes: ByteArray, filename: String, mime: String?): Attachment =
        api.uploadAttachment(taskId, filePart(bytes, filename, mime))
    suspend fun deleteAttachment(attachmentId: String) = api.deleteAttachment(attachmentId)

    suspend fun events(taskId: String): List<TaskEvent> = api.events(taskId).orEmpty()

    /** Moves the task (and its subtasks) to another board's first/given column. */
    suspend fun transfer(taskId: String, boardId: String, columnId: String? = null) =
        api.transferTask(taskId, website.msdnna.tessera.data.model.TransferTaskRequest(boardId, columnId))

    /** Workspace tasks for the relation autocomplete (cross-board). Includes
     *  subtasks so a subtask can be picked as a blocking relation (web parity). */
    suspend fun workspaceTasks(workspaceId: String): List<website.msdnna.tessera.data.model.WorkspaceTask> =
        api.workspaceTasks(workspaceId, includeSubtasks = 1).orEmpty()

    /** The task a «#N» link names, or null when the workspace has no such number. */
    suspend fun taskByNumber(workspaceId: String, number: Int): website.msdnna.tessera.data.model.Task? =
        runCatching { api.taskByNumber(workspaceId, number) }.getOrNull()

    /** Downloads an attachment's bytes (auth'd) to a cache file, returning it. */
    suspend fun downloadAttachment(cacheDir: java.io.File, attachmentId: String, filename: String): java.io.File {
        val body = api.downloadAttachment(attachmentId)
        val dir = java.io.File(cacheDir, "attachments").apply { mkdirs() }
        val safe = filename.ifBlank { attachmentId }.replace(Regex("[/\\\\]"), "_")
        val out = java.io.File(dir, safe)
        body.byteStream().use { input -> out.outputStream().use { input.copyTo(it) } }
        return out
    }

    /** Uploads an inline image and returns its public `/api/uploads/…` URL. */
    suspend fun uploadMedia(bytes: ByteArray, filename: String, mime: String?): String =
        api.uploadMedia(filePart(bytes, filename, mime)).url

    private fun filePart(bytes: ByteArray, filename: String, mime: String?): MultipartBody.Part {
        val media = (mime ?: "application/octet-stream").toMediaTypeOrNull()
        return MultipartBody.Part.createFormData("file", filename, bytes.toRequestBody(media))
    }
}
