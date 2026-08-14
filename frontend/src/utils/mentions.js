// Building the @-mention roster offered by the comment composer.
//
// `label` is the text actually inserted into the comment, `display` the row in
// the popup. They differ on purpose: a comment is pushed to GitLab verbatim, so
// "@Евгений Полянский" resolves to nothing there — the GitLab login has to be
// what lands in the text, while the human-readable name stays on screen.
//
// Where the login comes from, in order:
//   1. the member's OAuth identity (`gl_username` from /members) — lives on the
//      user, so it works on any board;
//   2. the GitLab project roster, for members linked by PAT rather than OAuth;
//   3. nothing — members with no GitLab account at all (e.g. `admin`) keep
//      inserting their name, exactly as before.

// glLoginsByUserId maps tessera_user_id → gl_username from the *unfiltered*
// GitLab roster. gitlabMembersList drops entries already mapped to a Tessera
// member, which is precisely the half we need here, so this takes the raw map.
export function glLoginsByUserId(gitlabMembersMap) {
  const out = {}
  for (const g of Object.values(gitlabMembersMap || {})) {
    const uid = g.tessera_user_id || g.tessera_user_id_pat
    if (uid && g.gl_username && !out[uid]) out[uid] = g.gl_username
  }
  return out
}

// buildMentionItems merges Tessera members with GitLab-only users into one
// suggestion list. gitlabMembers is the already-filtered list (nobody who maps
// to a Tessera member), so nobody shows up twice.
export function buildMentionItems(members, gitlabMembers, gitlabMembersMap) {
  const byId = glLoginsByUserId(gitlabMembersMap)
  return [
    ...(members || []).map((m) => {
      const login = m.gl_username || byId[m.user_id] || ''
      return {
        id: m.user_id,
        label: login || m.name,
        display: m.name,
        // Shown muted next to the name: the inserted text is no longer what the
        // row says, so the popup has to spell the login out.
        hint: login ? `@${login}` : '',
        avatarUserId: m.user_id,
      }
    }),
    ...(gitlabMembers || []).map((g) => ({
      id: null,
      label: g.gl_username,
      display: g.gl_name || g.gl_username,
      avatarSrc: g.gl_avatar_url,
      gitlab: true,
    })),
  ]
}
