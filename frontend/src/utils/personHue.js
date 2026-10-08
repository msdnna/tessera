// A person's tone (v2, #3107 — PackageE «Звонок»): the call draws each tile in
// a hue of its own instead of video it does not have. Picked from the aurora's
// seven, by a stable hash of the user id, so a person keeps their colour across
// calls and clients.
export const PERSON_HUES = [
  '#7c5cff',
  '#2f80ed',
  '#0eb0a9',
  '#18a058',
  '#f0a020',
  '#e0533d',
  '#eb2f96',
]

export function personHue(id) {
  const s = String(id || '')
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0
  return PERSON_HUES[h % PERSON_HUES.length]
}
