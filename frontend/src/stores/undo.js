import { defineStore } from 'pinia'
import { ref } from 'vue'

// Undo toasts (v2 interaction model, #2969): a reversible action happens at once
// and offers its undo here instead of asking first.
//
// A toast lives UNDO_MS, pausing while hovered or focused; ⌘Z undoes the newest
// live one from anywhere but a text field, F6 moves focus onto it. Up to
// MAX_TOASTS stack, the newest at the bottom. After a successful undo the toast
// turns into a short «… восстановлена» note; a failed one offers «Повторить».
//
// Text is stored as `{ key, params }` (or a function returning a string) and
// translated when drawn — a label resolved at push time would stay in the old
// language after a switch.

export const UNDO_MS = 7000
export const DONE_MS = 2000
export const MAX_TOASTS = 3

let seq = 0

export const useUndoStore = defineStore('undo', () => {
  const toasts = ref([])

  function find(id) {
    return toasts.value.find((t) => t.id === id)
  }

  function drop(id) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  // push shows a toast. `undo` is the async reversal; `icon` is 'trash' |
  // 'archive'; `doneText` / `errorText` caption the outcome. Returns the id.
  function push({ text, undo, icon = 'trash', doneText = null, errorText = null }) {
    const id = ++seq
    toasts.value = [
      ...toasts.value,
      { id, text, undo, icon, doneText, errorText, status: 'live', left: UNDO_MS, paused: 0 },
    ]
    // The oldest live toast makes room: its action simply stands.
    while (toasts.value.length > MAX_TOASTS) toasts.value = toasts.value.slice(1)
    return id
  }

  // tick advances every running timer by dt ms. Driven by the host's interval;
  // a store-side clock keeps the countdown testable without timers.
  function tick(dt) {
    for (const t of toasts.value) {
      if (t.paused > 0) continue
      t.left -= dt
    }
    toasts.value = toasts.value.filter((t) => t.left > 0 || t.status === 'busy')
  }

  // pause/resume nest: hover and focus each hold the timer independently.
  function pause(id, on) {
    const t = find(id)
    if (!t) return
    t.paused = Math.max(0, t.paused + (on ? 1 : -1))
  }

  async function undo(id) {
    const t = find(id)
    if (!t || t.status === 'busy' || t.status === 'done') return
    t.status = 'busy'
    try {
      await t.undo()
      t.status = 'done'
      t.left = DONE_MS
      t.paused = 0
    } catch {
      t.status = 'error'
      t.left = UNDO_MS
    }
  }

  // undoLast is ⌘Z: the newest toast that still can be undone.
  function undoLast() {
    const live = [...toasts.value].reverse().find((t) => t.status === 'live')
    if (!live) return false
    undo(live.id)
    return true
  }

  function dismiss(id) {
    drop(id)
  }

  function clear() {
    toasts.value = []
  }

  return { toasts, push, tick, pause, undo, undoLast, dismiss, clear }
})
