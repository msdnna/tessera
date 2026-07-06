// Central resolution of the API/WS server origin.
//
//  - Plain web: same-origin. `serverBase()` is '' → axios baseURL '/api' and the
//    WebSocket is derived from `location.host`. Behaviour is byte-identical to
//    before this helper existed.
//  - Tauri desktop: the app is served from a custom protocol (tauri://localhost /
//    http://tauri.localhost), so relative '/api' and `location.host` don't reach
//    the backend. Instead we use a stored, in-app-editable base URL (set on the
//    login screen), defaulting to production. Mirrors the Android configurable
//    server-URL setting.
const KEY = 'tessera_server_base'
const DEFAULT_BASE = 'https://tessera.msdnna.website'

// isTauri reports whether we're running inside the Tauri webview (v2 injects
// __TAURI_INTERNALS__; older/global builds expose __TAURI__).
export function isTauri() {
  return (
    typeof window !== 'undefined' && (!!window.__TAURI_INTERNALS__ || !!window.__TAURI__)
  )
}

// serverBase returns '' for web (same-origin) or an absolute origin without a
// trailing slash for desktop.
export function serverBase() {
  if (!isTauri()) return ''
  const stored = (localStorage.getItem(KEY) || '').trim().replace(/\/+$/, '')
  return stored || DEFAULT_BASE
}

// setServerBase persists the desktop server origin (called from the login-screen
// popover). Empty clears it back to the default.
export function setServerBase(url) {
  const clean = (url || '').trim().replace(/\/+$/, '')
  if (clean) localStorage.setItem(KEY, clean)
  else localStorage.removeItem(KEY)
}

// apiBaseURL is the axios baseURL: '/api' on web, '<base>/api' on desktop.
export function apiBaseURL() {
  return `${serverBase()}/api`
}

// absolutizeApiUrl prefixes a root-relative '/api/...' URL (as the backend emits
// for GitLab avatar/asset proxies and rewritten attachment links) with the
// configured server origin when running on desktop. On web it's a no-op, so
// same-origin '/api/...' URLs keep working unchanged.
export function absolutizeApiUrl(url) {
  if (!url || !isTauri()) return url
  if (url.startsWith('/api/')) return serverBase() + url
  return url
}

// wsURL builds the realtime WebSocket URL: ws(s)://<host>/api/ws. On web it uses
// the current origin; on desktop it derives host/scheme from the stored base.
export function wsURL() {
  const base = serverBase()
  if (!base) {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/api/ws`
  }
  const u = new URL(base)
  const proto = u.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${u.host}/api/ws`
}
