import { beforeEach, afterEach, describe, it, expect } from 'vitest'

import {
  DEFAULT_BASE,
  serverUrlError,
  listServers,
  activeServerId,
  addServer,
  setActiveServerId,
  setActiveServerUrl,
} from '@/utils/servers'
import { sSet, sGet } from '@/utils/serverStorage'
import { setServerBase, serverBase } from '@/utils/serverBase'

// The registry only exists inside the Tauri shell.
function desktop() {
  window.__TAURI_INTERNALS__ = {}
  localStorage.setItem('tessera_servers', '[]')
}

// A signed-in desktop profile: the active server plus the session keys that
// utils/servers.js namespaces to it.
function signedInProfile(url) {
  const p = addServer({ name: 'Self-hosted', url })
  setActiveServerId(p.id)
  sSet('tessera_refresh_token', 'rt-live')
  sSet('tessera_user', '{"id":"u1"}')
  sSet('tessera_ws', 'ws-1')
  return p
}

beforeEach(() => {
  localStorage.clear()
})
afterEach(() => {
  delete window.__TAURI_INTERNALS__
  localStorage.clear()
})

describe('server address guard (#2932)', () => {
  it('accepts an http(s) origin and names why anything else is refused', () => {
    expect(serverUrlError('https://tessera.example.com')).toBe('')
    expect(serverUrlError('http://192.168.1.10:8090')).toBe('')
    // Trailing slashes are normalised away, not an error.
    expect(serverUrlError('https://tessera.example.com/')).toBe('')
    expect(serverUrlError('')).toBe('empty')
    expect(serverUrlError('   ')).toBe('empty')
    // The natural typo: a host with no scheme is not a URL at all.
    expect(serverUrlError('tessera.example.com')).toBe('parse')
    // Parses, but not something the app can talk to.
    expect(serverUrlError('ftp://tessera.example.com')).toBe('scheme')
    expect(serverUrlError('javascript:alert(1)')).toBe('scheme')
  })

  it('refuses a scheme-less address instead of storing it', () => {
    desktop()
    const p = signedInProfile('https://mine.example.com')

    expect(setActiveServerUrl('mine.example.com')).toBe(false)

    // The profile still points where it did — a relative baseURL would send
    // every request into the tauri:// origin and wsURL() would throw on it.
    expect(listServers().find((s) => s.id === p.id).url).toBe('https://mine.example.com')
    expect(serverBase()).toBe('https://mine.example.com')
  })

  it('keeps the session when the typed address is refused', () => {
    desktop()
    signedInProfile('https://mine.example.com')

    setActiveServerUrl('mine.example.com')

    // updateServer() purges the profile's namespace whenever the host changes,
    // and an unparseable address reads as "different host" — so committing the
    // typo would sign the user out of the server they were aiming at.
    expect(sGet('tessera_refresh_token')).toBe('rt-live')
    expect(sGet('tessera_user')).toBe('{"id":"u1"}')
    expect(sGet('tessera_ws')).toBe('ws-1')
  })

  it('still re-points the profile at a valid new address', () => {
    desktop()
    const p = signedInProfile('https://mine.example.com')

    expect(setActiveServerUrl('https://other.example.com/')).toBe(true)

    expect(listServers().find((s) => s.id === p.id).url).toBe('https://other.example.com')
    // A genuine host change is still a session change: the namespace goes.
    expect(sGet('tessera_refresh_token')).toBe(null)
  })

  it('still restores the built-in default on an empty address', () => {
    desktop()
    const p = signedInProfile('https://mine.example.com')

    expect(setActiveServerUrl('')).toBe(true)

    expect(listServers().find((s) => s.id === p.id).url).toBe(DEFAULT_BASE)
  })

  it('reports the same verdict through setServerBase, which the two legacy fields call', () => {
    desktop()
    signedInProfile('https://mine.example.com')

    // The login-screen popover and the settings screen go through this one, and
    // it is the boolean they have to branch on before reloading the app.
    expect(setServerBase('mine.example.com')).toBe(false)
    expect(setServerBase('https://mine.example.com')).toBe(true)
  })

  it('refuses a bad address on web too, without touching the legacy key', () => {
    // No Tauri: there is no profile, and setActiveServerUrl falls back to the
    // pre-#2930 single-server key. It must not land a broken value there either.
    localStorage.setItem('tessera_server_base', 'https://mine.example.com')

    expect(setActiveServerUrl('mine.example.com')).toBe(false)
    expect(localStorage.getItem('tessera_server_base')).toBe('https://mine.example.com')

    expect(activeServerId()).toBe('')
  })
})
