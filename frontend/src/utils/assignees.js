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
