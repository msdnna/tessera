// Undo toasts (v2 interaction model, #2969): the store's clock, pause, undo and
// the stack limit. The host only draws what is pinned here.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useUndoStore, UNDO_MS, DONE_MS, MAX_TOASTS } from '@/stores/undo'

describe('undo store', () => {
  let undo
  beforeEach(() => {
    setActivePinia(createPinia())
    undo = useUndoStore()
  })

  it('expires a toast after UNDO_MS, not before', () => {
    undo.push({ text: 'x', undo: vi.fn() })
    undo.tick(UNDO_MS - 100)
    expect(undo.toasts).toHaveLength(1)
    undo.tick(200)
    expect(undo.toasts).toHaveLength(0)
  })

  it('holds the timer while paused (hover and focus nest)', () => {
    const id = undo.push({ text: 'x', undo: vi.fn() })
    undo.pause(id, true)
    undo.pause(id, true)
    undo.tick(UNDO_MS * 2)
    expect(undo.toasts).toHaveLength(1)
    undo.pause(id, false)
    undo.tick(UNDO_MS * 2)
    expect(undo.toasts).toHaveLength(1)
    undo.pause(id, false)
    undo.tick(UNDO_MS)
    expect(undo.toasts).toHaveLength(0)
  })

  it('undoLast reverses the newest live toast and shows «done» briefly', async () => {
    const first = vi.fn(() => Promise.resolve())
    const second = vi.fn(() => Promise.resolve())
    undo.push({ text: 'a', undo: first })
    undo.push({ text: 'b', undo: second })
    expect(undo.undoLast()).toBe(true)
    await Promise.resolve()
    await Promise.resolve()
    expect(second).toHaveBeenCalledOnce()
    expect(first).not.toHaveBeenCalled()
    const done = undo.toasts.find((t) => t.text === 'b')
    expect(done.status).toBe('done')
    undo.tick(DONE_MS + 1)
    expect(undo.toasts.find((t) => t.text === 'b')).toBeUndefined()
  })

  it('a failed undo turns into a retry that can succeed', async () => {
    let fail = true
    const fn = vi.fn(() => (fail ? Promise.reject(new Error('net')) : Promise.resolve()))
    const id = undo.push({ text: 'x', undo: fn })
    await undo.undo(id)
    expect(undo.toasts[0].status).toBe('error')
    fail = false
    await undo.undo(id)
    expect(undo.toasts[0].status).toBe('done')
  })

  it('keeps at most MAX_TOASTS, dropping the oldest', () => {
    for (let i = 0; i < MAX_TOASTS + 2; i++) undo.push({ text: String(i), undo: vi.fn() })
    expect(undo.toasts.map((t) => t.text)).toEqual(['2', '3', '4'])
  })

  it('a busy toast outlives its timer until the undo settles', () => {
    let settle
    const id = undo.push({ text: 'x', undo: () => new Promise((r) => (settle = r)) })
    undo.undo(id)
    undo.tick(UNDO_MS * 2)
    expect(undo.toasts).toHaveLength(1)
    settle()
  })
})
