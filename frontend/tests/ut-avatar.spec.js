import { describe, it, expect } from 'vitest'
import { AVATAR_MAX_BYTES, AVATAR_TYPES, avatarPickError } from '@/utils/avatar'

// The avatar picker restates two server rules (`handlers/users.go`, UploadMyAvatar):
// the 2 MiB cap and the PNG/JPEG/GIF/WebP allowlist. It restated neither, so both
// refusals reached the user as the server's raw English sentence in a Russian UI.

// A stand-in for the File the <input type="file"> hands over — avatarPickError
// only ever reads .size and .type.
const pick = (size, type) => ({ size, type, name: 'avatar' })

describe('avatar picker rules', () => {
  it('pins the cap and the allowlist to the server’s', () => {
    expect(AVATAR_MAX_BYTES).toBe(2 * 1024 * 1024)
    expect(AVATAR_TYPES).toEqual(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])
  })

  it('refuses a file over the cap before it is uploaded', () => {
    expect(avatarPickError(pick(5 * 1024 * 1024, 'image/png'))).toBe('tooLarge')
  })

  it('takes a file exactly at the cap, because the server does', () => {
    // The server's test is `> maxAvatarBytes`, so the boundary byte is legal;
    // refusing it here would bounce a picture the server would have stored.
    expect(avatarPickError(pick(AVATAR_MAX_BYTES, 'image/png'))).toBe('')
  })

  it('refuses an SVG, which the file dialog’s "All files" lets through', () => {
    expect(avatarPickError(pick(4096, 'image/svg+xml'))).toBe('badType')
  })

  it('refuses a non-image the browser has typed', () => {
    expect(avatarPickError(pick(4096, 'application/pdf'))).toBe('badType')
  })

  it('reports the size before the type, matching the server’s order', () => {
    // The server checks Size first and never reaches the sniff, so a 5 MB SVG is
    // "too large" on both sides — the user is not told to re-pick a smaller SVG.
    expect(avatarPickError(pick(5 * 1024 * 1024, 'image/svg+xml'))).toBe('tooLarge')
  })

  it('accepts every type on the allowlist', () => {
    for (const type of AVATAR_TYPES) expect(avatarPickError(pick(1024, type))).toBe('')
  })

  it('hands an untyped file to the server, which sniffs the bytes', () => {
    // The browser types a file by its extension; a .png saved without one comes
    // through as ''. The server would take it — so the picker must not refuse it.
    expect(avatarPickError(pick(1024, ''))).toBe('')
    expect(avatarPickError(pick(1024, undefined))).toBe('')
  })

  it('matches the type case-insensitively', () => {
    expect(avatarPickError(pick(1024, 'IMAGE/PNG'))).toBe('')
  })

  it('says nothing about an empty pick', () => {
    expect(avatarPickError(null)).toBe('')
    expect(avatarPickError(undefined)).toBe('')
  })
})
