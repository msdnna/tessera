// Person-facet matching for the board composer bar (filter by assignee / by author).
// Kept out of KanbanBoard.vue so the predicates stay unit-testable. A selected value
// is either a Tessera user uuid or a GitLab login prefixed `gl:` — the same encoding
// the filter menu emits for both facets.

import { uiCollator } from '@/i18n'

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
//
// A Tessera person also matches through their linked GitLab login, exactly as the
// author facet below does — the task can carry that login instead of their user id.
// The sync resolves a GitLab assignee to a Tessera account when it can, so the two
// never coexist on one row, but a login lands in `gitlab_assignee_logins` whenever
// the account was not resolvable AT THAT MOMENT: a member pinned by hand before they
// had an account (source='user' rows survive every sync rebuild — see
// DeleteGitlabSourcedAssignees, which only rebuilds the sync-made set), or an issue
// synced before the person linked theirs and not touched in GitLab since.
//
// Their login is then no longer offered as a `gl:` option either — the roster behind
// the filter menu drops a GitLab member who maps to a workspace member (boardView's
// gitlabMembersList), so one person is never listed twice. Without this lookup that
// card answered to NO value of the facet at all: not to the person's own entry, and
// not to a `gl:` entry the menu no longer has.
export function matchesAssignee(task, selected, glLoginByUserId = {}) {
  if (!selected || !selected.length) return true
  const ids = task.assignee_ids || []
  const logins = task.gitlab_assignee_logins || []
  return selected.some((a) => {
    if (isGitlabValue(a)) return logins.includes(gitlabLogin(a))
    if (ids.includes(a)) return true
    const login = glLoginByUserId[a]
    return !!login && logins.includes(login)
  })
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
  const cmp = uiCollator()
  return [...byLogin.values()].sort((a, b) => cmp.compare(a.gl_name, b.gl_name))
}
