import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Handing a downloaded blob to the browser is a two-step dance with a race in
// it: `a.click()` only *starts* the download, and revoking the object URL
// before the browser has picked the bytes up cancels it. ConferenceRecordings
// learned that on recording files and defers its revoke by a minute, saying so
// in a comment; the two other download paths — a task attachment (up to 25 MiB,
// backend/handlers/attachments.go maxAttachmentBytes) and a document export to
// PDF/DOCX — revoked synchronously on the next line, so the same gap was wide
// open there. These tests pin the deferral on every path that hands a blob out.

const downloadAttachment = vi.fn()
const isTauri = vi.fn(() => false)

vi.mock('@/api', () => ({ tasks: { downloadAttachment: (...a) => downloadAttachment(...a) } }))
vi.mock('@/utils/serverBase', () => ({ isTauri: (...a) => isTauri(...a) }))

const { saveAttachment } = await import('@/utils/download')
const { downloadBlob } = await import('@/utils/docOffice')
const { BLOB_REVOKE_DELAY_MS, handOffBlob } = await import('@/utils/blobHandoff')

let createObjectURL
let revokeObjectURL
let anchor

beforeEach(() => {
  vi.useFakeTimers()
  createObjectURL = vi.fn(() => 'blob:x')
  revokeObjectURL = vi.fn()
  vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
  anchor = { click: vi.fn(), remove: vi.fn(), href: '', download: '' }
  vi.spyOn(document, 'createElement').mockReturnValue(anchor)
  vi.spyOn(document.body, 'appendChild').mockImplementation(() => anchor)
  isTauri.mockReturnValue(false)
  downloadAttachment.mockResolvedValue({ data: new Blob(['x']) })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('downloadBlob (document export)', () => {
  it('starts the download before it revokes anything', () => {
    downloadBlob(new Blob(['x']), 'Отчёт.pdf')

    expect(anchor.download).toBe('Отчёт.pdf')
    expect(anchor.click).toHaveBeenCalled()
    // The bug: revoked on the very next line, so a multi-megabyte export could
    // lose its bytes between the click and the browser reading them.
    expect(revokeObjectURL).not.toHaveBeenCalled()
  })

  it('still revokes once the browser has had its window', () => {
    downloadBlob(new Blob(['x']), 'Отчёт.pdf')
    vi.advanceTimersByTime(BLOB_REVOKE_DELAY_MS)

    // Deferred, not dropped: without the revoke every export leaks the whole
    // file for the lifetime of the tab.
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
  })
})

describe('saveAttachment (web path)', () => {
  it('starts the download before it revokes anything', async () => {
    await expect(saveAttachment('att-1', 'Договор.pdf')).resolves.toBe('downloaded')

    expect(anchor.download).toBe('Договор.pdf')
    expect(anchor.click).toHaveBeenCalled()
    expect(revokeObjectURL).not.toHaveBeenCalled()
  })

  it('still revokes once the browser has had its window', async () => {
    await saveAttachment('att-1', 'Договор.pdf')
    vi.advanceTimersByTime(BLOB_REVOKE_DELAY_MS)

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
  })

  it('leaves the desktop save-as path alone — it never makes an object URL', async () => {
    isTauri.mockReturnValue(true)

    // The Tauri branch dynamically imports the dialog/fs plugins, which are not
    // installed in the unit-test environment; the assertion that matters is
    // that nothing was handed to the browser as a blob URL either way.
    await saveAttachment('att-1', 'Договор.pdf').catch(() => {})

    expect(createObjectURL).not.toHaveBeenCalled()
    expect(revokeObjectURL).not.toHaveBeenCalled()
  })
})

// The desktop shell registers no download handler — wry only installs one when
// the app asks for it, and desktop/src-tauri never does — so `<a download>`
// starts nothing there and the file silently never appears. saveAttachment knew
// that and took a Save-as path; the three downloads that went through
// handOffBlob did not, and every one of them was dead in the desktop client
// (#2932). Same assertion shape as the case above: the plugins are absent from
// the unit environment, so what is pinned is that the browser dance is not the
// path taken.
describe('the desktop client never hands a blob to the browser', () => {
  beforeEach(() => {
    isTauri.mockReturnValue(true)
  })

  it('does not, for a document export', async () => {
    await downloadBlob(new Blob(['x']), 'Отчёт.docx').catch(() => {})

    expect(createObjectURL).not.toHaveBeenCalled()
    expect(anchor.click).not.toHaveBeenCalled()
  })

  it('does not, for a conference file', async () => {
    try {
      await handOffBlob(new Blob(['x']), 'скрин.png')
    } catch {
      // The dialog/fs plugins are not installed in the unit environment.
    }

    expect(createObjectURL).not.toHaveBeenCalled()
    expect(anchor.click).not.toHaveBeenCalled()
  })

  it('still uses the browser everywhere else', async () => {
    isTauri.mockReturnValue(false)

    const outcome = await handOffBlob(new Blob(['x']), 'запись.mp4')

    expect(anchor.download).toBe('запись.mp4')
    expect(anchor.click).toHaveBeenCalled()
    expect(outcome).toBe('downloaded')
  })
})
