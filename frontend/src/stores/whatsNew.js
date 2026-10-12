import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { acknowledgements } from '@/api'
import { useTourStore } from '@/stores/tour'
import { isBrandNewAccount } from '@/utils/account'
import { WHATS_NEW } from '@/data/whatsNew'

// Per-user "seen once" state for the What's-New modal and the sidebar spotlight
// hints (#2749), backed by the generic acknowledgements endpoint. Keys:
//   whatsnew:web:<version>  — the changelog modal for a release was dismissed here
//   spotlight:<navKey>      — an arrow hint pointing at a sidebar item was dismissed
//
// The changelog key is namespaced per client: web and Android version
// independently, so a shared `whatsnew:<version>` let one client's far-higher
// numbers raise the other's baseline and hide its card for good (see Android's
// util/WhatsNew.kt). Legacy bare keys are still honoured, but only for a version
// this build actually ships (whatsNewVersion), so the other client's numbers no
// longer leak in. Spotlights stay shared — a navKey names the same sidebar feature
// on both, so dismissing the arrow once is meant to settle it everywhere.
//
// The running web build is __APP_VERSION__; highlights are only ever shown up to
// that version (never for a release the loaded bundle predates).

const WHATSNEW_PREFIX = 'whatsnew:'
const WHATSNEW_WEB_PREFIX = 'whatsnew:web:'
export const SPOTLIGHT_PREFIX = 'spotlight:'
// Settings → «Подсказки» (#3101): a kind of hint switched off. `new` — «Что
// нового» and the hints at what changed; `ctx` — hints at first encounter.
export const HINTS_OFF_PREFIX = 'hints:off:'
// The v2 release card with the «Что изменилось» mini-tour (#3000), shown once
// to an account that used v1. `…:later` — «Позже» sent the mini-tour's points
// to the spotlight queue instead.
export const V2_INTRO = 'whatschanged:v2'
export const V2_LATER = 'whatschanged:later'

// The release version an ack key vouches for on web, or null if it doesn't count
// here. Mirrors Android's whatsNewVersion: web-namespaced keys always count; a
// legacy bare `whatsnew:<v>` counts only for a version we ship (ownVersions);
// another client's namespace (whatsnew:android:*) is ignored.
function whatsNewVersion(key, ownVersions) {
  if (key.startsWith(WHATSNEW_WEB_PREFIX)) return key.slice(WHATSNEW_WEB_PREFIX.length)
  if (!key.startsWith(WHATSNEW_PREFIX)) return null
  const rest = key.slice(WHATSNEW_PREFIX.length)
  if (rest.includes(':')) return null // another client's namespace
  return ownVersions.has(rest) ? rest : null // legacy bare key — only for our releases
}

// Numeric semver compare for simple x.y.z strings. Returns >0 if a>b.
function cmp(a, b) {
  const pa = String(a).split('.').map(Number)
  const pb = String(b).split('.').map(Number)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] || 0) - (pb[i] || 0)
    if (d) return d
  }
  return 0
}

const currentVersion = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.0.0'

// Freshly-registered accounts are excluded from What's New: they never updated
// *into* anything, so the changelog would be noise (and, in the e2e suite, every
// registered user would get the modal's mask over the page, blocking clicks —
// #2749). The test itself is shared with the Get Started guide, see utils/account.

export const useWhatsNewStore = defineStore('whatsNew', () => {
  const tour = useTourStore()
  const acked = ref(new Set())
  // key → when it was first set, for «показана 6 окт.» in Settings → «Подсказки».
  const ackedAt = ref({})
  const loaded = ref(false)
  // The v2 card replaces the release list once for a v1 user (load() decides).
  const v2Intro = ref(false)
  // The release entries to show in the modal this session (computed on load).
  const pendingRaw = ref([])
  // While the Get Started guide is running it owns the screen: the changelog
  // modal would drop its own mask on top of the guide's. Nothing is lost — the
  // queue is only hidden, so it surfaces as soon as the guide is over (or on the
  // next load). Autostart alone can't collide (a brand-new account gets the
  // changelog baselined silently), but «Обучение» in the footer can be pressed
  // by anyone, at any time.
  const pending = computed(() =>
    tour.active || has(HINTS_OFF_PREFIX + 'new') ? [] : pendingRaw.value,
  )
  // The v2 card is on screen: same gate as the release list.
  const showV2Intro = computed(() => v2Intro.value && !tour.active && !historyOpen.value)
  // Spotlight hints to show one-at-a-time AFTER the modal is dismissed. Kept
  // separate from `pending` so dismissing the modal doesn't wipe the queue.
  const spotlightQueue = ref([])
  // The same modal opened by hand from the version stamp in the footer (#2812):
  // the full changelog rather than "what you just updated into".
  const historyOpen = ref(false)

  function openHistory() {
    historyOpen.value = true
  }

  // Closing the history acknowledges nothing: it was opened deliberately, and
  // advancing the baseline here would silently eat the next real What's New.
  function closeHistory() {
    historyOpen.value = false
  }

  // Are we showing the history right now? An update the user hasn't seen yet
  // wins — that modal is already on screen, and it is the one that writes acks.
  const isHistory = computed(() => historyOpen.value && pending.value.length === 0)

  // What the modal renders. In history mode: every curated release, newest
  // first, regardless of acks or of the build the bundle was cut from (#2812
  // asks for "everything in whatsNew.js"); otherwise the update queue.
  const entries = computed(() => {
    if (!isHistory.value) return pending.value
    return [...WHATS_NEW].sort((a, b) => cmp(b.version, a.version))
  })

  function has(key) {
    return acked.value.has(key)
  }

  async function ack(key) {
    if (acked.value.has(key)) return
    // optimistic; the endpoint is idempotent. A fresh Set so computed readers
    // (the checklist, the hint queue) see the change.
    acked.value = new Set([...acked.value, key])
    ackedAt.value = { ...ackedAt.value, [key]: new Date().toISOString() }
    try {
      await acknowledgements.ack(key)
    } catch {
      /* offline — the optimistic add keeps it hidden this session; a failed
         write just means it may reappear next session, which is acceptable */
    }
  }

  // Clear flags (#3100): one key, or every key under a namespace prefix.
  async function forget(key) {
    if (!acked.value.has(key)) return
    const next = new Set(acked.value)
    next.delete(key)
    acked.value = next
    try {
      await acknowledgements.forget({ key })
    } catch {
      /* offline — it comes back on the next load; nothing is lost */
    }
  }
  async function forgetPrefix(prefix) {
    acked.value = new Set([...acked.value].filter((k) => !k.startsWith(prefix)))
    try {
      await acknowledgements.forget({ prefix })
    } catch {
      /* offline — see forget() */
    }
  }

  // Load acks and decide what (if anything) to surface. First-ever run with this
  // A user with no whatsnew acks has baseline 0.0.0, so a first run surfaces the
  // curated highlights up to the current build once (debut + testable: clearing
  // the whatsnew acks makes it show again). Nothing is written until the user
  // dismisses — so the mere presence of the modal never leaves a DB trail.
  //
  // Returns whether the acks were actually read: the Get Started guide rides on
  // the same set, and "the request failed" must not read as "this user has no
  // getstarted ack" — that would re-run the guide for someone who finished it.
  async function load() {
    try {
      const { data } = await acknowledgements.list()
      acked.value = new Set((data || []).map((a) => a.key))
      ackedAt.value = Object.fromEntries((data || []).map((a) => [a.key, a.ack_at]))
    } catch {
      loaded.value = true
      return false // offline / unauth — surface nothing rather than risk a double-show
    }
    loaded.value = true

    const ownVersions = new Set(WHATS_NEW.map((e) => e.version))
    const ackedVersions = [...acked.value]
      .map((k) => whatsNewVersion(k, ownVersions))
      .filter((v) => v !== null)

    // Brand-new sign-ups (no acks yet, account created on/after this build) get
    // baselined silently — nothing to catch up on, nothing to interrupt them with.
    if (ackedVersions.length === 0 && isBrandNewAccount()) {
      await ack(WHATSNEW_WEB_PREFIX + currentVersion)
      return true
    }

    // A v1 user — they walked or skipped the old guide, or have read a release
    // card — gets the v2 card once instead of the list (#3000). A brand-new
    // account never does: it has nothing to compare with.
    const usedV1 = [...acked.value].some(
      (k) => k.startsWith('getstarted:') || k.startsWith(WHATSNEW_PREFIX),
    )
    v2Intro.value =
      usedV1 && !isBrandNewAccount() && !has(V2_INTRO) && !has(HINTS_OFF_PREFIX + 'new')

    const highest = ackedVersions.reduce((m, v) => (cmp(v, m) > 0 ? v : m), '0.0.0')
    const releases = WHATS_NEW.filter(
      (e) => cmp(e.version, highest) > 0 && cmp(e.version, currentVersion) <= 0,
    ).sort((a, b) => cmp(b.version, a.version))
    pendingRaw.value = releases

    // Spotlights from those releases the user hasn't dismissed yet, newest first,
    // de-duplicated by navKey. Snapshotted here so it outlives dismissModal().
    const seen = new Set()
    spotlightQueue.value = releases
      .map((e) => e.spotlight)
      .filter((s) => s && !has(SPOTLIGHT_PREFIX + s.navKey))
      .filter((s) => (seen.has(s.navKey) ? false : seen.add(s.navKey)))
    return true
  }

  // Called when the user dismisses the modal: mark every shown release seen, and
  // advance the baseline to the current build. The spotlight queue is untouched
  // and starts showing once the modal is gone.
  async function dismissModal() {
    const keys = pendingRaw.value.map((e) => WHATSNEW_WEB_PREFIX + e.version)
    pendingRaw.value = []
    await Promise.all([...keys, WHATSNEW_WEB_PREFIX + currentVersion].map(ack))
  }

  // The spotlight to show right now: the head of the queue, but only once the
  // modal is closed (modal first, then spotlights one at a time) — and never
  // while the guide is drawing its own arrows, nor while the history is open
  // (`pending` is empty exactly then, so the arrow would land on top of it).
  const currentSpotlight = computed(() =>
    pending.value.length === 0 && !tour.active && !historyOpen.value && !showV2Intro.value
      ? spotlightQueue.value[0] || null
      : null,
  )

  // The v2 card is answered: «Показать, что изменилось» (tour = true) or
  // «Позже». Either way the releases it stood in for are read, and the card
  // never comes back. «Позже» sends the mini-tour's points to the spotlight
  // queue (stores/hints.js reads V2_LATER).
  async function closeV2Intro({ tour: walk = false } = {}) {
    v2Intro.value = false
    const keys = [V2_INTRO]
    if (!walk) keys.push(V2_LATER)
    await Promise.all([...keys.map(ack), dismissModal()])
  }

  async function dismissSpotlight(navKey) {
    spotlightQueue.value = spotlightQueue.value.filter((s) => s.navKey !== navKey)
    await ack(SPOTLIGHT_PREFIX + navKey)
  }

  return {
    acked,
    ackedAt,
    loaded,
    v2Intro,
    showV2Intro,
    pending,
    entries,
    historyOpen,
    isHistory,
    spotlightQueue,
    currentSpotlight,
    load,
    has,
    ack,
    forget,
    forgetPrefix,
    closeV2Intro,
    openHistory,
    closeHistory,
    dismissModal,
    dismissSpotlight,
  }
})
