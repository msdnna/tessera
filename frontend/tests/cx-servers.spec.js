import { beforeEach, afterEach, describe, it, expect } from 'vitest'

import {
  DEFAULT_BASE,
  ensureRegistry,
  listServers,
  activeServerId,
  activeServer,
  setActiveServerId,
  addServer,
  updateServer,
  removeServer,
  setActiveServerUrl,
  serverLabel,
  normalizeUrl,
} from '@/utils/servers'
import { skey, sGet, sSet, sRemove, sGetFor } from '@/utils/serverStorage'
import { serverBase, apiBaseURL, wsURL } from '@/utils/serverBase'

// The registry only exists inside the Tauri shell; every case that exercises it
// has to pretend to be one.
function desktop(on = true) {
  if (on) window.__TAURI_INTERNALS__ = {}
  else delete window.__TAURI_INTERNALS__
}

// A desktop app that has already been through the migration and has no servers
// left. Writing the (empty) list explicitly is what tells ensureRegistry() there
// is nothing to migrate — otherwise it would fold the legacy single-server
// setting into a profile #1 first, which is exactly what the migration cases
// below assert and what the rest of them are not about.
function emptyRegistry() {
  desktop()
  localStorage.setItem('tessera_servers', '[]')
}

beforeEach(() => {
  localStorage.clear()
  desktop(false)
})
afterEach(() => {
  desktop(false)
  localStorage.clear()
})

describe('server registry (#2930)', () => {
  it('stays completely inert on web', () => {
    localStorage.setItem('tessera_refresh_token', 'web-token')
    ensureRegistry()
    expect(localStorage.getItem('tessera_servers')).toBe(null)
    expect(listServers()).toEqual([])
    expect(activeServerId()).toBe('')
    // …and the session keys keep their plain names.
    expect(skey('tessera_refresh_token')).toBe('tessera_refresh_token')
    expect(sGet('tessera_refresh_token')).toBe('web-token')
    expect(serverBase()).toBe('')
    expect(apiBaseURL()).toBe('/api')
  })

  it('migrates a pre-#2930 desktop install without signing the user out', () => {
    localStorage.setItem('tessera_server_base', 'https://old.example.com/')
    localStorage.setItem('tessera_refresh_token', 'rt-1')
    localStorage.setItem('tessera_user', '{"id":"u1"}')
    localStorage.setItem('tessera_ws', 'ws-1')
    desktop()

    ensureRegistry()

    const list = listServers()
    expect(list).toHaveLength(1)
    // The trailing slash is dropped — '<base>/api' is built by concatenation.
    expect(list[0].url).toBe('https://old.example.com')
    expect(list[0].name).toBe('old.example.com')
    expect(activeServerId()).toBe(list[0].id)

    // The session moved into the profile's namespace, and is still readable
    // through the wrapper the stores use.
    const id = list[0].id
    expect(localStorage.getItem('tessera_refresh_token')).toBe(null)
    expect(localStorage.getItem(`tessera_refresh_token::${id}`)).toBe('rt-1')
    expect(sGet('tessera_refresh_token')).toBe('rt-1')
    expect(sGet('tessera_user')).toBe('{"id":"u1"}')
    expect(sGet('tessera_ws')).toBe('ws-1')

    expect(serverBase()).toBe('https://old.example.com')
    expect(apiBaseURL()).toBe('https://old.example.com/api')
    expect(wsURL()).toBe('wss://old.example.com/api/ws')
  })

  it('falls back to the built-in address when there was no stored one', () => {
    desktop()
    ensureRegistry()
    expect(listServers()[0].url).toBe(DEFAULT_BASE)
    expect(serverBase()).toBe(DEFAULT_BASE)
  })

  it('migrates only once — a second call leaves the registry alone', () => {
    localStorage.setItem('tessera_server_base', 'https://a.example.com')
    desktop()
    ensureRegistry()
    const first = listServers()[0].id
    sSet('tessera_refresh_token', 'rt-new')

    ensureRegistry()

    expect(listServers()).toHaveLength(1)
    expect(listServers()[0].id).toBe(first)
    expect(sGet('tessera_refresh_token')).toBe('rt-new')
  })

  it('keeps two servers sessions apart', () => {
    emptyRegistry()
    const a = addServer({ name: 'A', url: 'https://a.example.com' })
    const b = addServer({ name: 'B', url: 'https://b.example.com' })

    setActiveServerId(a.id)
    sSet('tessera_refresh_token', 'token-a')
    sSet('tessera_ws', 'ws-a')

    setActiveServerId(b.id)
    // B starts with nothing — A's session must not leak in.
    expect(sGet('tessera_refresh_token')).toBe(null)
    expect(sGet('tessera_ws')).toBe(null)
    sSet('tessera_refresh_token', 'token-b')

    expect(serverBase()).toBe('https://b.example.com')
    expect(sGetFor('tessera_refresh_token', a.id)).toBe('token-a')
    expect(sGetFor('tessera_refresh_token', b.id)).toBe('token-b')

    setActiveServerId(a.id)
    expect(sGet('tessera_refresh_token')).toBe('token-a')
    expect(serverBase()).toBe('https://a.example.com')
  })

  it('signing out of one server leaves the other signed in', () => {
    emptyRegistry()
    const a = addServer({ url: 'https://a.example.com' })
    const b = addServer({ url: 'https://b.example.com' })
    setActiveServerId(a.id)
    sSet('tessera_refresh_token', 'token-a')
    setActiveServerId(b.id)
    sSet('tessera_refresh_token', 'token-b')

    sRemove('tessera_refresh_token')

    expect(sGetFor('tessera_refresh_token', b.id)).toBe(null)
    expect(sGetFor('tessera_refresh_token', a.id)).toBe('token-a')
  })

  it('removing a server purges its session and re-points the active one', () => {
    emptyRegistry()
    const a = addServer({ url: 'https://a.example.com' })
    const b = addServer({ url: 'https://b.example.com' })
    setActiveServerId(b.id)
    sSet('tessera_refresh_token', 'token-b')
    sSet('tessera_user', '{"id":"u"}')

    removeServer(b.id)

    expect(listServers().map((s) => s.id)).toEqual([a.id])
    // No orphan refresh token for a server the user believes they removed.
    expect(localStorage.getItem(`tessera_refresh_token::${b.id}`)).toBe(null)
    expect(localStorage.getItem(`tessera_user::${b.id}`)).toBe(null)
    expect(activeServerId()).toBe(a.id)
  })

  it('survives a dangling active id', () => {
    emptyRegistry()
    const a = addServer({ url: 'https://a.example.com' })
    localStorage.setItem('tessera_active_server', 'ghost')
    expect(activeServerId()).toBe(a.id)
    expect(activeServer().url).toBe('https://a.example.com')
  })

  it('names a profile after its host when no name is given', () => {
    emptyRegistry()
    const s = addServer({ url: 'https://tessera.example.com/' })
    expect(s.name).toBe('tessera.example.com')
    expect(updateServer(s.id, { name: '  ' }).name).toBe('tessera.example.com')
    expect(updateServer(s.id, { name: 'Работа' }).name).toBe('Работа')
  })

  it('setServerBase re-points the active profile instead of a global key', () => {
    emptyRegistry()
    const a = addServer({ url: 'https://a.example.com' })
    setActiveServerId(a.id)
    setActiveServerUrl('https://moved.example.com/')
    expect(activeServer().url).toBe('https://moved.example.com')
    expect(listServers()).toHaveLength(1)
  })

  it('normalizes and labels addresses defensively', () => {
    expect(normalizeUrl('  https://x.example.com//  ')).toBe('https://x.example.com')
    expect(normalizeUrl(null)).toBe('')
    expect(serverLabel('not a url')).toBe('not a url')
    expect(serverLabel('')).toBe('Tessera')
  })
})
