import { describe, it, expect } from 'vitest'
import {
  ATTACHMENT_MAX_BYTES,
  MEDIA_MAX_BYTES,
  MEDIA_TYPES,
  attachmentPickError,
  attachmentSizeText,
  mediaPickError,
  mediaPickMessage,
} from '@/utils/upload'
import { formatFileSize } from '@/utils/docPdf'
import { i18n, setI18nLocale } from '@/i18n'

// The two task upload paths restate the server's rules (`handlers/attachments.go`):
// 25 MiB for an attachment, 8 MiB + a PNG/JPEG/GIF/WebP/BMP sniff allowlist for an
// inline image. Neither restated anything, so the file travelled the wire in full
// and the refusal came back as the handler's hardcoded Russian sentence — rendered
// verbatim in an English UI, because errors.js knows no sentinel for it.

// A stand-in for the File the picker / drop / paste hands over — the rules only
// ever read .size and .type.
const pick = (size, type) => ({ size, type, name: 'file' })

describe('task attachment rules', () => {
  it('pins the cap to the server’s maxAttachmentBytes', () => {
    expect(ATTACHMENT_MAX_BYTES).toBe(25 * 1024 * 1024)
  })

  it('refuses a file over the cap before it is uploaded', () => {
    expect(attachmentPickError(pick(40 * 1024 * 1024, 'application/zip'))).toBe('tooLarge')
  })

  it('takes a file exactly at the cap, because the server does', () => {
    // The server's test is `> maxAttachmentBytes`, so the boundary byte is legal;
    // refusing it here would bounce a file the server would have stored.
    expect(attachmentPickError(pick(ATTACHMENT_MAX_BYTES, 'application/zip'))).toBe('')
  })

  it('says nothing about the type — an attachment may be anything', () => {
    // Unlike the media route, the attachment handler has no allowlist at all: it
    // stores whatever arrives. A picker stricter than that would lose real files.
    for (const type of ['image/svg+xml', 'application/pdf', 'application/x-msdownload', ''])
      expect(attachmentPickError(pick(1024, type))).toBe('')
  })

  it('says nothing about an empty pick', () => {
    expect(attachmentPickError(null)).toBe('')
    expect(attachmentPickError(undefined)).toBe('')
  })
})

describe('inline image rules', () => {
  it('pins the cap and the allowlist to the server’s', () => {
    expect(MEDIA_MAX_BYTES).toBe(8 * 1024 * 1024)
    expect(MEDIA_TYPES).toEqual(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/bmp'])
  })

  it('refuses an image over the cap before it is uploaded', () => {
    // A phone screenshot or a camera JPEG clears 8 MiB easily, and the editor
    // uploads it on paste — the slowest possible way to learn it was refused.
    expect(mediaPickError(pick(12 * 1024 * 1024, 'image/png'))).toBe('tooLarge')
  })

  it('takes an image exactly at the cap, because the server does', () => {
    expect(mediaPickError(pick(MEDIA_MAX_BYTES, 'image/jpeg'))).toBe('')
  })

  it('refuses an SVG, which `image/*` alone lets through', () => {
    // The editor routed anything typed image/* to the media route, and SVG is
    // absent from the server's allowlist on purpose (script-bearing document on
    // the app's own origin).
    expect(mediaPickError(pick(4096, 'image/svg+xml'))).toBe('badType')
  })

  it('reports the size before the type, matching the server’s order', () => {
    // The server checks Size first and never reaches the sniff, so a 12 MB SVG is
    // "too large" on both sides — the user is not invited to re-pick a smaller SVG.
    expect(mediaPickError(pick(12 * 1024 * 1024, 'image/svg+xml'))).toBe('tooLarge')
  })

  it('accepts every type on the allowlist', () => {
    for (const type of MEDIA_TYPES) expect(mediaPickError(pick(1024, type))).toBe('')
  })

  it('accepts BMP, which the allowlist carries and the avatar one does not', () => {
    // The two allowlists genuinely differ; copying the avatar rule here would
    // bounce a BMP the media route accepts.
    expect(mediaPickError(pick(1024, 'image/bmp'))).toBe('')
  })

  it('hands an untyped file to the server, which sniffs the bytes', () => {
    expect(mediaPickError(pick(1024, ''))).toBe('')
    expect(mediaPickError(pick(1024, undefined))).toBe('')
  })

  it('matches the type case-insensitively', () => {
    expect(mediaPickError(pick(1024, 'IMAGE/PNG'))).toBe('')
  })

  it('says nothing about an empty pick', () => {
    expect(mediaPickError(null)).toBe('')
    expect(mediaPickError(undefined)).toBe('')
  })
})

// The reason code above is only half the rule: every caller then has to turn it
// into a sentence, and each one that writes that ternary itself is a place the
// wording can drift. The document editor had no rule at all, so a photo dropped
// onto a page was refused by the asset route's own hardcoded *Russian* string —
// in whatever language the reader had chosen.
describe('inline image refusal, phrased', () => {
  it('says nothing when the file is fine', () => {
    expect(mediaPickMessage(pick(1024, 'image/png'))).toBe('')
    expect(mediaPickMessage(null)).toBe('')
  })

  it('states the cap in the reader’s own units, not the server’s', () => {
    // The server answers "файл слишком большой" — no number at all, and in one
    // fixed language. The point of saying it here is that it can carry both.
    const msg = mediaPickMessage(pick(12 * 1024 * 1024, 'image/png'))
    expect(msg).toBe(i18n.global.t('documents.editor.error.imageTooLarge', { limit: '8 МБ' }))
    expect(msg).toContain('8 МБ')
  })

  it('names the type as the reason when the type is what is wrong', () => {
    expect(mediaPickMessage(pick(4096, 'image/svg+xml'))).toBe(
      i18n.global.t('documents.editor.error.imageType'),
    )
  })

  it('follows the language the reader switched to', async () => {
    // Same failure as #2799: a module-level lookup would freeze the sentence in
    // whichever locale happened to be active when the bundle loaded. The unit
    // has to follow too — it is formatted, not baked into the phrase.
    try {
      await setI18nLocale('en')
      const msg = mediaPickMessage(pick(12 * 1024 * 1024, 'image/png'))
      expect(msg).toBe(i18n.global.t('documents.editor.error.imageTooLarge', { limit: '8 MB' }))
      expect(msg).not.toContain('МБ')
    } finally {
      await setI18nLocale('ru')
    }
  })
})

// The size of an attachment already uploaded — the label under its name in the
// files tab. It used to be written by hand there with `.toFixed()`, i.e. with a
// decimal point, while every other place the same file is weighed (a document's
// PDF block, a call's chat rail, and the "file is larger than" refusal in this
// very tab) goes through Intl.NumberFormat.
describe('attachment size label', () => {
  it('writes the separator in the language the reader is reading', () => {
    // The Russian comma is not decoration: `formatFileSize` is what puts it on
    // the same file's size two lines away in the same component.
    expect(attachmentSizeText(1.4 * 1024 * 1024)).toBe('1,4 МБ')
    expect(attachmentSizeText(1.4 * 1024 * 1024)).toContain(formatFileSize(1.4 * 1024 * 1024))
  })

  it('keeps one fraction digit on anything scaled', () => {
    // toFixed(0) on kilobytes read 14.6 KB as a flat "15 КБ" — a number that
    // disagreed with the documents rail about the very same file.
    expect(attachmentSizeText(15000)).toBe('14,6 КБ')
    expect(attachmentSizeText(2 * 1024 * 1024)).toBe('2 МБ')
  })

  it('never puts a fraction on whole bytes', () => {
    expect(attachmentSizeText(0)).toBe('0 Б')
    expect(attachmentSizeText(512)).toBe('512 Б')
    // Four digits are grouped the way the locale groups them, which is what the
    // documents rail does with the same number — «1 023», not «1023».
    expect(attachmentSizeText(1023)).toBe(formatFileSize(1023))
  })

  it('follows the language the reader switched to', async () => {
    try {
      await setI18nLocale('en')
      expect(attachmentSizeText(1.4 * 1024 * 1024)).toBe('1.4 MB')
      expect(attachmentSizeText(15000)).toBe('14.6 KB')
    } finally {
      await setI18nLocale('ru')
    }
  })
})
