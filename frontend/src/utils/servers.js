// Registry of Tessera servers the desktop app can switch between (#2930).
//
// The desktop build is a Tauri shell around this very SPA, talking to a
// configurable origin (see serverBase.js). Before this file there was exactly
// one such origin, stored under `tessera_server_base`; now there is a list of
// named profiles and one of them is active.
//
// On the web everything here is inert: the SPA is same-origin with its API, so
// `listServers()` is empty, `activeServerId()` is '' and no key is ever written.
// That is deliberate — this feature must not be observable in a browser.

const LIST_KEY = 'tessera_servers'
const ACTIVE_KEY = 'tessera_active_server'
// Pre-#2930 single-server setting. Read once during migration, then left alone.
const LEGACY_BASE_KEY = 'tessera_server_base'

export const DEFAULT_BASE = 'https://tessera.msdnna.website'
export const DEFAULT_COLOR = '#7c5cff'

// Session state that belongs to ONE server and is therefore namespaced per
// profile on desktop (`<key>::<serverId>`). Without this, switching servers
// would carry the previous server's session, workspace and preferences over —
// and the refresh token of server A would be posted to server B.
export const SESSION_KEYS = [
  'tessera_refresh_token',
  'tessera_user',
  'tessera_prefs',
  'tessera_color',
  'tessera_dark',
  'tessera_ws',
  'tessera_device_id',
]

// isTauri reports whether we're running inside the Tauri webview. It lives here
// rather than in serverBase.js because the registry needs it and serverBase
// imports from this module — serverBase re-exports it, so every existing
// `import { isTauri } from '@/utils/serverBase'` keeps working.
//
// Detection is deliberately NOT limited to the injected IPC globals. Tauri v2
// injects __TAURI_INTERNALS__ (and, with withGlobalTauri, __TAURI__) via an init
// script — but that can lag the very first synchronous reads of the bundle. If
// isTauri() answered "no" for even one such read at boot, the session helpers
// would look at the *un-namespaced* keys, find nothing, and start the app signed
// out — logging desktop users out on every reload/switch (#2930 rework). So we
// also check the location: the production webview serves the app from a Tauri
// custom protocol (tauri://localhost on Linux/macOS, http://tauri.localhost on
// Windows WebView2), which is available from the first line, before the globals.
export function isTauri() {
  if (typeof window === 'undefined') return false
  if (window.__TAURI_INTERNALS__ || window.__TAURI__) return true
  const loc = window.location
  return !!loc && (loc.protocol === 'tauri:' || loc.hostname === 'tauri.localhost')
}

// normalizeUrl trims and drops trailing slashes — '<base>/api' is built by
// concatenation, so a stray slash would produce '//api'.
export function normalizeUrl(url) {
  return (url || '').trim().replace(/\/+$/, '')
}

// serverLabel is the fallback display name: the host of the address.
export function serverLabel(url) {
  try {
    return new URL(url).hostname || 'Tessera'
  } catch {
    return normalizeUrl(url) || 'Tessera'
  }
}

function newId() {
  return (
    (typeof crypto !== 'undefined' && crypto.randomUUID && crypto.randomUUID()) ||
    `srv-${Date.now()}-${Math.random().toString(36).slice(2)}`
  )
}

function readList() {
  try {
    const raw = JSON.parse(localStorage.getItem(LIST_KEY) || 'null')
    return Array.isArray(raw) ? raw.filter((s) => s && s.id) : []
  } catch {
    return []
  }
}

function writeList(list) {
  localStorage.setItem(LIST_KEY, JSON.stringify(list))
}

// ensureRegistry migrates a pre-#2930 install the first time anything asks for
// a server: the single stored base URL becomes profile #1 and the session keys
// already in localStorage are renamed into its namespace. Without the rename an
// update would silently sign everybody out — the session would still be on disk
// under the old names, just nowhere anyone looks.
//
// Stateless on purpose (one getItem to decide, no module-level flag): the entry
// point differs between the app, the tests and the e2e build, and a cached "done"
// flag would go stale the moment a test clears localStorage.
export function ensureRegistry() {
  if (!isTauri()) return
  if (localStorage.getItem(LIST_KEY) !== null) return
  const id = newId()
  const url = normalizeUrl(localStorage.getItem(LEGACY_BASE_KEY)) || DEFAULT_BASE
  writeList([{ id, name: serverLabel(url), url, color: DEFAULT_COLOR, icon: '', insecure: false }])
  localStorage.setItem(ACTIVE_KEY, id)
  for (const key of SESSION_KEYS) {
    const value = localStorage.getItem(key)
    if (value !== null && localStorage.getItem(`${key}::${id}`) === null) {
      localStorage.setItem(`${key}::${id}`, value)
      localStorage.removeItem(key)
    }
  }
}

export function listServers() {
  ensureRegistry()
  return readList()
}

// activeServerId is '' on web (there is only one server, and its keys are not
// namespaced). On desktop a dangling id — the active profile was deleted in
// another window — falls back to the first one rather than to no server at all.
export function activeServerId() {
  if (!isTauri()) return ''
  ensureRegistry()
  const list = readList()
  const id = localStorage.getItem(ACTIVE_KEY) || ''
  if (list.some((s) => s.id === id)) return id
  return list[0]?.id || ''
}

export function activeServer() {
  const id = activeServerId()
  if (!id) return null
  return readList().find((s) => s.id === id) || null
}

export function setActiveServerId(id) {
  if (!readList().some((s) => s.id === id)) return
  localStorage.setItem(ACTIVE_KEY, id)
}

export function addServer({ name, url, color, icon, insecure } = {}) {
  ensureRegistry()
  const clean = normalizeUrl(url)
  const profile = {
    id: newId(),
    name: (name || '').trim() || serverLabel(clean),
    url: clean,
    color: color || DEFAULT_COLOR,
    icon: icon || '',
    insecure: !!insecure,
  }
  writeList([...readList(), profile])
  return profile
}

// sameHost reports whether two addresses name the same install. The scheme is
// deliberately not part of it: putting TLS in front of a server one already has
// a session on must not sign the user out of it. An unparseable address (the
// field is free text) falls back to a literal comparison.
function sameHost(a, b) {
  if (a === b) return true
  try {
    return new URL(a).host === new URL(b).host
  } catch {
    return false
  }
}

export function updateServer(id, patch = {}) {
  const list = readList()
  const before = list.find((s) => s.id === id)
  const next = list.map((s) => {
    if (s.id !== id) return s
    const merged = { ...s, ...patch, id: s.id }
    if (patch.url !== undefined) merged.url = normalizeUrl(patch.url)
    if (patch.name !== undefined) merged.name = (patch.name || '').trim() || serverLabel(merged.url)
    return merged
  })
  writeList(next)
  const after = next.find((s) => s.id === id) || null
  // Re-pointing a profile at a different host is a *server* change wearing the
  // old profile id, so the namespace would hand the new machine a session that
  // belongs to the previous one: its refresh token posted to a host that never
  // issued it (the very thing SESSION_KEYS exists to prevent), and a cached user
  // and remembered workspace drawn over a server that has never heard of them.
  // Removal purges the namespace for this reason; so does an address edit.
  if (before && after && !sameHost(before.url, after.url)) purgeNamespace(id)
  return after
}

// purgeNamespace drops every `<key>::<id>` entry of one profile.
function purgeNamespace(id) {
  const suffix = `::${id}`
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && key.endsWith(suffix)) localStorage.removeItem(key)
  }
}

// removeServer drops the profile AND every key in its namespace. Leaving the
// session behind would be a refresh token for a server the user believes they
// removed, kept indefinitely.
//
// Removing the *last* profile resets it instead of emptying the registry: the
// session is purged as always, but a fresh default profile takes its place. The
// desktop build assumes throughout that exactly one server is active, and an
// empty list makes activeServerId() '' — which is not "no server" to the rest of
// the code, it is the *web* answer. skey() would drop back to the un-namespaced
// keys and write the next session where nothing looks for it again; serverBase()
// would fall through to DEFAULT_BASE, aiming a self-hosted desktop at the public
// instance; and setActiveServerUrl() would write the legacy key serverBase() no
// longer reads, so the address field on the login screen would stop doing
// anything. Nor is the state recoverable on its own: ensureRegistry() sees a
// non-null "[]" and does not re-seed.
export function removeServer(id) {
  const list = readList().filter((s) => s.id !== id)
  writeList(list)
  purgeNamespace(id)
  if (list.length === 0) {
    // A new id, not the removed one: the namespace has just been purged, and
    // reusing the id would hand the replacement whatever the sweep above missed.
    const fresh = {
      id: newId(),
      name: serverLabel(DEFAULT_BASE),
      url: DEFAULT_BASE,
      color: DEFAULT_COLOR,
      icon: '',
      insecure: false,
    }
    list.push(fresh)
    writeList(list)
    localStorage.setItem(ACTIVE_KEY, fresh.id)
    return list
  }
  if (localStorage.getItem(ACTIVE_KEY) === id) {
    localStorage.setItem(ACTIVE_KEY, list[0].id)
  }
  return list
}

// setActiveServerUrl backs the pre-existing single-server setting (the login
// popover and the settings screen): it re-points the active profile instead of
// writing the legacy key. On web — where there is no profile — it keeps writing
// the legacy key, so nothing about the browser build changes.
export function setActiveServerUrl(url) {
  const clean = normalizeUrl(url)
  const id = activeServerId()
  if (!id) {
    if (clean) localStorage.setItem(LEGACY_BASE_KEY, clean)
    else localStorage.removeItem(LEGACY_BASE_KEY)
    return
  }
  updateServer(id, { url: clean || DEFAULT_BASE })
}
