import { describe, it, expect } from 'vitest'
import { isStaleChunkError, reloadOnce, RELOAD_WINDOW_MS } from '@/utils/staleChunk'

// A tab opened before a deploy asks for chunks the server no longer has
// (Sentry: «Failed to fetch dynamically imported module»).
function memoryStorage() {
  const m = new Map()
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, String(v)) }
}

describe('stale chunk recovery', () => {
  it('recognizes the browsers’ failed-import messages', () => {
    expect(
      isStaleChunkError(
        new TypeError('Failed to fetch dynamically imported module: /assets/A-1.js'),
      ),
    ).toBe(true)
    expect(isStaleChunkError(new TypeError('error loading dynamically imported module'))).toBe(true)
    expect(isStaleChunkError(new TypeError('Importing a module script failed.'))).toBe(true)
    expect(isStaleChunkError(new Error('Request failed with status code 500'))).toBe(false)
    expect(isStaleChunkError(undefined)).toBe(false)
  })

  it('reloads a target once, then lets the error through', () => {
    const storage = memoryStorage()
    const went = []
    const go = (u) => went.push(u)
    expect(reloadOnce('/conferences', { storage, now: 1000, go })).toBe(true)
    expect(reloadOnce('/conferences', { storage, now: 2000, go })).toBe(false)
    expect(went).toEqual(['/conferences'])
    // Another target, or the same one after the window, reloads again.
    expect(reloadOnce('/documents', { storage, now: 3000, go })).toBe(true)
    expect(reloadOnce('/documents', { storage, now: 3000 + RELOAD_WINDOW_MS, go })).toBe(true)
    expect(went).toEqual(['/conferences', '/documents', '/documents'])
  })
})
