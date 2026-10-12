// Central resolution of the API/WS server origin.
//
//  - Plain web: same-origin. `serverBase()` is '' → axios baseURL '/api' and the
//    WebSocket is derived from `location.host`. Behaviour is byte-identical to
//    before this helper existed.
//  - Tauri desktop: the app is served from a custom protocol (tauri://localhost /
//    http://tauri.localhost), so relative '/api' and `location.host` don't reach
//    the backend. Instead we use a stored, in-app-editable base URL (set on the
//    login screen), defaulting to production. Mirrors the Android configurable
//    server-URL setting. Since #2930 that origin is the *active* entry of a
//    list of server profiles — see utils/servers.js.
import { isTauri, activeServer, setActiveServerUrl, serverUrlError, DEFAULT_BASE } from './servers'

// Re-exported so the many `import { isTauri } from '@/utils/serverBase'` call
// sites keep working; the definition moved to servers.js, which needs it and
// must not import from here (that would be a cycle).
export { isTauri }

// serverBase returns '' for web (same-origin) or an absolute origin without a
// trailing slash for desktop.
export function serverBase() {
  if (!isTauri()) return ''
  return activeServer()?.url || DEFAULT_BASE
}

// setServerBase re-points the active server profile (called from the
// login-screen popover and the settings screen). Empty restores the default.
// Answers false when the address was refused — the caller must show the error
// and NOT reload, or the app reloads against an address that was never stored.
export function setServerBase(url) {
  return setActiveServerUrl(url)
}

// Re-exported so the two address fields can name the refusal before saving,
// the way the rail's edit modal already does.
export { serverUrlError }

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
  return wsOrigin() + '/api/ws'
}

// docWsURL builds the per-document collaboration socket URL (presence + block
// locks, #2729). It is a separate connection from wsURL(): that one is
// workspace-scoped and read-only, this one is per document and two-way.
export function docWsURL(docId) {
  return `${wsOrigin()}/api/documents/${docId}/ws`
}

// confWsURL builds the per-conference room socket URL (#2869): presence, hands,
// the screen-share queue and moderation. Media does not go through it — that is
// a direct connection to the SFU, whose address the backend hands out with the
// join token.
export function confWsURL(confId) {
  return `${wsOrigin()}/api/conferences/${confId}/ws`
}

// wsOrigin is ws(s)://<host> — the current origin on web, the stored base on
// desktop.
function wsOrigin() {
  const base = serverBase()
  if (!base) {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}`
  }
  const u = new URL(base)
  const proto = u.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${u.host}`
}
