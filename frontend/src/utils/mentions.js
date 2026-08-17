// Mention plumbing shared by the composer, the renderer and the hover card.
//
// It exists because the two places that render task content built this list
// differently: the comments tab mapped members into `{id,label,display,…}`, while
// the description handed RichContent the raw member rows (`{user_id,name,…}`).
// highlightMentions matches on `label`, so in descriptions no known name matched
// and multi-word names fell through to the generic handle token — "@Ann Lee"
// highlighted as "@Ann". One builder keeps both call sites on the same shape.

// Roles as shown on the hover card. Unknown roles fall back to the raw value
// rather than to "Участник" — inventing a role is worse than showing the code.
const ROLE_LABELS = { owner: 'Владелец', admin: 'Админ', member: 'Участник' }

export function roleLabel(role) {
  return ROLE_LABELS[role] || role || ''
}

// buildMentionItems merges the Tessera roster with the GitLab roster into the
// single shape mentions are matched, inserted and hover-carded by.
//
// Tessera members insert their display name; GitLab-only users insert their
// `@username` so GitLab resolves the mention on writeback. `label` is the
// inserted text, `display` the row.
//
// Pass the FULL GitLab roster here, not the store's already-filtered
// gitlabMembersList: a member who signed in via GitLab OAuth is in the Tessera
// roster (matched on display name) yet mentioned by their `@gl_username`. If
// their GitLab row were dropped before we get here, that username would resolve
// to nobody and the hover card never shows. We keep the same de-dup — a GitLab
// user mapped to a Tessera member is folded into that member (carrying the
// username) rather than listed twice.
export function buildMentionItems(members, gitlabMembers) {
  const mems = members || []
  const gls = gitlabMembers || []
  // GitLab row per Tessera user it maps to, so a linked member picks up the
  // `@gl_username` they're actually mentioned by.
  const glByTesseraUser = new Map()
  for (const g of gls) {
    if (g.tessera_user_id) glByTesseraUser.set(g.tessera_user_id, g)
  }
  const memberIds = new Set(mems.map((m) => m.user_id))
  return [
    ...mems.map((m) => {
      const gl = glByTesseraUser.get(m.user_id)
      return {
        id: m.user_id,
        label: m.name,
        display: m.name,
        email: m.email,
        role: m.role,
        username: gl ? gl.gl_username : undefined,
        avatarUserId: m.user_id,
      }
    }),
    // GitLab-only users: not linked to any Tessera member in this workspace.
    ...gls
      .filter((g) => !(g.tessera_user_id && memberIds.has(g.tessera_user_id)))
      .map((g) => ({
        id: null,
        label: g.gl_username,
        display: g.gl_name || g.gl_username,
        username: g.gl_username,
        avatarSrc: g.gl_avatar_url,
        gitlab: true,
      })),
  ]
}

// resolveMention turns a rendered mention chip back into the person it names.
// `id` comes from the chip's data-id (set when the name matched a known member,
// and by the legacy TipTap-era chips), `label` from its data-label or text.
// Returns null for a handle nobody in either roster owns — the caller shows no
// card at all rather than one that repeats the text being hovered.
export function resolveMention(items, ref) {
  const list = items || []
  const id = ref && ref.id
  if (id) {
    const byId = list.find((m) => m.id === id)
    if (byId) return byId
  }
  const key = String((ref && ref.label) || '')
    .trim()
    .toLowerCase()
  if (!key) return null
  return (
    list.find((m) => m.username && m.username.toLowerCase() === key) ||
    list.find((m) => m.label && m.label.toLowerCase() === key) ||
    null
  )
}
