import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  isTauri,
  listServers,
  activeServerId,
  setActiveServerId,
  addServer,
  updateServer,
  removeServer,
} from '@/utils/servers'

// Desktop multi-server switcher (#2930). The registry itself is plain
// localStorage (utils/servers.js) so non-Vue code — the api layer, the session
// storage wrapper — can read it without a pinia instance; this store is the
// reactive view the rail paints from.
export const useServersStore = defineStore('servers', () => {
  const list = ref(listServers())
  const activeId = ref(activeServerId())
  // serverId → unread notification count, filled in by useServerBadges() for
  // the *inactive* servers. The active one reads the live notifications store.
  const badges = ref({})
  // serverId → true when that server answered "session over": the tile is drawn
  // muted and the poller stops asking until the user signs in there again.
  const signedOut = ref({})

  // The whole feature only exists in the desktop shell. On web the rail never
  // renders and this store stays empty.
  const enabled = computed(() => isTauri())
  const active = computed(() => list.value.find((s) => s.id === activeId.value) || null)
  const others = computed(() => list.value.filter((s) => s.id !== activeId.value))

  function reload() {
    list.value = listServers()
    activeId.value = activeServerId()
  }

  // Switching servers reloads the page rather than re-pointing the running app.
  // axios' baseURL, both websockets, every pinia store and the router all hold
  // server-scoped state; re-initialising them live is far more code and far more
  // ways to end up half-switched. In the Tauri webview the reload is instant.
  function switchTo(id) {
    if (!id || id === activeId.value) return
    setActiveServerId(id)
    location.reload()
  }

  function add(profile) {
    const created = addServer(profile)
    reload()
    return created
  }

  // Editing a profile's *address* moves the running app exactly as far as
  // switchTo() does, so it reloads for the same reason: axios' baseURL is
  // snapshotted at import. Without the reload the app splits in two — ordinary
  // requests keep going to the old host while the token refresh, the OAuth
  // redirect and the avatar URLs (which call apiBaseURL() afresh) already go to
  // the new one. The name, colour and icon touch nothing the app talks to; an
  // inactive profile isn't what this window is pointed at.
  function update(id, patch) {
    const before = list.value.find((s) => s.id === id)?.url || ''
    const saved = updateServer(id, patch)
    reload()
    if (saved && id === activeId.value && saved.url !== before) location.reload()
    return saved
  }

  // Removing the active server leaves the app pointed at whatever the registry
  // picked instead, so it has to reload for the same reason switchTo() does.
  function remove(id) {
    const wasActive = id === activeId.value
    removeServer(id)
    delete badges.value[id]
    delete signedOut.value[id]
    reload()
    if (wasActive) location.reload()
  }

  function setBadge(id, count) {
    badges.value = { ...badges.value, [id]: count }
  }

  function setSignedOut(id, value) {
    signedOut.value = { ...signedOut.value, [id]: !!value }
  }

  return {
    list,
    activeId,
    badges,
    signedOut,
    enabled,
    active,
    others,
    reload,
    switchTo,
    add,
    update,
    remove,
    setBadge,
    setSignedOut,
  }
})
