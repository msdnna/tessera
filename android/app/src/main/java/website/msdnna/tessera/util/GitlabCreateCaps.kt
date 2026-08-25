package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.GitlabIntegration

/** What this board's GitLab binding allows the task modal to offer: creating an
 *  issue from a task, and prefilling its description from a repo template.
 *  [integrationId] is the binding the templates are read from. */
data class GitlabCreateCaps(
    val integrationId: String? = null,
    val canCreate: Boolean = false,
    val fetchTemplates: Boolean = false,
)

/**
 * Resolves the create-issue capability for one board (web `KanbanBoard.vue`, where
 * the same rule lives).
 *
 * The binding that counts is the one targeting THIS board — a workspace can hold
 * several, and a neighbouring board's `push_create` says nothing about ours.
 * Templates are a sub-option of creation: fetching them without being able to
 * create would prefill a description nobody can push.
 */
fun gitlabCreateCaps(integrations: List<GitlabIntegration>, boardId: String): GitlabCreateCaps {
    if (boardId.isBlank()) return GitlabCreateCaps()
    val binding = integrations.firstOrNull { it.boardId == boardId && it.enabled && it.writeback.pushCreate }
        ?: return GitlabCreateCaps()
    return GitlabCreateCaps(
        integrationId = binding.id,
        canCreate = true,
        fetchTemplates = binding.writeback.fetchTemplates,
    )
}
