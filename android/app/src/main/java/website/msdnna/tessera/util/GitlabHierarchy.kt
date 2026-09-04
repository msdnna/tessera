package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.Task

/**
 * Where one subtask stands in the GitLab issue hierarchy (#2592), in the order the
 * three states are worth telling apart:
 *
 * - [CHILD] — has its own issue AND a parent work item over there: nothing to do.
 * - [DETACHED] — has an issue, but GitLab refused the hierarchy (empty parent gid).
 *   The subtask is NOT lost, it is simply top-level in GitLab.
 * - [ABSENT] — no issue at all: created before the parent was grouped, or the push
 *   is still queued.
 */
enum class GlSubtaskState { CHILD, DETACHED, ABSENT }

/** Mirrors the web `glState` in `TaskSubtasksTab.vue`. An issue number is what makes
 *  the difference between «not there» and «there»; a parent global id is what makes
 *  the difference between «there» and «in the hierarchy». */
fun glSubtaskState(sub: Task): GlSubtaskState = when {
    sub.glIid == null || sub.glIid == 0L -> GlSubtaskState.ABSENT
    sub.glParentGlobalId.isNullOrBlank() -> GlSubtaskState.DETACHED
    else -> GlSubtaskState.CHILD
}

/**
 * Whether this board's GitLab binding pushes subtasks into the issue hierarchy — the
 * gate for the whole grouping half of the task modal (web `KanbanBoard.vue`).
 *
 * Grouping is its own flag and NOT a sub-option of `push_create`: a binding may push
 * subtasks into an existing hierarchy without allowing issues to be created from
 * tasks. Only the binding targeting THIS board counts — a workspace can hold several,
 * and a neighbour's setting says nothing about ours.
 */
fun gitlabCanGroup(integrations: List<GitlabIntegration>, boardId: String): Boolean {
    if (boardId.isBlank()) return false
    return integrations.any { it.boardId == boardId && it.enabled && it.writeback.pushChildren }
}
