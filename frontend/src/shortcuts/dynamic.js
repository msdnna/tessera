// Keys of the workspace's palette commands (#3069, CmdsWin «Клавиши»): bindings
// that come from the server, not from the catalog. They join the one dispatcher
// as entries `macro:<id>:<keys>` (the keys in the id keep the parse cache honest
// when a command's keys change) and never shadow the app: a chord that collides
// with a catalog binding — the same keys, or one sequence starting the other —
// is refused here and reported by `keyConflicts` in the command editor.
import { byId } from './catalog'
import { attach } from './dispatcher'
import { parseKeys } from './keymap'

const PREFIX = 'macro:'

const sameChord = (a, b) =>
  a.key === b.key &&
  !!a.mod === !!b.mod &&
  !!a.shift === !!b.shift &&
  !!a.alt === !!b.alt &&
  (a.char || '') === (b.char || '')

// keyConflicts — the bindings (catalog ids, or other commands' ids) these keys
// would collide with. `except` skips one id (the command being edited).
export function keyConflicts(keys, { except = '' } = {}) {
  const seq = parseKeys(keys)
  if (!seq.length) return []
  const out = []
  for (const [id, e] of byId) {
    if (!e?.keys || id === except || (except && id.startsWith(`${PREFIX}${except}:`))) continue
    const other = parseKeys(e.keys)
    const n = Math.min(seq.length, other.length)
    let prefix = n > 0
    for (let i = 0; i < n && prefix; i++) prefix = sameChord(seq[i], other[i])
    if (prefix) out.push(id)
  }
  return out
}

let detach = []

// setMacroKeys replaces the command bindings: [{ id, keys, run }]. A binding in
// conflict is skipped (the editor already said why).
export function setMacroKeys(list) {
  for (const off of detach) off()
  detach = []
  for (const id of [...byId.keys()]) if (id.startsWith(PREFIX)) byId.delete(id)
  for (const m of list) {
    if (!m.keys || keyConflicts(m.keys).length) continue
    const id = `${PREFIX}${m.id}:${m.keys}`
    byId.set(id, { id, keys: m.keys, macro: true })
    detach.push(attach(id, m.run))
  }
}

// The dispatcher id a command's keys are bound under (for <ShortcutKeys>).
export function macroKeyId(m) {
  const id = `${PREFIX}${m.id}:${m.keys}`
  return m.keys && byId.has(id) ? id : null
}
