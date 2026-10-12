// The one global keydown listener of the app (v2, #2956).
//
// Components attach handlers to catalog ids (`useShortcuts`); this module owns
// the window listener, walks multi-key sequences («G, then B») and decides who
// gets a keystroke. Rules:
//   - keys typed into a text field belong to the field, unless the entry says
//     `input` (⌘K must still open the palette from inside a field);
//   - while a modal dialog or an open dropdown owns the screen, only `modal`
//     entries fire — the arrows belong to the dropdown, not to the board below;
//   - a sequence waits up to SEQ_TIMEOUT for its next key. When one binding is
//     also the start of longer ones (P vs «P, then 0–4»), the short one fires
//     only once the wait runs out — unless it is `eager` (N opens the quick
//     actions at once, and its second key still picks an item).
import { ref } from 'vue'
import { byId } from './catalog'
import { parseKeys, matchChord, capsOf } from './keymap'
import { isTypingTarget } from '@/utils/keys'
import { tourEvent } from '@/utils/tourEvents'

export const SEQ_TIMEOUT = 1000

// id → stack of { run, when }; the most recently attached live handler wins,
// so a dialog that takes over «Esc» shadows the board's without unhooking it.
const handlers = new Map()
const parsed = new Map()
// Our own overlays (the palette) that must silence the page underneath, on top
// of the Naive modals the DOM check below already sees.
let blockers = 0

// Caps of the keys pressed so far in an unfinished sequence — the «G …» chip.
export const pendingCaps = ref([])

let pending = null // { alive: [{ id, step, args }], fallback: { id, args } | null }
let timer = 0

function sequenceOf(id) {
  if (!parsed.has(id)) parsed.set(id, parseKeys(byId.get(id)?.keys))
  return parsed.get(id)
}

export function attach(id, run, when) {
  // A palette-only command (#3068: `board.exportCsv`) has no keys but still
  // needs a live handler for the palette to offer it; an id the catalog doesn't
  // know is a typo.
  const entry = byId.get(id)
  if (!entry || (!entry.keys && !entry.command)) throw new Error(`unknown shortcut: ${id}`)
  const h = { run, when }
  if (!handlers.has(id)) handlers.set(id, [])
  handlers.get(id).push(h)
  return () => {
    const list = handlers.get(id)
    const i = list ? list.indexOf(h) : -1
    if (i >= 0) list.splice(i, 1)
  }
}

export function pushBlocker() {
  blockers++
  let done = false
  return () => {
    if (!done) blockers--
    done = true
  }
}

function liveHandler(id) {
  const list = handlers.get(id)
  if (!list) return null
  for (let i = list.length - 1; i >= 0; i--) {
    const h = list[i]
    if (!h.when || h.when()) return h
  }
  return null
}

// Something modal owns the keyboard: a Naive dialog/drawer, an open dropdown
// menu, or one of our own overlays.
function blocked() {
  if (blockers > 0) return true
  if (typeof document === 'undefined') return false
  return !!document.querySelector('.n-modal-container .n-modal, .n-drawer, .n-dropdown-menu')
}

function eligible(e) {
  const typing = isTypingTarget(e.target)
  const isBlocked = blocked()
  const out = []
  for (const [id] of handlers) {
    const entry = byId.get(id)
    // A workspace command's binding that was replaced (shortcuts/dynamic.js).
    if (!entry) continue
    if (typing && !entry.input) continue
    if (isBlocked && !entry.modal) continue
    if (e.repeat && !entry.repeat) continue
    if (!liveHandler(id)) continue
    out.push(id)
  }
  return out
}

function clearPending() {
  clearTimeout(timer)
  timer = 0
  pending = null
  pendingCaps.value = []
}

// Keys the «Начало работы» checklist counts (#3000): driving a card from the
// keyboard, folding the stage strip.
function report(id) {
  if (id === 'board.stageStrip') tourEvent('stage.strip')
  else if (id.startsWith('card.') && id !== 'card.clear') tourEvent('card.keys')
}

function fire(id, args, e) {
  const h = liveHandler(id)
  if (!h) return
  h.run(...args, e)
  report(id)
}

export function handleKeydown(e) {
  if (e.isComposing || e.defaultPrevented) return
  if (['Shift', 'Control', 'Alt', 'Meta'].includes(e.key)) return

  const pool = pending ? pending.alive : eligible(e).map((id) => ({ id, step: 0, args: [] }))
  const next = []
  const done = []
  for (const p of pool) {
    const seq = sequenceOf(p.id)
    if (!seq.length) continue
    const hit = matchChord(seq[p.step], e)
    if (!hit) continue
    const args = hit.arg === undefined ? p.args : [...p.args, hit.arg]
    if (p.step + 1 === seq.length) done.push({ id: p.id, args })
    else next.push({ id: p.id, step: p.step + 1, args })
  }

  if (!done.length && !next.length) {
    // A dead end mid-sequence: drop it and read this key as a fresh start
    // («G, Esc» must still let Esc do its job).
    if (pending) {
      clearPending()
      handleKeydown(e)
    }
    return
  }
  e.preventDefault()

  // Two complete matches for one key (Esc closes the quick-actions menu and
  // also clears the card selection): the entry with the higher `prio` wins.
  done.sort((a, b) => (byId.get(b.id)?.prio || 0) - (byId.get(a.id)?.prio || 0))

  if (!next.length) {
    clearPending()
    fire(done[0].id, done[0].args, e)
    return
  }

  const eager = done.find((d) => byId.get(d.id)?.eager)
  if (eager) fire(eager.id, eager.args, e)
  const caps = [...pendingCaps.value, ...capsOf(sequenceOf(next[0].id)[next[0].step - 1])]
  clearTimeout(timer)
  pending = { alive: next, fallback: eager ? null : done[0] || null }
  pendingCaps.value = caps
  timer = setTimeout(() => {
    const fb = pending?.fallback
    clearPending()
    if (fb) fire(fb.id, fb.args, null)
  }, SEQ_TIMEOUT)
}

// For the palette's «Команды»: is anyone on screen handling this shortcut right
// now, and run it as if its keys were pressed.
export function isLive(id) {
  return !!liveHandler(id)
}

export function runShortcut(id, ...args) {
  fire(id, args, null)
}

let installed = 0
export function install() {
  if (installed++ === 0) window.addEventListener('keydown', handleKeydown)
  return () => {
    if (--installed === 0) {
      window.removeEventListener('keydown', handleKeydown)
      clearPending()
    }
  }
}

// Tests only: forget every handler and any half-typed sequence.
export function resetShortcuts() {
  handlers.clear()
  blockers = 0
  clearPending()
}
