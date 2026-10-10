// The workspace's shared invitation link (#3062) — the pure part of its row in
// «Участники».

// The choices the link's menu offers; the server accepts 1–30 days and 1–500
// people, these are the sensible stops.
export const LINK_TTL_DAYS = [1, 7, 30]
export const LINK_USES = [5, 10, 25, 50, 100]

// daysLeft rounds the remaining lifetime up to whole days — the mockup's
// «6 дней»; a link with hours left reads «1 день», never «0».
export function daysLeft(expiresAt, now = Date.now()) {
  const ms = new Date(expiresAt).getTime() - now
  if (!Number.isFinite(ms) || ms <= 0) return 0
  return Math.ceil(ms / 864e5)
}

// linkMeta words the row's settings as the mockup does: «Участник · 6 дней».
// How many have joined of how many may is the menu's (and the tooltip's) —
// see linkUses — so the row keeps room for the link itself.
export function linkMeta(link, t, now = Date.now()) {
  if (!link) return ''
  const role = t(link.role === 'admin' ? 'shell.members.admin' : 'shell.members.member')
  const left = daysLeft(link.expires_at, now)
  return [
    role,
    left ? t('shell.members.days', { n: left }, left) : t('shell.members.inviteExpired'),
  ].join(' · ')
}

// linkUses is «3/25» — joined by the link out of its limit.
export function linkUses(link, t) {
  if (!link) return ''
  return t('shell.members.usesOf', { n: link.uses ?? 0, max: link.max_uses })
}

// shortLink drops the scheme for display: «tessera.dev/join/7Kp2…».
export function shortLink(url) {
  return String(url || '').replace(/^https?:\/\//, '')
}
