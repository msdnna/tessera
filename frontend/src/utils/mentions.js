// Mention plumbing shared by the composer, the renderer and the hover card.
//
// It builds one row shape used in three places at once:
//   • the composer popup (@-suggestions) — inserts `label`, shows `display` + `hint`;
//   • highlightMentions in the renderer — matches on `label` and `display`;
//   • the hover card — resolves a chip back to the person via `resolveMention`.
//
// `label` is the text actually inserted into the content, `display` the human name
// shown in the popup and the card. They differ on purpose: a comment is pushed to
// GitLab verbatim, so "@Евгений Полянский" resolves to nothing there — the GitLab
// login has to be what lands in the text, while the readable name stays on screen.
import { i18n } from '@/i18n'

// Roles as shown on the hover card, sharing the member-list catalogue entries.
// Unknown roles fall back to the raw value rather than to "member" — inventing a
// role is worse than showing the code. Resolved per call (this module is used
// outside a setup context), so a language switch reaches the card too (#2799).
const ROLES = ['owner', 'admin', 'member']

export function roleLabel(role) {
  if (!role) return ''
  return ROLES.includes(role) ? i18n.global.t(`shell.members.${role}`) : role
}

// glLoginsByUserId maps tessera_user_id → gl_username from the *unfiltered* GitLab
// roster map. gitlabMembersList drops entries already mapped to a Tessera member,
// which is precisely the half we need here, so this takes the raw map. Covers both
// OAuth- (`tessera_user_id`) and PAT-linked (`tessera_user_id_pat`) rows.
export function glLoginsByUserId(gitlabMembersMap) {
  const out = {}
  for (const g of Object.values(gitlabMembersMap || {})) {
    const uid = g.tessera_user_id || g.tessera_user_id_pat
    if (uid && g.gl_username && !out[uid]) out[uid] = g.gl_username
  }
  return out
}

// buildMentionItems merges the Tessera roster with the GitLab roster into the
// single shape mentions are matched, inserted and hover-carded by.
//
// Where a member's inserted login comes from, in order:
//   1. the member's own OAuth identity (`gl_username` on the /members row);
//   2. the GitLab project roster (`gitlabMembers` rows carrying `tessera_user_id`,
//      i.e. OAuth-linked users listed there);
//   3. the raw roster map (`gitlabMembersMap`), which also covers PAT-linked
//      members via `tessera_user_id_pat`;
//   4. nothing — members with no GitLab account keep inserting their name.
//
// Pass the FULL GitLab roster as `gitlabMembers` (Object.values of the store map),
// not the already-filtered gitlabMembersList: a member who signed in via GitLab
// OAuth is in the Tessera roster (matched on user id) yet mentioned by their
// `@gl_username`. A GitLab user mapped to a Tessera member is folded into that
// member (carrying the username) rather than listed twice.
export function buildMentionItems(members, gitlabMembers, gitlabMembersMap) {
  const mems = members || []
  const gls = gitlabMembers || []
  const byId = glLoginsByUserId(gitlabMembersMap)
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
      const login = m.gl_username || (gl && gl.gl_username) || byId[m.user_id] || ''
      return {
        id: m.user_id,
        // Inserted text: the login when we know it (so GitLab resolves the
        // mention on writeback), else the display name.
        label: login || m.name,
        display: m.name,
        // Shown muted next to the name in the popup: the inserted text is no
        // longer what the row says, so the popup spells the login out.
        hint: login ? `@${login}` : '',
        email: m.email,
        role: m.role,
        // Lets the hover card resolve an old "@login" chip to this member even
        // though `label` may since have changed.
        username: login || undefined,
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
    list.find((m) => m.display && m.display.toLowerCase() === key) ||
    null
  )
}
