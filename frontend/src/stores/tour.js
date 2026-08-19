import { defineStore } from 'pinia'
import { ref, computed, watchEffect } from 'vue'
import { acknowledgements } from '@/api'
import { GET_STARTED } from '@/data/getStarted'
import { isBrandNewAccount } from '@/utils/account'

// Step engine for the Get Started guide (#2753). The scenario itself lives in
// src/data/getStarted.js; this store only knows how to walk a list of steps,
// when a step is allowed to advance, and how the outcome is recorded.
//
// Acks reuse the generic endpoint (no backend work needed — the key space is
// free-form, and migration 0064 already names `getstarted:<step>` as a case):
//   getstarted:done     — the user walked the guide to the end
//   getstarted:skipped  — the user pressed "Пропустить"
//
// A step looks like:
//   {
//     id: 'workspaces',
//     anchor: 'ws-switch',            // data-tour key or a raw CSS selector
//     extra: ['card-due', 'card-tags'],  // more arrows, "show, don't ask" steps
//     title, body,
//     mode: 'info' | 'action',        // info → «Понятно» + «Пропустить»
//                                     // action → «Пропустить» only, the user acts
//     mask: false,                    // opt out of the dimming mask
//     advanceOn: {                    // action steps only
//       click: 'menu-project',        // …the user clicked this anchor, or
//       snapshot: () => projects.length,   // …a value taken on step entry
//       when: (base) => projects.length > base,  // …changed as expected
//       count: 'project-row',         // …or more elements match this anchor
//                                     //    than when the step opened, or
//       set: '[data-tour="tm-due"][data-tour-set]',  // …anything matches at all
//     },
//   }
//
// `snapshot`/`when` and `count` are the answer to "wait for the click or for the
// entity?": we advance on the entity actually being created, so closing the
// modal without creating anything leaves the tour where it was instead of
// running ahead. `count` is the same rule for entities with no store to watch
// (board tasks live in KanbanBoard's own state) — the caller reports how many
// elements match, and the first report after entering the step is the baseline,
// so re-running the guide with projects already in the tree still walks the user
// through creating one.
//
// `set` is the same idea for the task modal's fields (#2759), where "created" is
// a boolean, not a count: the field carries a data-tour-set marker while it has a
// value, and the step ends as soon as it matches. Deliberately baseline-free —
// a count baseline would deadlock the step on a task whose field was already
// filled, and only «Пропустить» would get the user out.

export const TOUR_PREFIX = 'getstarted:'
export const TOUR_DONE = TOUR_PREFIX + 'done'
export const TOUR_SKIPPED = TOUR_PREFIX + 'skipped'
// Which step the user was on, so an F5 mid-tour doesn't restart from scratch.
// (SPA navigation keeps the store, only a reload needs this.)
export const TOUR_STEP_KEY = 'tessera_tour_step'

export const useTourStore = defineStore('tour', () => {
  const steps = ref([])
  const index = ref(-1)
  const active = ref(false)

  const current = computed(() => (active.value ? steps.value[index.value] || null : null))
  const isLast = computed(() => index.value >= steps.value.length - 1)
  // Anchors of the current step: the primary one first — the arrow head, the
  // mask cutout and the "did it ever show up?" timeout all key off it.
  const anchors = computed(() => {
    const step = current.value
    if (!step) return []
    return [step.anchor, ...(step.extra || [])].filter(Boolean)
  })

  // Value captured when an action step became current, handed back to when().
  let entry = null
  // Same idea for advanceOn.count, except the baseline can only be taken from
  // the first report the view makes after the step opened (the store has no DOM).
  let entryCount = null

  function persist(step) {
    try {
      if (step) localStorage.setItem(TOUR_STEP_KEY, step.id)
      else localStorage.removeItem(TOUR_STEP_KEY)
    } catch {
      /* private mode / quota — the tour still works, it just can't survive F5 */
    }
  }

  function arm() {
    const step = steps.value[index.value] || null
    entry = step?.advanceOn?.snapshot?.() ?? null
    entryCount = null
    persist(step)
  }

  function start(list, { fromId } = {}) {
    const next = Array.isArray(list) ? list.filter(Boolean) : []
    if (!next.length) return false
    steps.value = next
    const at = fromId ? next.findIndex((s) => s.id === fromId) : 0
    index.value = at >= 0 ? at : 0
    active.value = true
    arm()
    return true
  }

  // «Обучение» in the sidebar footer: always from the first step, whatever the
  // acks say — re-running the guide is the whole point of the entry point.
  function startGuide() {
    return start(GET_STARTED)
  }

  // Autostart for a first-time user, decided from the acks the What's-New store
  // already fetched (no second request).
  //
  // Runs only for an account created on/after this build AND with no
  // `getstarted:*` ack: pressing «Пропустить» or reaching the end therefore ends
  // the autostart forever, and re-running is manual from then on.
  //
  // On mobile the guide is not shown AND no ack is written (the author's call):
  // the sidebar lives in a closed drawer there, so the scenario has nothing to
  // point at — the account keeps its right to see the guide on a desktop.
  //
  // `fromId` restores the step across an F5 mid-guide. Only a reload needs it;
  // SPA navigation keeps the store. A guide the user walked away from is never
  // resumed *for* them — they either pressed «Пропустить» (ack, gone) or come
  // back to the same browser, where continuing where they stopped is the least
  // surprising thing that can happen.
  function autoStart({ acked, mobile } = {}) {
    if (active.value || mobile) return false
    const keys = acked instanceof Set ? [...acked] : Array.isArray(acked) ? acked : []
    if (keys.some((k) => String(k).startsWith(TOUR_PREFIX))) return false
    if (!isBrandNewAccount()) return false
    let fromId
    try {
      fromId = localStorage.getItem(TOUR_STEP_KEY) || undefined
    } catch {
      /* private mode — start from the top */
    }
    return start(GET_STARTED, { fromId })
  }

  function stop() {
    active.value = false
    index.value = -1
    steps.value = []
    entry = null
    entryCount = null
    persist(null)
  }

  async function ack(key) {
    try {
      await acknowledgements.ack(key)
    } catch {
      /* offline — the guide is over for this session either way; the endpoint is
         idempotent, so a retry next session costs nothing */
    }
  }

  // Walked to the end.
  async function finish() {
    if (!active.value) return
    stop()
    await ack(TOUR_DONE)
  }

  // "Пропустить" — the whole guide, from any step. Per the author: a guide
  // abandoned halfway isn't resumed on the next login, it stays available from
  // the sidebar footer.
  async function skip() {
    if (!active.value) return
    stop()
    await ack(TOUR_SKIPPED)
  }

  function next() {
    if (!active.value) return
    if (isLast.value) {
      finish()
      return
    }
    index.value += 1
    arm()
  }

  // The step's anchor never appeared (the user navigated away, the section isn't
  // in this workspace). Move on rather than leaving a popover pinned to nothing.
  function anchorMissing(stepId) {
    if (!active.value || current.value?.id !== stepId) return
    next()
  }

  // The user clicked the element an action step asked them to click.
  function clicked(stepId) {
    const step = current.value
    if (!active.value || step?.id !== stepId || step.mode !== 'action') return
    // A step that also waits on an entity (`when`) is not done just because the
    // element was clicked — the watcher below decides.
    if (typeof step.advanceOn?.when === 'function') return
    next()
  }

  // How many elements currently match the step's advanceOn.count (or .set)
  // anchor. For `count` the first report after the step opened is the baseline
  // and growth ends the step; for `set` any match at all ends it.
  function counted(stepId, n) {
    const step = current.value
    if (!active.value || step?.id !== stepId || step.mode !== 'action') return
    const on = step.advanceOn || {}
    if (typeof n !== 'number') return
    if (on.set) {
      if (n > 0) next()
      return
    }
    if (!on.count) return
    if (entryCount === null) {
      entryCount = n
      return
    }
    if (n > entryCount) next()
  }

  // Entity-based advancement: re-checked whenever anything when() reads changes.
  watchEffect(() => {
    const step = current.value
    if (!step || step.mode !== 'action') return
    const when = step.advanceOn?.when
    if (typeof when === 'function' && when(entry)) next()
  })

  return {
    steps,
    index,
    active,
    current,
    isLast,
    anchors,
    start,
    startGuide,
    autoStart,
    stop,
    next,
    skip,
    finish,
    clicked,
    counted,
    anchorMissing,
  }
})
