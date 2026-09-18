// The half of a task's assignees that has no Tessera account (#2932).
//
// The backend splits the two: a GitLab member linked to a Tessera user is written
// as an ordinary `task_assignees` row (so the board row's `assignee_ids` covers
// them), an unlinked one lands in `task_gitlab_assignees` and reaches the client
// only as `gitlab_assignee_logins` — a list of logins, with no avatar and no
// display name attached. The name and the face come from the workspace's GitLab
// roster (`gitlabMembersList`), which the board loads once and every surface that
// draws these people already holds.
//
// Pulled out of TaskCardPills because it is not the card's rule: the same person
// has to be spelled the same way on the card, in the list rows, in the chart lanes
// and in the composer's «Исполнитель» facet, and each surface that re-derived it
// locally got a different subset of the rule right (or, in the list view's case,
// never asked at all and drew a synced card as unassigned).
//
// `gitlab_assignees` is the older, name-only shape of the same field and stays as
// the fallback: rows synced before logins were carried still have to show a face.

/**
 * A task's GitLab-only assignees, resolved against the workspace roster.
 *
 * @param {Object} task board task row
 * @param {Array<{gl_username?: string, gl_name?: string, gl_avatar_url?: string}>} [glMembers]
 *   workspace GitLab roster (`boardView.gitlabMembersList`)
 * @returns {Array<{login: string, name: string, avatar_url: string|null}>}
 */
export function glAssigneesOf(task, glMembers = []) {
  const logins = (task?.gitlab_assignee_logins || []).filter(Boolean)
  if (!logins.length) {
    return (task?.gitlab_assignees || [])
      .filter(Boolean)
      .map((n) => ({ login: '', name: String(n), avatar_url: null }))
  }
  const byLogin = new Map()
  for (const g of glMembers || []) {
    if (g?.gl_username) byLogin.set(g.gl_username, g)
  }
  return logins.map((login) => {
    const m = byLogin.get(login)
    return { login, name: m?.gl_name || login, avatar_url: m?.gl_avatar_url || null }
  })
}

/**
 * The GitLab half of an assignee picker: the workspace roster PLUS the GitLab
 * people this task already carries that the roster no longer holds (#2932).
 *
 * The roster is not a list of everyone who can be on a card — it is the project's
 * CURRENT membership, pruned to it on every refresh (DeleteStaleGitlabProjectMembers)
 * and refreshed at most hourly. A task's own GitLab assignees outlive it: a row
 * pinned by hand is `source='user'` and survives every sync rebuild (which only
 * replaces the `source='gitlab'` set), so someone who has since left the GitLab
 * project stays assigned indefinitely. The card drew their face (glAssigneesOf) while
 * the picker, built from the roster alone, had no row for them — no checkmark, and no
 * way to take them off the task at all.
 *
 * Extras are appended after the roster and deduped by login, so the ordinary case
 * (everyone assignable is a member) renders exactly as before.
 *
 * `assigned` accepts either shape the two callers hold: the board row's resolved
 * `{login, name, avatar_url}` (glAssigneesOf) and the task detail's raw
 * `{gl_username, gl_name, gl_avatar_url}`. A legacy name-only entry has no login,
 * cannot be toggled, and is skipped.
 *
 * @param {Array<{gl_username?: string, gl_name?: string, gl_avatar_url?: string}>} [glMembers]
 *   workspace GitLab roster (`boardView.gitlabMembersList`)
 * @param {Array<Object>} [assigned] the task's own GitLab assignees (either shape)
 * @param {string} [query] search text; matches display name or login (the Tessera
 *   half of the same picker filters by name, and the two lists share one box)
 * @returns {Array<{gl_username: string, gl_name: string, gl_avatar_url: string}>}
 */
export function glPickerMembers(glMembers = [], assigned = [], query = '') {
  const out = []
  const seen = new Set()
  const push = (login, name, avatar) => {
    if (!login || seen.has(login)) return
    seen.add(login)
    out.push({ gl_username: login, gl_name: name || login, gl_avatar_url: avatar || '' })
  }
  for (const g of glMembers || []) push(g?.gl_username, g?.gl_name, g?.gl_avatar_url)
  for (const a of assigned || []) {
    push(a?.gl_username || a?.login, a?.gl_name || a?.name, a?.gl_avatar_url || a?.avatar_url)
  }
  const q = String(query || '')
    .trim()
    .toLowerCase()
  if (!q) return out
  return out.filter(
    (m) => m.gl_name.toLowerCase().includes(q) || m.gl_username.toLowerCase().includes(q),
  )
}
