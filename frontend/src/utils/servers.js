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

// isTauri reports whether we're running inside the Tauri webview (v2 injects
// __TAURI_INTERNALS__; older/global builds expose __TAURI__). It lives here
// rather than in serverBase.js because the registry needs it and serverBase
// imports from this module — serverBase re-exports it, so every existing
// `import { isTauri } from '@/utils/serverBase'` keeps working.
export function isTauri() {
  return typeof window !== 'undefined' && (!!window.__TAURI_INTERNALS__ || !!window.__TAURI__)
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

export function updateServer(id, patch = {}) {
  const list = readList()
  const next = list.map((s) => {
    if (s.id !== id) return s
    const merged = { ...s, ...patch, id: s.id }
    if (patch.url !== undefined) merged.url = normalizeUrl(patch.url)
    if (patch.name !== undefined) merged.name = (patch.name || '').trim() || serverLabel(merged.url)
    return merged
  })
  writeList(next)
  return next.find((s) => s.id === id) || null
}

// removeServer drops the profile AND every key in its namespace. Leaving the
// session behind would be a refresh token for a server the user believes they
// removed, kept indefinitely.
export function removeServer(id) {
  const list = readList().filter((s) => s.id !== id)
  writeList(list)
  const suffix = `::${id}`
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && key.endsWith(suffix)) localStorage.removeItem(key)
  }
  if (localStorage.getItem(ACTIVE_KEY) === id) {
    if (list[0]) localStorage.setItem(ACTIVE_KEY, list[0].id)
    else localStorage.removeItem(ACTIVE_KEY)
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
