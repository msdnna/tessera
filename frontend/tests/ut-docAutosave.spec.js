import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useDocAutosave } from '@/composables/useDocAutosave'

const doc = (t) => ({ type: 'doc', content: [{ type: 'paragraph', text: t }] })

describe('useDocAutosave', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('collapses a burst of edits into one save', async () => {
    const save = vi.fn().mockResolvedValue({ updated_at: 'v2' })
    const a = useDocAutosave(save, { delay: 800 })
    a.schedule(doc('р'))
    a.schedule(doc('ра'))
    a.schedule(doc('раз'))
    expect(save).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(800)
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenCalledWith(doc('раз'))
    expect(a.dirty.value).toBe(false)
    expect(a.savedAt.value).toBe('v2')
  })

  // Losing the last edit when the view unmounts or the route changes is the
  // failure that makes autosave worse than a save button.
  it('flush sends the pending edit without waiting for the debounce', async () => {
    const save = vi.fn().mockResolvedValue({ updated_at: 'v2' })
    const a = useDocAutosave(save, { delay: 800 })
    a.schedule(doc('раз'))
    await a.flush()
    expect(save).toHaveBeenCalledTimes(1)
    expect(a.dirty.value).toBe(false)
  })

  // Two PATCHes in flight arrive in network order, so the winner would be the
  // fastest request rather than the newest edit.
  it('serialises saves and sends the newest content last', async () => {
    const order = []
    let release
    const first = new Promise((r) => {
      release = r
    })
    const save = vi.fn().mockImplementation(async (content) => {
      order.push(content.content[0].text)
      if (order.length === 1) await first
      return { updated_at: 'v' + order.length }
    })
    const a = useDocAutosave(save, { delay: 0 })
    a.schedule(doc('первый'))
    await vi.advanceTimersByTimeAsync(0)
    a.schedule(doc('второй'))
    a.schedule(doc('третий'))
    await vi.advanceTimersByTimeAsync(0)
    expect(save).toHaveBeenCalledTimes(1)
    release({ updated_at: 'v1' })
    await vi.advanceTimersByTimeAsync(0)
    expect(order).toEqual(['первый', 'третий'])
  })

  it('stops saving after a 409 and reports the conflict', async () => {
    const err = new Error('document changed elsewhere')
    err.status = 409
    const save = vi.fn().mockRejectedValue(err)
    const a = useDocAutosave(save, { delay: 0 })
    a.schedule(doc('раз'))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.conflict.value).toBe(true)
    a.schedule(doc('два'))
    await vi.advanceTimersByTimeAsync(100)
    expect(save).toHaveBeenCalledTimes(1)
  })

  it('keeps the content queued after a non-conflict failure', async () => {
    const save = vi
      .fn()
      .mockRejectedValueOnce(new Error('сеть'))
      .mockResolvedValue({ updated_at: 'v2' })
    const a = useDocAutosave(save, { delay: 0 })
    a.schedule(doc('раз'))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.error.value).toBe('сеть')
    expect(a.dirty.value).toBe(true)
    await a.flush()
    expect(save).toHaveBeenCalledTimes(2)
    expect(a.dirty.value).toBe(false)
    expect(a.error.value).toBe('')
  })

  it('cancel drops queued content', async () => {
    const save = vi.fn().mockResolvedValue({ updated_at: 'v2' })
    const a = useDocAutosave(save, { delay: 800 })
    a.schedule(doc('раз'))
    a.cancel()
    await vi.advanceTimersByTimeAsync(2000)
    expect(save).not.toHaveBeenCalled()
    expect(a.dirty.value).toBe(false)
  })

  it('resolveConflict clears the flag and lets saving resume', async () => {
    const err = new Error('conflict')
    err.status = 409
    const save = vi.fn().mockRejectedValueOnce(err).mockResolvedValue({ updated_at: 'v3' })
    const a = useDocAutosave(save, { delay: 0 })
    a.schedule(doc('раз'))
    await vi.advanceTimersByTimeAsync(0)
    a.resolveConflict('v2')
    a.schedule(doc('два'))
    await vi.advanceTimersByTimeAsync(0)
    expect(save).toHaveBeenCalledTimes(2)
    expect(a.conflict.value).toBe(false)
    expect(a.savedAt.value).toBe('v3')
  })
})
