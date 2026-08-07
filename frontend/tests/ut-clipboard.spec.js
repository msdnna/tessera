import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { copyText } from '@/utils/clipboard'

// Swap navigator.clipboard / document.execCommand per test, restoring after.
// jsdom defines neither, so we assign them directly rather than spy.
function setClipboard(value) {
  Object.defineProperty(navigator, 'clipboard', { value, configurable: true })
}
function setExec(fn) {
  document.execCommand = fn
}

describe('copyText', () => {
  beforeEach(() => {
    setClipboard(undefined)
    setExec(vi.fn().mockReturnValue(true))
  })
  afterEach(() => {
    setClipboard(undefined)
    delete document.execCommand
  })

  it('uses the async Clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue()
    setClipboard({ writeText })
    const ok = await copyText('hello')
    expect(ok).toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(document.execCommand).not.toHaveBeenCalled()
  })

  it('falls back to execCommand when the Clipboard API rejects', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) })
    const ok = await copyText('world')
    expect(ok).toBe(true)
    expect(document.execCommand).toHaveBeenCalledWith('copy')
  })

  it('falls back to execCommand when there is no Clipboard API', async () => {
    const ok = await copyText('no-clipboard')
    expect(ok).toBe(true)
    expect(document.execCommand).toHaveBeenCalledWith('copy')
  })

  it('returns false when both paths fail', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) })
    setExec(vi.fn().mockReturnValue(false))
    expect(await copyText('nope')).toBe(false)
  })

  it('coerces nullish input to an empty string', async () => {
    const writeText = vi.fn().mockResolvedValue()
    setClipboard({ writeText })
    await copyText(null)
    expect(writeText).toHaveBeenCalledWith('')
  })
})
