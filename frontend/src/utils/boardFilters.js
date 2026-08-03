// Person-facet matching for the board composer bar (filter by assignee / by author).
// Kept out of KanbanBoard.vue so the predicates stay unit-testable. A selected value
// is either a Tessera user uuid or a GitLab login prefixed `gl:` — the same encoding
// the filter menu emits for both facets.

const GL_PREFIX = 'gl:'

// True for a `gl:<username>` filter value (as opposed to a Tessera user uuid).
export function isGitlabValue(v) {
  return typeof v === 'string' && v.startsWith(GL_PREFIX)
}

// The bare GitLab login behind a `gl:` value ('' for a Tessera uuid).
export function gitlabLogin(v) {
  return isGitlabValue(v) ? v.slice(GL_PREFIX.length) : ''
}

// Assignee facet: a task matches when any selected person is among its Tessera
// assignees or its GitLab assignee logins. An empty selection filters nothing.
export function matchesAssignee(task, selected) {
  if (!selected || !selected.length) return true
  const ids = task.assignee_ids || []
  const logins = task.gitlab_assignee_logins || []
  return selected.some((a) =>
    isGitlabValue(a) ? logins.includes(gitlabLogin(a)) : ids.includes(a),
  )
}

// Author facet. GitLab-synced tasks have `created_by IS NULL` — their author lives
// in `gitlab_author` — so a Tessera person also matches through the GitLab login
// linked to their account (`glLoginByUserId`: tessera user id → gl_username).
// A task with neither author is filtered out once any author is selected.
export function matchesAuthor(task, selected, glLoginByUserId = {}) {
  if (!selected || !selected.length) return true
  const createdBy = task.created_by || null
  const glAuthor = task.gitlab_author || null
  return selected.some((a) => {
    if (isGitlabValue(a)) return !!glAuthor && glAuthor === gitlabLogin(a)
    if (createdBy && createdBy === a) return true
    const login = glLoginByUserId[a]
    return !!login && !!glAuthor && glAuthor === login
  })
}

// Distinct GitLab authors present on the board, for the author-filter menu: an issue
// can be opened by someone outside the project's member roster, who would otherwise
// never appear as a filter option. Sorted by display name.
export function boardGitlabAuthors(tasks) {
  const byLogin = new Map()
  for (const t of tasks || []) {
    const login = t.gitlab_author
    if (!login || byLogin.has(login)) continue
    byLogin.set(login, {
      gl_username: login,
      gl_name: t.gitlab_author_name || login,
      gl_avatar_url: t.gitlab_author_avatar_url || '',
    })
  }
  return [...byLogin.values()].sort((a, b) => a.gl_name.localeCompare(b.gl_name, 'ru'))
}
