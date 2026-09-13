import { describe, it, expect } from 'vitest'
import {
  ATTACHMENT_MAX_BYTES,
  MEDIA_MAX_BYTES,
  MEDIA_TYPES,
  attachmentPickError,
  mediaPickError,
} from '@/utils/upload'

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
