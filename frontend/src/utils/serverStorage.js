// Per-server view of localStorage (#2930).
//
// On desktop the app can be signed into several Tessera servers at once, so
// anything that describes a session — the refresh token, the cached user, the
// preferences, the remembered workspace, the device id — is stored under
// `<key>::<serverId>` instead of `<key>`. On web `skey()` is the identity
// function and every call below is a plain localStorage call, unchanged.
import { isTauri, activeServerId } from './servers'

export function skey(name) {
  if (!isTauri()) return name
  const id = activeServerId()
  return id ? `${name}::${id}` : name
}

export function sGet(name) {
  return localStorage.getItem(skey(name))
}

export function sSet(name, value) {
  localStorage.setItem(skey(name), value)
}

export function sRemove(name) {
  localStorage.removeItem(skey(name))
}

// sGetFor reads another server's key — used by the badge poller, which asks
// every *inactive* profile for its unread count, and by nothing else.
export function sGetFor(name, serverId) {
  return localStorage.getItem(serverId ? `${name}::${serverId}` : name)
}

export function sSetFor(name, serverId, value) {
  localStorage.setItem(serverId ? `${name}::${serverId}` : name, value)
}

export function sRemoveFor(name, serverId) {
  localStorage.removeItem(serverId ? `${name}::${serverId}` : name)
}
