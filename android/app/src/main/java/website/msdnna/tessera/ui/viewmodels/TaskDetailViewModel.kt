package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import java.io.File
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Attachment
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Comment
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Recurrence
import website.msdnna.tessera.data.model.Relation
import website.msdnna.tessera.data.model.TaskDetail
import website.msdnna.tessera.data.model.TaskEvent
import website.msdnna.tessera.data.model.WorkspaceTask
import website.msdnna.tessera.data.repository.BoardRepository
import website.msdnna.tessera.data.repository.TaskRepository
import website.msdnna.tessera.util.MoveNeighbors
import website.msdnna.tessera.util.errorMessage

private val TagPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96",
)

data class TaskDetailUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val detail: TaskDetail? = null,
    val comments: List<Comment> = emptyList(),
    val relations: List<Relation> = emptyList(),
    val attachments: List<Attachment> = emptyList(),
    val events: List<TaskEvent> = emptyList(),
    /** The task's board columns — for the status row, recurrence selects and
     *  the subtask column chips. */
    val columns: List<BoardColumn> = emptyList(),
    /** `boards.done_column_id` of the task's board — target of the status «close»
     *  check. Null when the board has none (fall back to the completed flag). */
    val doneColumnId: String? = null,
    /** A column move is in flight — the status row disables itself meanwhile. */
    val moving: Boolean = false,
    /** Cross-board task candidates for the relation autocomplete (lazy-loaded). */
    val relationCandidates: List<WorkspaceTask> = emptyList(),
    /** True once any mutation happened, so the host can refresh the board on close. */
    val changed: Boolean = false,
)

/**
 * Owns one opened task's detail (the modal). Card-level fields go through
 * [BoardRepository] (shared semantics with the kanban card); the rich
 * comments/relations/files/journal go through [TaskRepository]. Every mutation
 * reloads the affected slice and flips [TaskDetailUiState.changed] so the board
 * can refresh when the modal closes.
 */
class TaskDetailViewModel(
    private val taskRepo: TaskRepository = TaskRepository(),
    private val boardRepo: BoardRepository = BoardRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(TaskDetailUiState())
    val state: StateFlow<TaskDetailUiState> = _state.asStateFlow()

    private var taskId: String = ""
    private var workspaceId: String = ""
    private var projectId: String = "" // the task's project — scopes new tags

    fun load(taskId: String, workspaceId: String, projectId: String) {
        this.taskId = taskId
        this.workspaceId = workspaceId
        this.projectId = projectId
        _state.update { TaskDetailUiState(loading = true) }
        launchCatching {
            val detail = taskRepo.detail(taskId)
            _state.update { it.copy(loading = false, detail = detail) }
            // Board columns for the status row / recurrence selects, and the board
            // itself for its done column (both best-effort). Read from the task's
            // own board id, so a task opened from the relations tab (possibly on
            // another board) still gets its own columns.
            _state.update {
                it.copy(
                    columns = runCatching { boardRepo.columns(detail.boardId) }.getOrDefault(emptyList()),
                    doneColumnId = runCatching { boardRepo.board(detail.boardId) }.getOrNull()?.doneColumnId,
                )
            }
            // Collab lists are best-effort and independent — a failure of one
            // shouldn't blank the modal.
            _state.update { it.copy(comments = runCatching { taskRepo.comments(taskId) }.getOrDefault(emptyList())) }
            _state.update { it.copy(relations = runCatching { taskRepo.relations(taskId) }.getOrDefault(emptyList())) }
            _state.update { it.copy(attachments = runCatching { taskRepo.attachments(taskId) }.getOrDefault(emptyList())) }
            _state.update { it.copy(events = runCatching { taskRepo.events(taskId) }.getOrDefault(emptyList())) }
        }
    }

    private suspend fun reloadDetail() {
        val detail = taskRepo.detail(taskId)
        _state.update { it.copy(detail = detail, changed = true) }
    }

    // ── card-level fields ────────────────────────────────────────────────────

    fun saveCore(title: String, description: String) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.updateTask(d.asTask(), title = title.trim().ifBlank { d.title }, description = description)
        reloadDetail()
    }

    fun saveDescription(description: String) = mutate {
        val d = state.value.detail ?: return@mutate
        if (description == d.description) return@mutate
        boardRepo.updateTask(d.asTask(), description = description)
        reloadDetail()
    }

    fun setPriority(priority: Int) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.updateTask(d.asTask(), priority = priority)
        reloadDetail()
    }

    fun setDue(dueIso: String?) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.updateTask(d.asTask(), dueDate = dueIso)
        reloadDetail()
    }

    /** Sets the due date and recurrence rule together (the due popover commits both). */
    fun setDueAndRecurrence(dueIso: String?, startIso: String?, recurrence: Recurrence?) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.updateTask(d.asTask(), dueDate = dueIso, startDate = startIso, recurrence = recurrence)
        reloadDetail()
    }

    fun setDueNotify(lead: Int?, repeat: Int?, enabled: Boolean?) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.setDueNotify(d.id, lead, repeat, enabled)
        reloadDetail()
    }

    fun setCompleted(completed: Boolean) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.updateTask(d.asTask(), completed = completed)
        reloadDetail()
    }

    // ── status (board column) ─────────────────────────────────────────────────
    // Moving is `PATCH /tasks/:id/move`, so the backend does the completed/reopened
    // bookkeeping and the journal entry for us. [neighbours] pins the landing slot
    // (see util/Status.kt — bare nulls would drop the card near the top).

    /** Moves this task to another column of its board. */
    fun moveToColumn(columnId: String, neighbours: MoveNeighbors) = mutate {
        val d = state.value.detail ?: return@mutate
        if (columnId.isBlank() || columnId == d.columnId) return@mutate
        _state.update { it.copy(moving = true) }
        try {
            boardRepo.moveTask(d.id, columnId, neighbours.beforeId, neighbours.afterId)
            // A recurring task completed by entering the done column bounces
            // straight back out with an advanced due date — re-read instead of
            // trusting the click.
            reloadDetail()
        } finally {
            _state.update { it.copy(moving = false) }
        }
    }

    /** The same move for a subtask row, without opening it. */
    fun moveSubtask(subId: String, columnId: String, neighbours: MoveNeighbors) = mutate {
        val sub = state.value.detail?.subtasks?.find { it.id == subId } ?: return@mutate
        if (columnId.isBlank() || columnId == sub.columnId) return@mutate
        boardRepo.moveTask(subId, columnId, neighbours.beforeId, neighbours.afterId)
        reloadDetail()
    }

    /** Sets the task's estimate (canonical value), or null to clear it. */
    fun setEstimate(value: Double?) = mutate {
        val d = state.value.detail ?: return@mutate
        boardRepo.updateTask(d.asTask(), estimate = value)
        reloadDetail()
    }

    fun toggleTag(tagId: String) = mutate {
        val d = state.value.detail ?: return@mutate
        if (d.tags.any { it.id == tagId }) boardRepo.removeTag(d.id, tagId) else boardRepo.addTag(d.id, tagId)
        reloadDetail()
    }

    /** Creates a tag with a random palette colour and attaches it; returns it via reload. */
    fun createTagAndAdd(name: String, onTagsChanged: () -> Unit) = mutate {
        val d = state.value.detail ?: return@mutate
        val tag = boardRepo.createTag(projectId, name.trim(), TagPalette.random())
        boardRepo.addTag(d.id, tag.id)
        reloadDetail()
        onTagsChanged()
    }

    fun toggleAssignee(userId: String) = mutate {
        val d = state.value.detail ?: return@mutate
        if (d.assignees.any { it.id == userId }) boardRepo.removeAssignee(d.id, userId) else boardRepo.addAssignee(d.id, userId)
        reloadDetail()
    }

    // Assign/unassign a GitLab project member (may have no Tessera account); the
    // backend mirrors it to the issue when push_assignees is on.
    fun toggleGitlabAssignee(m: website.msdnna.tessera.data.model.GitlabMember) = mutate {
        val d = state.value.detail ?: return@mutate
        if (d.gitlabAssignees.any { it.glUsername == m.glUsername }) boardRepo.removeGitlabAssignee(d.id, m.glUsername)
        else boardRepo.pinGitlabAssignee(d.id, m)
        reloadDetail()
    }

    /** Assign (non-null) or clear (null) the task's milestone. */
    fun setMilestone(milestoneId: String?) = mutate {
        boardRepo.setTaskMilestone(taskId, milestoneId)
        reloadDetail()
    }

    /** Inline-create a milestone on the task's project and assign it; [onCreated]
     *  receives the new milestone so the picker can show it immediately. */
    fun createMilestoneAndAssign(
        title: String,
        onCreated: (website.msdnna.tessera.data.model.Milestone) -> Unit,
    ) = mutate {
        if (title.isBlank() || projectId.isBlank()) return@mutate
        val m = boardRepo.createMilestone(projectId, title)
        boardRepo.setTaskMilestone(taskId, m.id)
        onCreated(m)
        reloadDetail()
    }

    fun attachToParent(parentId: String) = mutate {
        boardRepo.setParent(taskId, parentId)
        reloadDetail()
    }

    fun detachFromParent() = mutate {
        boardRepo.setParent(taskId, null)
        reloadDetail()
    }

    fun addSubtask(columnId: String, title: String) = mutate {
        boardRepo.createTask(state.value.detail?.boardId ?: return@mutate, columnId, title, parentId = taskId)
        reloadDetail()
    }

    fun toggleSubtaskDone(subId: String, completed: Boolean) = mutate {
        val sub = state.value.detail?.subtasks?.find { it.id == subId } ?: return@mutate
        boardRepo.updateTask(sub, completed = completed)
        reloadDetail()
    }

    // ── comments ─────────────────────────────────────────────────────────────

    fun postComment(body: String, members: List<Member>) = mutate {
        if (body.isBlank()) return@mutate
        taskRepo.addComment(taskId, body, detectMentions(body, members))
        _state.update { it.copy(comments = taskRepo.comments(taskId), changed = true) }
    }

    fun editComment(commentId: String, body: String) = mutate {
        if (body.isBlank()) return@mutate
        taskRepo.editComment(commentId, body)
        _state.update { it.copy(comments = taskRepo.comments(taskId), changed = true) }
    }

    fun deleteComment(commentId: String) = mutate {
        taskRepo.deleteComment(commentId)
        _state.update { it.copy(comments = taskRepo.comments(taskId), changed = true) }
    }

    // ── relations ────────────────────────────────────────────────────────────

    fun addRelation(number: Long, kind: String) = mutate {
        taskRepo.addRelation(taskId, number, kind)
        _state.update { it.copy(relations = taskRepo.relations(taskId), changed = true) }
    }

    fun removeRelation(relationId: String) = mutate {
        taskRepo.deleteRelation(relationId)
        _state.update { it.copy(relations = taskRepo.relations(taskId), changed = true) }
    }

    /** Lazily loads workspace tasks for the relation autocomplete (best-effort —
     *  manual #number entry still works if this fails). */
    fun ensureRelationCandidates() {
        if (_state.value.relationCandidates.isNotEmpty() || workspaceId.isBlank()) return
        viewModelScope.launch {
            runCatching { taskRepo.workspaceTasks(workspaceId) }.getOrNull()?.let { tasks ->
                _state.update { it.copy(relationCandidates = tasks) }
            }
        }
    }

    // ── transfer to another board ──────────────────────────────────────────────

    fun transfer(boardId: String, onDone: () -> Unit) = mutate {
        taskRepo.transfer(taskId, boardId)
        _state.update { it.copy(changed = true) }
        onDone()
    }

    // ── attachments ──────────────────────────────────────────────────────────

    fun uploadAttachment(bytes: ByteArray, filename: String, mime: String?) = mutate {
        taskRepo.uploadAttachment(taskId, bytes, filename, mime)
        _state.update { it.copy(attachments = taskRepo.attachments(taskId), changed = true) }
    }

    fun removeAttachment(attachmentId: String) = mutate {
        taskRepo.deleteAttachment(attachmentId)
        _state.update { it.copy(attachments = taskRepo.attachments(taskId), changed = true) }
    }

    /** Downloads an attachment (auth'd) to a cache file and hands it back for
     *  opening/sharing via the host's FileProvider. */
    fun downloadAttachment(cacheDir: File, attachmentId: String, filename: String, onReady: (File) -> Unit) = mutate {
        val file = taskRepo.downloadAttachment(cacheDir, attachmentId, filename)
        onReady(file)
    }

    /** Uploads an inline editor image and returns its public URL (null on failure). */
    suspend fun uploadMediaUrl(bytes: ByteArray, filename: String, mime: String?): String? =
        runCatching { taskRepo.uploadMedia(bytes, filename, mime) }.getOrNull()

    // ── lifecycle actions ────────────────────────────────────────────────────

    fun archive(onDone: () -> Unit) = mutate {
        boardRepo.archiveTask(taskId)
        _state.update { it.copy(changed = true) }
        onDone()
    }

    fun delete(onDone: () -> Unit) = mutate {
        boardRepo.deleteTask(taskId)
        _state.update { it.copy(changed = true) }
        onDone()
    }

    fun clearError() = _state.update { it.copy(error = null) }

    /** Matches `@Name` substrings against known members, longest names first. */
    private fun detectMentions(body: String, members: List<Member>): List<String> =
        members.filter { it.name.isNotBlank() && body.contains("@${it.name}") }.map { it.userId }

    private fun mutate(block: suspend () -> Unit) {
        viewModelScope.launch {
            runCatching { block() }.exceptionOrNull()?.let { e ->
                _state.update { it.copy(error = errorMessage(e)) }
            }
        }
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            runCatching { block() }.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}
