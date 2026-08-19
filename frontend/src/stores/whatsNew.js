import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { acknowledgements } from '@/api'
import { useTourStore } from '@/stores/tour'
import { isBrandNewAccount } from '@/utils/account'
import { WHATS_NEW } from '@/data/whatsNew'

// Per-user "seen once" state for the What's-New modal and the sidebar spotlight
// hints (#2749), backed by the generic acknowledgements endpoint. Keys:
//   whatsnew:<version>   — the changelog modal for a release was dismissed
//   spotlight:<navKey>   — an arrow hint pointing at a sidebar item was dismissed
//
// The running web build is __APP_VERSION__; highlights are only ever shown up to
// that version (never for a release the loaded bundle predates).

const WHATSNEW_PREFIX = 'whatsnew:'
const SPOTLIGHT_PREFIX = 'spotlight:'

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
  const loaded = ref(false)
  // The release entries to show in the modal this session (computed on load).
  const pendingRaw = ref([])
  // While the Get Started guide is running it owns the screen: the changelog
  // modal would drop its own mask on top of the guide's. Nothing is lost — the
  // queue is only hidden, so it surfaces as soon as the guide is over (or on the
  // next load). Autostart alone can't collide (a brand-new account gets the
  // changelog baselined silently), but «Обучение» in the footer can be pressed
  // by anyone, at any time.
  const pending = computed(() => (tour.active ? [] : pendingRaw.value))
  // Spotlight hints to show one-at-a-time AFTER the modal is dismissed. Kept
  // separate from `pending` so dismissing the modal doesn't wipe the queue.
  const spotlightQueue = ref([])

  function has(key) {
    return acked.value.has(key)
  }

  async function ack(key) {
    if (acked.value.has(key)) return
    acked.value.add(key) // optimistic; the endpoint is idempotent
    try {
      await acknowledgements.ack(key)
    } catch {
      /* offline — the optimistic add keeps it hidden this session; a failed
         write just means it may reappear next session, which is acceptable */
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
    } catch {
      loaded.value = true
      return false // offline / unauth — surface nothing rather than risk a double-show
    }
    loaded.value = true

    const ackedVersions = [...acked.value]
      .filter((k) => k.startsWith(WHATSNEW_PREFIX))
      .map((k) => k.slice(WHATSNEW_PREFIX.length))

    // Brand-new sign-ups (no acks yet, account created on/after this build) get
    // baselined silently — nothing to catch up on, nothing to interrupt them with.
    if (ackedVersions.length === 0 && isBrandNewAccount()) {
      await ack(WHATSNEW_PREFIX + currentVersion)
      return true
    }

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
    const keys = pendingRaw.value.map((e) => WHATSNEW_PREFIX + e.version)
    pendingRaw.value = []
    await Promise.all([...keys, WHATSNEW_PREFIX + currentVersion].map(ack))
  }

  // The spotlight to show right now: the head of the queue, but only once the
  // modal is closed (modal first, then spotlights one at a time) — and never
  // while the guide is drawing its own arrows.
  const currentSpotlight = computed(() =>
    pending.value.length === 0 && !tour.active ? spotlightQueue.value[0] || null : null,
  )

  async function dismissSpotlight(navKey) {
    spotlightQueue.value = spotlightQueue.value.filter((s) => s.navKey !== navKey)
    await ack(SPOTLIGHT_PREFIX + navKey)
  }

  return {
    acked,
    loaded,
    pending,
    spotlightQueue,
    currentSpotlight,
    load,
    has,
    ack,
    dismissModal,
    dismissSpotlight,
  }
})
