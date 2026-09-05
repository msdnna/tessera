package website.msdnna.tessera.data.model

import com.google.gson.JsonElement
import com.google.gson.annotations.SerializedName

/**
 * A workspace document (#2718). Mirrors the backend `Document`
 * (`GET /workspaces/:id/documents`, `GET /documents/:id`).
 *
 * One model for both calls, as on the server: the list query simply omits
 * `content`, so it arrives null and the body is fetched when a document is
 * opened. [content] is the raw ProseMirror tree — see [website.msdnna.tessera.util.parseDocBlocks].
 */
data class Document(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("project_id") val projectId: String? = null,
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("slug") val slug: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("preview") val preview: String = "",
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("content") val content: JsonElement? = null,
)

/**
 * `POST /workspaces/:id/documents`. Passing [templateId] is how a document is
 * created *from* a template — the server has no separate endpoint for it, so
 * slug, position and authorship stay on one path.
 */
data class CreateDocumentRequest(
    @SerializedName("title") val title: String,
    @SerializedName("icon") val icon: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("project_id") val projectId: String? = null,
    @SerializedName("template_id") val templateId: String? = null,
)

/**
 * `PATCH /documents/:id` — metadata only; the body has its own endpoint.
 *
 * Every field is optional on the server, and Gson omits nulls, so an absent
 * field means "leave it alone". Clearing a parent (moving a document to the
 * root) is therefore not expressible here — the web client has the same
 * limitation, because the Go side reads absent and null the same way.
 */
data class UpdateDocumentRequest(
    @SerializedName("title") val title: String? = null,
    @SerializedName("icon") val icon: String? = null,
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("project_id") val projectId: String? = null,
    @SerializedName("position") val position: Double? = null,
)

/**
 * `PATCH /documents/:id/content`.
 *
 * [updatedAt] is the timestamp the client last saw: the server answers 409 when
 * it no longer matches, instead of silently overwriting someone else's edit.
 * [connId] is the document socket this save came from, so the save is announced
 * to the rest of the room but not back to us (#2729).
 */
data class UpdateDocumentContentRequest(
    @SerializedName("content") val content: JsonElement,
    @SerializedName("updated_at") val updatedAt: String,
    @SerializedName("conn_id") val connId: String = "",
)

/** The 200 of a content save — deliberately not the whole document. */
data class DocumentContentSaved(
    @SerializedName("id") val id: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("preview") val preview: String = "",
)

/** The 201 of `POST /documents/:id/assets` — a signed URL for the image. */
data class DocumentAssetUploaded(
    @SerializedName("url") val url: String = "",
)

/**
 * A comment anchored to a block (#2730). Roots and replies arrive in one list
 * and are threaded on the client: [parentId] null is a root, and a reply always
 * carries the root's [blockId].
 */
data class DocumentComment(
    @SerializedName("id") val id: String = "",
    @SerializedName("document_id") val documentId: String = "",
    @SerializedName("block_id") val blockId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("body") val body: String = "",
    @SerializedName("quote") val quote: String = "",
    @SerializedName("resolved_at") val resolvedAt: String? = null,
    @SerializedName("resolved_by") val resolvedBy: String? = null,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    // Joined in by the list query only; a create/update response carries the raw
    // row without them.
    @SerializedName("author_name") val authorName: String? = null,
    @SerializedName("author_email") val authorEmail: String? = null,
) {
    val isRoot: Boolean get() = parentId == null
    val isResolved: Boolean get() = resolvedAt != null
}

data class CreateDocumentCommentRequest(
    @SerializedName("body") val body: String,
    @SerializedName("block_id") val blockId: String = "",
    @SerializedName("parent_id") val parentId: String? = null,
    @SerializedName("quote") val quote: String = "",
)

data class UpdateDocumentCommentRequest(
    @SerializedName("body") val body: String,
)

data class ResolveDocumentCommentRequest(
    @SerializedName("resolved") val resolved: Boolean = true,
)

/**
 * One entry of the version journal (#2731).
 *
 * [content] is null in the list on purpose — a document is up to a megabyte of
 * ProseMirror JSON and the journal shows fifty entries, so a body arrives only
 * from `GET /document-versions/:id`.
 */
data class DocumentVersion(
    @SerializedName("id") val id: String = "",
    @SerializedName("document_id") val documentId: String = "",
    @SerializedName("revision") val revision: Int = 0,
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("preview") val preview: String = "",
    @SerializedName("label") val label: String = "",
    @SerializedName("manual") val manual: Boolean = false,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("author_name") val authorName: String? = null,
    @SerializedName("author_email") val authorEmail: String? = null,
    @SerializedName("content") val content: JsonElement? = null,
)

data class CreateDocumentVersionRequest(
    @SerializedName("label") val label: String = "",
)

/**
 * A template from the workspace gallery (#2734). Like the version journal the
 * list carries [preview] rather than [content]; a body is fetched only when a
 * template is previewed.
 */
data class DocumentTemplate(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("author_id") val authorId: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("preview") val preview: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    @SerializedName("author_name") val authorName: String? = null,
    @SerializedName("content") val content: JsonElement? = null,
)

/**
 * `POST /workspaces/:id/document-templates`. Either [content] or [documentId] —
 * the latter snapshots an existing document into a template.
 */
data class CreateDocumentTemplateRequest(
    @SerializedName("title") val title: String,
    @SerializedName("description") val description: String = "",
    @SerializedName("icon") val icon: String = "",
    @SerializedName("content") val content: JsonElement? = null,
    @SerializedName("document_id") val documentId: String? = null,
)

data class UpdateDocumentTemplateRequest(
    @SerializedName("title") val title: String? = null,
    @SerializedName("description") val description: String? = null,
    @SerializedName("icon") val icon: String? = null,
)

/**
 * A document→task link read from the document side (#2732). The task fields are
 * joined in so the panel renders a row without a request per link.
 */
data class DocumentTaskLink(
    @SerializedName("id") val id: String = "",
    @SerializedName("document_id") val documentId: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("block_id") val blockId: String = "",
    @SerializedName("quote") val quote: String = "",
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("task_title") val taskTitle: String = "",
    @SerializedName("task_board_id") val taskBoardId: String = "",
    @SerializedName("task_priority") val taskPriority: Int = 0,
    @SerializedName("task_completed_at") val taskCompletedAt: String? = null,
)

/** The same link table read from the task side, for the task modal's Документы tab. */
data class TaskDocumentLink(
    @SerializedName("id") val id: String = "",
    @SerializedName("document_id") val documentId: String = "",
    @SerializedName("task_id") val taskId: String = "",
    @SerializedName("block_id") val blockId: String = "",
    @SerializedName("quote") val quote: String = "",
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("document_title") val documentTitle: String = "",
    @SerializedName("document_icon") val documentIcon: String = "",
    @SerializedName("document_slug") val documentSlug: String = "",
    @SerializedName("document_workspace_id") val documentWorkspaceId: String = "",
)

data class CreateDocumentTaskLinkRequest(
    @SerializedName("task_id") val taskId: String,
    @SerializedName("block_id") val blockId: String = "",
    @SerializedName("quote") val quote: String = "",
)

/**
 * An approval route over a pinned snapshot (#2732).
 *
 * [steps] and the joined [versionRevision]/[createdByName] come from the list
 * only: creating or cancelling a route answers with the bare row, so both
 * default rather than being required.
 */
data class DocumentApproval(
    @SerializedName("id") val id: String = "",
    @SerializedName("document_id") val documentId: String = "",
    @SerializedName("version_id") val versionId: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("status") val status: String = "",
    @SerializedName("mode") val mode: String = "",
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("closed_at") val closedAt: String? = null,
    @SerializedName("version_revision") val versionRevision: Int = 0,
    @SerializedName("created_by_name") val createdByName: String? = null,
    @SerializedName("steps") val steps: List<DocumentApprovalStep> = emptyList(),
) {
    val isOpen: Boolean get() = status == DocumentApprovalStatus.PENDING
}

/** One approver's slot on a route; [decidedAt] null means they have not signed. */
data class DocumentApprovalStep(
    @SerializedName("id") val id: String = "",
    @SerializedName("approval_id") val approvalId: String = "",
    @SerializedName("approver_id") val approverId: String? = null,
    @SerializedName("approver_name") val approverName: String = "",
    @SerializedName("position") val position: Int = 0,
    @SerializedName("status") val status: String = "",
    @SerializedName("comment") val comment: String = "",
    @SerializedName("signature") val signature: String = "",
    @SerializedName("decided_at") val decidedAt: String? = null,
    @SerializedName("approver_email") val approverEmail: String? = null,
)

/** Status values shared by a route and its steps — the server rejects others. */
object DocumentApprovalStatus {
    const val PENDING = "pending"
    const val APPROVED = "approved"
    const val REJECTED = "rejected"
    const val CANCELLED = "cancelled"
}

/** Route modes: everyone signs at once, or in [DocumentApprovalStep.position] order. */
object DocumentApprovalMode {
    const val PARALLEL = "parallel"
    const val SEQUENTIAL = "sequential"
}

data class CreateDocumentApprovalRequest(
    @SerializedName("approvers") val approvers: List<String>,
    @SerializedName("title") val title: String = "",
    @SerializedName("mode") val mode: String = DocumentApprovalMode.SEQUENTIAL,
)

/** [decision] is [DocumentApprovalStatus.APPROVED] or [DocumentApprovalStatus.REJECTED]. */
data class DecideDocumentApprovalRequest(
    @SerializedName("decision") val decision: String,
    @SerializedName("comment") val comment: String = "",
    @SerializedName("signature") val signature: String = "",
)

/**
 * `GET /document-converter` (#2733).
 *
 * [nativeFormats] comes back even when [available] is false: PDF import needs no
 * sidecar, so hiding import entirely on `available:false` would hide a feature
 * that works.
 */
data class DocumentConverterStatus(
    @SerializedName("available") val available: Boolean = false,
    @SerializedName("reason") val reason: String = "",
    @SerializedName("import_formats") val importFormats: List<String> = emptyList(),
    @SerializedName("native_formats") val nativeFormats: List<String> = emptyList(),
    @SerializedName("export_formats") val exportFormats: List<String> = emptyList(),
)

/**
 * The 201 of an office import.
 *
 * The body comes back as [html] rather than as blocks: the client parses it with
 * the editor's schema and saves it through the ordinary content endpoint, so an
 * import is validated by exactly the same code as typing.
 *
 * A PDF takes the same endpoint but is *stored* rather than converted, so it
 * arrives as [pdf] with [html] empty — that branch is what makes importing a
 * PDF work on an install with no converter deployed.
 */
data class DocumentImportResult(
    @SerializedName("document") val document: Document = Document(),
    @SerializedName("html") val html: String = "",
    @SerializedName("pdf") val pdf: JsonElement? = null,
    @SerializedName("images_dropped") val imagesDropped: Int = 0,
    @SerializedName("images_dropped_reason") val imagesDroppedReason: String = "",
    @SerializedName("source_file_name") val sourceFileName: String = "",
    @SerializedName("page") val page: JsonElement? = null,
    @SerializedName("sections_differ") val sectionsDiffer: Boolean = false,
)
