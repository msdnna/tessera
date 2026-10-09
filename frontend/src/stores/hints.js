import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  useWhatsNewStore,
  SPOTLIGHT_PREFIX,
  HINTS_OFF_PREFIX,
  V2_INTRO,
  V2_LATER,
} from '@/stores/whatsNew'
import { useTourStore } from '@/stores/tour'
import { HINTS, hintByKey } from '@/data/hints'

// The spotlight queue (#3000, mockup PackageH «Очередь spotlight»).
//
// One hint at a time, at most MAX_PER_SESSION per session, «new» before
// contextual. A hint shows only while its element is on screen; one whose
// element is missing (a collapsed sidebar, a board without a stage) waits
// WITHOUT holding up the next — the old queue stuck on a missing target until a
// reload. Whether a target is on screen, and whether the screen is free (no
// tour, window, form, palette, drag or typing), is the layer's business
// (components/tour/SpotlightHint.vue); this store decides what is eligible.
//
// Dismissed = the ack `spotlight:<key>`. Settings → «Подсказки» can switch a
// kind off (`hints:off:new|ctx`), bring one hint back («Показать снова») or all
// of them («Показать подсказки заново») — a hint brought back by hand is
// eligible regardless of the rules below («forced», kept on this browser).

export const MAX_PER_SESSION = 2
const SESSION_KEY = 'tessera_hints_session'
const FORCED_KEY = 'tessera_hints_forced'

function readSession() {
  try {
    return JSON.parse(sessionStorage.getItem(SESSION_KEY) || '[]')
  } catch {
    return []
  }
}
function readForced() {
  try {
    return JSON.parse(localStorage.getItem(FORCED_KEY) || '[]')
  } catch {
    return []
  }
}

export const useHintsStore = defineStore('hints', () => {
  const acks = useWhatsNewStore()
  const tour = useTourStore()
  // Keys shown in this browser session (each counts once against the budget).
  const session = ref(readSession())
  const forced = ref(readForced())
  // The hint on screen right now (the layer sets it).
  const current = ref(null)

  function saveSession() {
    try {
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(session.value))
    } catch {
      /* private mode — the budget just resets with the page */
    }
  }
  function saveForced() {
    try {
      localStorage.setItem(FORCED_KEY, JSON.stringify(forced.value))
    } catch {
      /* private mode */
    }
  }

  const kindOn = (kind) => !acks.has(HINTS_OFF_PREFIX + kind)
  const newOn = computed(() => kindOn('new'))
  const ctxOn = computed(() => kindOn('ctx'))
  const isForced = (key) => forced.value.includes(key)

  // Is a v2 hint offered at all (before its element is looked for)?
  function eligible(h) {
    if (acks.has(SPOTLIGHT_PREFIX + h.key)) return false
    if (!kindOn(h.kind)) return false
    if (isForced(h.key)) return true
    if (h.kind === 'ctx') return true
    if (h.mini) return acks.has(V2_LATER)
    if (h.after === 'v2') return acks.has(V2_INTRO)
    return false
  }

  // Release spotlights (data/whatsNew.js) ride the queue as «new» hints at the
  // sidebar item, once the release card is closed.
  const releaseHints = computed(() => {
    if (!newOn.value) return []
    const gateOpen =
      acks.pending.length === 0 && !acks.historyOpen && !acks.showV2Intro && !tour.active
    if (!gateOpen) return []
    return acks.spotlightQueue.map((s) => ({
      key: s.navKey,
      kind: 'new',
      release: true,
      anchor: `[data-nav="${s.navKey}"]`,
      titleKey: s.titleKey,
      bodyKey: s.bodyKey,
    }))
  })

  // Everything eligible, in the order it should be offered: «new» first.
  const queue = computed(() => {
    const own = HINTS.filter(eligible).map((h) => ({
      ...h,
      titleKey: `hints.items.${h.key}.title`,
      bodyKey: `hints.items.${h.key}.body`,
      metaKey: h.kind === 'ctx' ? `hints.items.${h.key}.meta` : null,
    }))
    const all = [...releaseHints.value, ...own]
    return [...all.filter((h) => h.kind === 'new'), ...all.filter((h) => h.kind !== 'new')]
  })

  const budgetLeft = computed(() => Math.max(0, MAX_PER_SESSION - session.value.length))
  // The layer may show `key` now: in the queue and within the session budget
  // (a hint already shown this session doesn't spend it twice).
  function canShow(key) {
    return session.value.includes(key) || budgetLeft.value > 0
  }

  function shown(hint) {
    current.value = hint
    if (hint && !session.value.includes(hint.key)) {
      session.value = [...session.value, hint.key]
      saveSession()
    }
  }
  function hide() {
    current.value = null
  }

  // «Понятно», ↵, Esc or a click on the target itself.
  async function dismiss(hint = current.value) {
    if (!hint) return
    if (current.value?.key === hint.key) current.value = null
    if (isForced(hint.key)) {
      forced.value = forced.value.filter((k) => k !== hint.key)
      saveForced()
    }
    if (hint.release) await acks.dismissSpotlight(hint.key)
    else await acks.ack(SPOTLIGHT_PREFIX + hint.key)
  }

  // Walking the «Что изменилось» mini-tour marks its hints shown.
  async function markSeen(key) {
    if (key) await acks.ack(SPOTLIGHT_PREFIX + key)
  }

  // Settings → «Подсказки».
  async function setKind(kind, on) {
    if (on) await acks.forget(HINTS_OFF_PREFIX + kind)
    else await acks.ack(HINTS_OFF_PREFIX + kind)
  }
  async function showAgain(key) {
    if (!isForced(key)) {
      forced.value = [...forced.value, key]
      saveForced()
    }
    // A hint brought back by hand is shown even past this session's budget.
    session.value = session.value.filter((k) => k !== key)
    saveSession()
    await acks.forget(SPOTLIGHT_PREFIX + key)
  }
  async function resetAll() {
    forced.value = HINTS.map((h) => h.key)
    saveForced()
    session.value = []
    saveSession()
    await acks.forgetPrefix(SPOTLIGHT_PREFIX)
  }

  // A row of Settings → «Подсказки»: { state: shown|queue|wait|off|later, at, n }.
  function status(key) {
    const h = hintByKey(key)
    if (!h) return { state: 'off' }
    const at = acks.ackedAt?.[SPOTLIGHT_PREFIX + key]
    if (acks.has(SPOTLIGHT_PREFIX + key)) return { state: 'shown', at }
    if (!kindOn(h.kind)) return { state: 'off' }
    if (h.kind === 'ctx') return { state: 'wait' }
    if (!eligible(h)) return { state: 'later' }
    const n = queue.value.filter((x) => x.kind === 'new').findIndex((x) => x.key === key)
    return { state: 'queue', n: n + 1 }
  }

  return {
    current,
    queue,
    newOn,
    ctxOn,
    budgetLeft,
    canShow,
    shown,
    hide,
    dismiss,
    markSeen,
    setKind,
    showAgain,
    resetAll,
    status,
  }
})
