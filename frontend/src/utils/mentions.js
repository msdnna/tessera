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

// buildMentionItems merges the Tessera roster with the GitLab-only roster into
// the single shape mentions are matched, inserted and hover-carded by.
//
// Tessera members insert their display name; GitLab-only users insert their
// `@username` so GitLab resolves the mention on writeback. `label` is the
// inserted text, `display` the row. The store's gitlabMembersList already drops
// GitLab users mapped to a Tessera member, so nobody shows up twice.
export function buildMentionItems(members, gitlabMembers) {
  return [
    ...(members || []).map((m) => ({
      id: m.user_id,
      label: m.name,
      display: m.name,
      email: m.email,
      role: m.role,
      avatarUserId: m.user_id,
    })),
    ...(gitlabMembers || []).map((g) => ({
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
