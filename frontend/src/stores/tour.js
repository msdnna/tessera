import { defineStore } from 'pinia'
import { ref, computed, watchEffect } from 'vue'
import { acknowledgements } from '@/api'
import { GET_STARTED, chapterById } from '@/data/getStarted'
import { isBrandNewAccount } from '@/utils/account'

// Step engine for the «Начало работы» tour (#2753, chapters since v2 #3000). The
// scenario lives in src/data/getStarted.js; this store only knows how to walk a
// list of steps, when a step may advance, and how the outcome is recorded.
//
// Acks reuse the generic endpoint (the key space is free-form):
//   getstarted:done      — «Основы» walked to the end (also the v1 guide's mark)
//   getstarted:skipped   — «Завершить обучение»: no autostart from now on
//   getstarted:ch:<id>   — a chapter walked to the end
// The checklist's results (getstarted:item:…) are stores/onboarding.js's.
//
// A step looks like (see data/getStarted.js for the full list of fields):
//   { id, anchor, extra, cut, mode: 'info'|'action', final, item, advanceOn }
//
// Advancement rules for action steps (`advanceOn`):
//   click   — the user clicked this anchor (true = the step's own anchor)
//   snapshot/when — a value taken on step entry changed as expected
//   count   — more elements match than when the step opened. The first report
//             after entering the step is the baseline, so re-running the guide
//             with projects already in the tree still walks the user through
//             creating one.
//   set     — a task-modal field carries a data-tour-set marker. Deliberately
//             baseline-free: a count baseline would deadlock the step on a task
//             whose field was already filled.
//   appear  — anything matches at all: a window opened, the board came up.
//             Baseline-free like `set`.
//   vanish  — the element was on screen and is gone: the window was closed, by
//             the button, Esc or a click outside alike.
//   moved   — the tracked element changed container (drag and drop, #2778):
//             address = querySelector(el)?.closest(within)?.getAttribute(by).
//             An empty address never ends the step — mid-drag SortableJS lifts
//             the node out of its list for a frame.
//
// v2 navigation (PackageH): «Назад» (←) goes back to the previous step of the
// chapter only across info steps — what an action step created is not undone,
// so there is no way back through one. Esc asks «Прервать обучение?»:
// «Продолжить позже» keeps the progress (the checklist, and the step for an F5),
// «Завершить обучение» ends it for good. An action step that is done says so
// for a moment («Засчитано») before the next one opens.

export const TOUR_PREFIX = 'getstarted:'
export const TOUR_DONE = TOUR_PREFIX + 'done'
export const TOUR_SKIPPED = TOUR_PREFIX + 'skipped'
export const TOUR_CHAPTER_PREFIX = TOUR_PREFIX + 'ch:'
// Which step the user was on, so an F5 mid-tour doesn't restart from scratch:
// `<chapter>:<step>` (a bare step id is the v1 guide's, i.e. «Основы»).
export const TOUR_STEP_KEY = 'tessera_tour_step'
// How long a done action step shows «Засчитано · следующий шаг».
export const DONE_MS = 600

// `{token}` in an anchor → the id the sidebar noted for it (noteCreated).
const TOKENS = [
  ['project', 'projectId'],
  ['board', 'boardId'],
  ['group', 'groupId'],
]

export const useTourStore = defineStore('tour', () => {
  const steps = ref([])
  const index = ref(-1)
  const active = ref(false)
  // The chapter being walked (null for an ad-hoc list of steps).
  const chapter = ref(null)
  // Esc pressed: the popover asks «Прервать обучение?» in place.
  const confirming = ref(false)
  // The id of an action step that has just been done, while it says so.
  const flash = ref(null)
  // Tests drive the engine synchronously.
  const doneMs = ref(DONE_MS)
  // Ids of the project/board/group the user creates while walking the guide, so
  // the steps that follow point at *that* row rather than the first one in the
  // tree (#2753 rework, #2778). Anchors carry `{project}` / `{board}` /
  // `{group}` tokens, expanded by resolve().
  const ctx = ref({})

  // A token with no id yet collapses to an empty attribute value that matches
  // nothing, so the step waits for the entity instead of grabbing a stray row.
  //
  // A token used as a SCOPE in front of the real anchor (`[data-tour-project=
  // "{project}"] [data-tour="board-add"]`) is dropped instead when its entity
  // isn't known — a chapter started mid-way (a checklist row, «Продолжить
  // позже» after a reload) has no ctx, and «+» of any project beats a step that
  // can only count down to «Не видим» (#3000). The ids are noted right after
  // the server answers, before the row lands in the tree, so a running tour
  // never sees the unscoped form.
  function resolve(sel) {
    if (typeof sel !== 'string') return sel
    let out = sel
    for (const [tok, key] of TOKENS) {
      const id = ctx.value[key]
      if (!id) out = out.replace(new RegExp(`\\[data-tour-${tok}="\\{${tok}\\}"\\]\\s+`, 'g'), '')
      out = out.replace(new RegExp(`\\{${tok}\\}`, 'g'), id || '')
    }
    return out
  }

  // The sidebar reports the entity it just created (project form / inline board
  // add / new group row) so the arrow lands on it. No-op unless a tour runs.
  function noteCreated(next) {
    if (!active.value || !next) return
    ctx.value = { ...ctx.value, ...next }
  }

  const current = computed(() => (active.value ? steps.value[index.value] || null : null))
  const isLast = computed(() => index.value >= steps.value.length - 1)
  // Position in the chapter for «3 из 10» and the segments under the text.
  const number = computed(() => (active.value ? index.value + 1 : 0))
  const total = computed(() => steps.value.length)
  // Anchors of the current step: the primary one first — the arrow head, the
  // mask cutout and the "did it ever show up?" timeout all key off it.
  const anchors = computed(() => {
    const step = current.value
    if (!step) return []
    return [step.anchor, ...(step.extra || [])].filter(Boolean).map(resolve)
  })
  // «Назад» exists on an info step whose previous step is an info step too.
  const canBack = computed(() => {
    const step = current.value
    const prev = steps.value[index.value - 1]
    return !!step && step.mode !== 'action' && !!prev && prev.mode !== 'action'
  })

  // Value captured when an action step became current, handed back to when().
  let entry = null
  // advanceOn.count / .vanish baseline: the first report after the step opened.
  let entryCount = null
  // advanceOn.moved baseline. `undefined` = "no report yet"; `null` is a real
  // baseline (a project at the root of the tree has no group above it).
  let entryPlace
  let flashTimer = 0

  function persist(step) {
    try {
      if (step) {
        localStorage.setItem(TOUR_STEP_KEY, chapter.value ? `${chapter.value}:${step.id}` : step.id)
      } else localStorage.removeItem(TOUR_STEP_KEY)
    } catch {
      /* private mode / quota — the tour still works, it just can't survive F5 */
    }
  }

  function arm() {
    const step = steps.value[index.value] || null
    entry = step?.advanceOn?.snapshot?.() ?? null
    entryCount = null
    entryPlace = undefined
    confirming.value = false
    persist(step)
  }

  function clearFlash() {
    clearTimeout(flashTimer)
    flashTimer = 0
    flash.value = null
  }

  // start(list) walks an ad-hoc list; start('<chapter id>') a chapter.
  function start(list, { fromId } = {}) {
    let next = list
    let ch = null
    if (typeof list === 'string') {
      ch = chapterById(list)
      next = ch?.steps
    }
    next = Array.isArray(next) ? next.filter(Boolean) : []
    if (!next.length) return false
    clearFlash()
    chapter.value = ch?.id || null
    steps.value = next
    const at = fromId ? next.findIndex((s) => s.id === fromId) : 0
    index.value = at >= 0 ? at : 0
    ctx.value = {}
    active.value = true
    arm()
    return true
  }

  // «Обучение» (sidebar footer, «Ещё», ⌘K): from the first step whatever the
  // acks say — re-running the guide is the whole point of the entry point. On a
  // phone it is the phone's «Основы».
  function startGuide({ mobile = false } = {}) {
    return start(mobile ? 'mobile' : 'basics')
  }

  // Where an F5 left the tour: { chapter, step } or null.
  function persisted() {
    let raw = null
    try {
      raw = localStorage.getItem(TOUR_STEP_KEY)
    } catch {
      /* private mode */
    }
    if (!raw) return null
    const i = raw.indexOf(':')
    return i < 0
      ? { chapter: 'basics', step: raw }
      : { chapter: raw.slice(0, i), step: raw.slice(i + 1) }
  }

  // Autostart for a first-time user, decided from the acks the What's-New store
  // already fetched (no second request).
  //
  // Runs only for an account created on/after this build AND with no
  // `getstarted:done|skipped` ack: «Завершить обучение» or reaching the end of
  // «Основы» ends the autostart forever.
  //
  //   desktop → «Основы», or «Знакомство» for an account invited into a space
  //             that already has projects (it has nothing to create);
  //   phone   → the phone's «Основы» (#3000: the tour exists there now, as a
  //             bottom sheet over the bottom navigation).
  //
  // A reload mid-chapter resumes it where the user was — continuing where they
  // stopped is the least surprising thing on the same browser.
  function autoStart({ acked, mobile = false, invited = false } = {}) {
    if (active.value) return false
    const keys = acked instanceof Set ? [...acked] : Array.isArray(acked) ? acked : []
    if (keys.some((k) => k === TOUR_DONE || k === TOUR_SKIPPED)) return false
    if (!isBrandNewAccount()) return false
    const at = persisted()
    if (at && chapterById(at.chapter)) {
      const ch = chapterById(at.chapter)
      // A phone doesn't resume a desktop chapter and the other way round.
      if (!!ch.mobile === !!mobile) return start(at.chapter, { fromId: at.step })
    }
    if (mobile) return start('mobile')
    // «Знакомство» is offered once; past it the account keeps «Основы» in the
    // checklist like anyone else.
    if (invited && !keys.includes(TOUR_CHAPTER_PREFIX + 'welcome')) return start('welcome')
    return start('basics')
  }

  function stop() {
    clearFlash()
    active.value = false
    index.value = -1
    steps.value = []
    chapter.value = null
    confirming.value = false
    entry = null
    entryCount = null
    entryPlace = undefined
    ctx.value = {}
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

  // Walked to the end of the chapter.
  async function finish() {
    if (!active.value) return
    const ch = chapter.value
    const last = current.value
    stop()
    const keys = []
    if (ch) keys.push(TOUR_CHAPTER_PREFIX + ch)
    // «Основы» (and an ad-hoc list — the v1 guide) also clears the autostart.
    if (!ch || ch === 'basics' || ch === 'mobile') keys.push(TOUR_DONE)
    if (last?.item) noteItem(last.item)
    await Promise.all(keys.map(ack))
  }

  // «Завершить обучение» — from any step. The tour is over and won't open by
  // itself again; the checklist and «Помощь → Обучение» still start chapters.
  async function skip() {
    if (!active.value) return
    stop()
    await ack(TOUR_SKIPPED)
  }

  // «Продолжить позже»: the tour steps aside, nothing is written — the checklist
  // already holds the results, the step stays in localStorage for «Продолжить».
  function later() {
    if (!active.value) return
    const keep = `${chapter.value || 'basics'}:${current.value?.id || ''}`
    stop()
    try {
      localStorage.setItem(TOUR_STEP_KEY, keep)
    } catch {
      /* private mode */
    }
  }

  function abort() {
    if (active.value) confirming.value = true
  }
  function resume() {
    confirming.value = false
  }

  // A result the step stood for — told to the checklist. Lazy import of the
  // store: onboarding asks the tour whether it is running, the tour tells
  // onboarding what got done.
  function noteItem(item) {
    if (!item) return
    import('@/stores/onboarding')
      .then(({ useOnboardingStore }) => useOnboardingStore().mark(item, { tour: true }))
      .catch(() => {})
  }

  // A step of the «Что изменилось» mini-tour stands for a «new» hint: walking
  // past it marks the hint shown, so it doesn't come back as a spotlight.
  function noteHint(key) {
    if (!key) return
    import('@/stores/hints')
      .then(({ useHintsStore }) => useHintsStore().markSeen(key))
      .catch(() => {})
  }

  function next() {
    if (!active.value) return
    clearFlash()
    const step = current.value
    if (step?.hint) noteHint(step.hint)
    if (isLast.value) {
      finish()
      return
    }
    if (step?.item) noteItem(step.item)
    index.value += 1
    arm()
  }

  function back() {
    if (!canBack.value) return
    clearFlash()
    index.value -= 1
    arm()
  }

  // An action step is done: say «Засчитано» for a moment, then move on.
  function complete(stepId) {
    const step = current.value
    if (!active.value || step?.id !== stepId || flash.value) return
    if (!doneMs.value) return next()
    flash.value = stepId
    flashTimer = setTimeout(() => {
      flashTimer = 0
      if (flash.value === stepId && current.value?.id === stepId) next()
      else flash.value = null
    }, doneMs.value)
  }

  // The step's anchor never appeared (the overlay counted it down, or the user
  // pressed «Пропустить шаг»). Move on rather than leaving a popover pinned to
  // nothing.
  function anchorMissing(stepId) {
    if (!active.value || current.value?.id !== stepId) return
    clearFlash()
    if (isLast.value) {
      finish()
      return
    }
    index.value += 1
    arm()
  }

  // The user clicked the element an action step asked them to click.
  function clicked(stepId) {
    const step = current.value
    if (!active.value || step?.id !== stepId || step.mode !== 'action') return
    // A step that also waits on an entity (`when`) is not done just because the
    // element was clicked — the watcher below decides.
    if (typeof step.advanceOn?.when === 'function') return
    complete(stepId)
  }

  // How many elements currently match the step's count / set / appear / vanish
  // selector. `count`: the first report after the step opened is the baseline
  // and growth ends the step; `set`/`appear`: any match at all ends it;
  // `vanish`: a match that drops to none ends it.
  function counted(stepId, n) {
    const step = current.value
    if (!active.value || step?.id !== stepId || step.mode !== 'action') return
    const on = step.advanceOn || {}
    if (typeof n !== 'number') return
    if (on.set || on.appear) {
      if (n > 0) complete(stepId)
      return
    }
    if (on.vanish) {
      if (n > 0) entryCount = n
      else if (entryCount) complete(stepId)
      return
    }
    if (!on.count) return
    if (entryCount === null) {
      entryCount = n
      return
    }
    if (n > entryCount) complete(stepId)
  }

  // Where the element an advanceOn.moved step tracks currently lives, as
  // reported by the view. First report = baseline; a *different*, non-empty
  // address ends the step.
  function located(stepId, place) {
    const step = current.value
    if (!active.value || step?.id !== stepId || step.mode !== 'action') return
    if (!step.advanceOn?.moved) return
    const at = place || null
    if (entryPlace === undefined) {
      entryPlace = at
      return
    }
    if (at === null) return
    if (at !== entryPlace) complete(stepId)
  }

  // Entity-based advancement: re-checked whenever anything when() reads changes.
  watchEffect(() => {
    const step = current.value
    if (!step || step.mode !== 'action') return
    const when = step.advanceOn?.when
    if (typeof when === 'function' && when(entry)) complete(step.id)
  })

  return {
    steps,
    index,
    active,
    chapter,
    confirming,
    flash,
    doneMs,
    current,
    isLast,
    number,
    total,
    anchors,
    canBack,
    resolve,
    noteCreated,
    start,
    startGuide,
    autoStart,
    persisted,
    stop,
    next,
    back,
    abort,
    resume,
    later,
    skip,
    finish,
    complete,
    clicked,
    counted,
    located,
    anchorMissing,
  }
})

// The v1 scenario, for anything that still imports it by that name.
export { GET_STARTED }
