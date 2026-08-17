// A stable colour per person, for telling collaborators apart in a document
// (#2729 rework).
//
// The point is not decoration: the colour is a *name*. "Жёлтый блок — это Петя"
// only works if Петя is yellow on every screen in the room, so the colour is
// derived from the user id rather than from anything local (join order, a random
// draw, an index into the viewer list — all of which differ per client and would
// make the cue actively misleading).

// Hues chosen to stay apart from each other and from the app's purple accent, so
// a held block never reads as a piece of ordinary UI chrome. They are base
// colours: pass them through readableHue() before using one as text on a themed
// background.
export const USER_COLORS = [
  '#e0a01f', // amber
  '#2f7fe0', // blue
  '#e05a4f', // coral
  '#2fa37a', // teal
  '#c159d1', // orchid
  '#d97528', // orange
  '#4f6fd9', // indigo
  '#8aa32f', // olive
]

/**
 * FNV-1a over the id. Any stable hash would do; this one is short, has no
 * dependencies and spreads short uuid-shaped strings evenly enough that two
 * people in a room rarely collide.
 *
 * @param {string} s input
 * @returns {number} 32-bit unsigned hash
 */
function hash(s) {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i += 1) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h >>> 0
}

/**
 * The colour that identifies a person across every client in a document.
 *
 * @param {string} userId the user's id
 * @returns {string} hex colour from USER_COLORS
 */
export function userColor(userId) {
  const id = String(userId || '')
  if (!id) return USER_COLORS[0]
  return USER_COLORS[hash(id) % USER_COLORS.length]
}
