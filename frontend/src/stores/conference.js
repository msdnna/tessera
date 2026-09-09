import { defineStore } from 'pinia'
import { computed, markRaw, ref, watch } from 'vue'
import { conferences as confApi } from '@/api'
import { useConfTransport } from '@/composables/useConfTransport'
import { useConfRoom } from '@/composables/useConfRoom'

// The live conference session, lifted above the router (#2888).
//
// Before this store the whole session — the SFU connection and the room socket —
// was created inside ConferenceRoom and died with it, so navigating to another
// section dropped the call. Here the transport and the room are one long-lived
// pair the whole app shares: ConferenceRoom is the full-screen view of it, the
// floating ConferenceMiniWindow is the compact one, and both read the same
// `store.transport` / `store.room`.
//
// markRaw on the two cores is deliberate: they are plain bags of refs and
// functions, not reactive state to be proxied — Pinia would otherwise unwrap the
// refs on access and every `store.transport.micOn` would read a value instead of
// a ref, breaking the views that destructure them.

// Which conference we are in, kept in sessionStorage so a full page reload can
// walk back into it (#2888). The RTCPeerConnection cannot survive a reload — it
// is torn down with the tab — but the membership on the server does, so we
// re-request a token and reconnect rather than pretending the socket lived.
const ACTIVE_KEY = 'tessera_conf_active'

function readActive() {
  try {
    return sessionStorage.getItem(ACTIVE_KEY) || ''
  } catch {
    return ''
  }
}
function writeActive(id) {
  try {
    if (id) sessionStorage.setItem(ACTIVE_KEY, id)
    else sessionStorage.removeItem(ACTIVE_KEY)
  } catch {
    // Private mode / storage disabled: the session still works, it just will not
    // survive a reload. Nothing to report.
  }
}

export const useConferenceSession = defineStore('conferenceSession', () => {
  // Created here, in the store's setup, so no component instance is current —
  // the transport skips the onBeforeUnmount(leave) it registers for callers, and
  // the session outlives whichever view opened it.
  const transport = markRaw(useConfTransport())
  const room = markRaw(useConfRoom())

  // The conference whose session is live, plus its title for the mini-window's
  // caption (the socket knows the roster, not the meeting's name).
  const activeId = ref('')
  const title = ref('')
  const active = computed(() => !!activeId.value)

  // ── session-wide coordination ─────────────────────────────────────────
  // These used to live in ConferenceRoom and so stopped the moment it unmounted.
  // Up here they hold for the life of the call, which is the whole point: a
  // minimised call still has to keep its roster badge and honour a force-mute.

  // Roster badge follows our own device state — the socket paints "muted" before
  // the first audio packet, so it needs telling directly.
  watch([transport.micOn, transport.camOn], ([mic, cam]) => room.setMedia(mic, cam))

  // Obey a host's force-mute locally, not only in the badge: the SFU has already
  // narrowed our publish permission, so the mic is going quiet either way.
  watch(
    () => room.forceMuted.value,
    (forced) => {
      if (forced && transport.micOn.value) transport.setMic(false)
    },
  )

  // The call ended or we were removed: the socket is already closing itself, so
  // drop the media too and clear the session (the mini-window then disappears).
  watch(
    () => room.ended.value,
    (over) => {
      if (over) stop()
    },
  )

  /**
   * Enter a conference's live session. Idempotent for the same id; switching to a
   * different one drops the first.
   *
   * @param {string} conferenceId
   * @param {string} [confTitle] the meeting's name, for the mini-window caption
   */
  function start(conferenceId, confTitle = '') {
    if (!conferenceId) return
    const fresh = activeId.value !== conferenceId
    if (activeId.value && !fresh) {
      // Same call — just refresh the caption if a better one arrived.
      if (confTitle) title.value = confTitle
      return
    }
    if (activeId.value) stop()
    activeId.value = conferenceId
    title.value = confTitle || ''
    writeActive(conferenceId)
    room.open(conferenceId)
    transport.join(conferenceId)
  }

  /**
   * Tear down the media and the socket without touching the server roster. Used
   * when membership has already ended elsewhere (the call finished, we were
   * kicked, or the view already called the leave endpoint).
   */
  async function stop() {
    room.close()
    activeId.value = ''
    title.value = ''
    writeActive('')
    await transport.leave()
  }

  /**
   * Leave the call for good: drop our seat on the server, then tear the session
   * down. This is what the mini-window's hang-up calls — ConferenceRoom is not
   * mounted there to run the view's own leave flow.
   */
  async function hangup() {
    const id = activeId.value
    if (id) {
      try {
        await confApi.leave(id)
      } catch {
        // The seat may already be gone (ended, kicked); tearing down locally is
        // still the right next step.
      }
    }
    await stop()
  }

  // Walk back into a call after a full page reload (#2888): the membership
  // outlived the tab even though the peer connection did not. Runs once, only
  // once we know who we are, and quietly forgets a call that has since ended or
  // been deleted rather than looping against a 404/409.
  let restored = false
  async function restore() {
    if (restored || activeId.value) return
    const id = readActive()
    if (!id) return
    restored = true
    try {
      const { data } = await confApi.get(id)
      if (data?.conference && data.conference.status !== 'ended') {
        start(id, data.conference.title)
      } else {
        writeActive('')
      }
    } catch {
      // Deleted, forbidden, or offline — do not hold a session we cannot enter.
      writeActive('')
    }
  }

  return { transport, room, activeId, title, active, start, stop, hangup, restore }
})
