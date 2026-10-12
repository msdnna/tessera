// Members v2 (#3063): the pure half of the «Участники» window — a member's
// activity, their groups, and what each role may do. Kept out of the component
// so the rules are tested without a window.

// Within this long a member counts as «в сети» (the green dot of MembersWin).
// The server stamps last_seen_at on every token issue (≈ every 15 min of real
// use) and on every socket connect, so a quarter of an hour is what "here now"
// can honestly mean.
export const ONLINE_MS = 15 * 60 * 1000

const toTime = (v) => {
  if (!v) return 0
  const t = new Date(v).getTime()
  return Number.isFinite(t) ? t : 0
}

// lastActivity: the latest of «был в приложении» (last_seen_at) and «последнее
// действие здесь» (last_active_at) — the one moment the row shows — plus whether
// that is recent enough for the dot. { at: ms | 0, online }.
export function lastActivity(member, now = Date.now()) {
  const at = Math.max(toTime(member?.last_seen_at), toTime(member?.last_active_at))
  return { at, online: !!at && now - at < ONLINE_MS }
}

// activityLabel: what the row says about `at`, as an i18n key under
// shell.members.activity plus its count — «сейчас», «12 мин назад», «2 ч
// назад», «вчера», «3 дня назад», or a date beyond a week (key 'date').
export function activityLabel(at, now = Date.now()) {
  if (!at) return { key: 'never', n: 0 }
  const diff = Math.max(0, now - at)
  const min = Math.floor(diff / 60000)
  if (min < 2) return { key: 'now', n: 0 }
  if (min < 60) return { key: 'min', n: min }
  const h = Math.floor(min / 60)
  if (h < 24) return { key: 'h', n: h }
  const d = Math.floor(h / 24)
  if (d < 2) return { key: 'yesterday', n: 1 }
  if (d < 7) return { key: 'days', n: d }
  return { key: 'date', n: 0 }
}

// groupsOf: the names of the groups a member is in, in the groups' own order.
export function groupsOf(userId, groups) {
  if (!userId) return []
  return (groups || []).filter((g) => (g.member_ids || []).includes(userId)).map((g) => g.name)
}

// groupMembers: a group's members that are on the given roster (a group lists
// ids; the roster is who is actually in the workspace right now).
export function groupMembers(group, members) {
  const ids = new Set(group?.member_ids || [])
  return (members || []).filter((m) => ids.has(m.user_id))
}

// groupToggle: what choosing a group in a picker does to a set of chosen ids —
// everyone in it is chosen unless they all already are, then all of them come
// off. { add: ids to add, remove: ids to remove, all: whether all were on }.
export function groupToggle(group, chosen, roster = null) {
  const on = chosen instanceof Set ? chosen : new Set(chosen || [])
  let ids = group?.member_ids || []
  if (roster) {
    const present = new Set(roster.map((m) => m.user_id))
    ids = ids.filter((id) => present.has(id))
  }
  const all = ids.length > 0 && ids.every((id) => on.has(id))
  return all
    ? { add: [], remove: ids, all }
    : { add: ids.filter((id) => !on.has(id)), remove: [], all }
}

// ROLE_MATRIX: what each role may do in a workspace — the «Роли и права»
// section. Read off the server's guards (requireMember / requireManager / the
// owner checks), not a configuration: the role editor that would make it one is
// #3076. Keys are i18n keys under shell.members.rights.
export const ROLES = ['owner', 'admin', 'member']
export const ROLE_MATRIX = [
  { key: 'work', owner: true, admin: true, member: true },
  { key: 'docs', owner: true, admin: true, member: true },
  { key: 'groupsUse', owner: true, admin: true, member: true },
  { key: 'invite', owner: true, admin: true, member: false },
  { key: 'roles', owner: true, admin: true, member: false },
  { key: 'requests', owner: true, admin: true, member: false },
  { key: 'groupsEdit', owner: true, admin: true, member: false },
  { key: 'settings', owner: true, admin: true, member: false },
  { key: 'transfer', owner: true, admin: false, member: false },
  { key: 'delete', owner: true, admin: false, member: false },
  { key: 'leave', owner: false, admin: true, member: true },
]
