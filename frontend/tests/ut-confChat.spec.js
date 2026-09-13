import { describe, it, expect } from 'vitest'
import {
  CONF_CHAT_MAX_BODY,
  CONF_CHAT_MAX_BYTES,
  CONF_CHAT_MAX_FILES,
  confChatBodyLength,
  confChatBodyTooLong,
  confChatCanSend,
} from '@/utils/confChat'

// The composer restates three server rules (`handlers/conference_chat.go`): the
// body length, the per-file size and the file count. Two of them were mirrored
// in the component from the start; the third was not, and the server's answer to
// it is a raw English 400 landing in a Russian UI in the middle of a call.

describe('conference chat composer rules', () => {
  it('pins the three caps to the server’s', () => {
    expect(CONF_CHAT_MAX_BODY).toBe(4000)
    expect(CONF_CHAT_MAX_FILES).toBe(5)
    expect(CONF_CHAT_MAX_BYTES).toBe(25 * 1024 * 1024)
  })

  describe('confChatBodyLength', () => {
    it('counts code points, the way Go counts runes', () => {
      // Four emoji: eight UTF-16 units, four runes. Counting the JS way would
      // make the composer stricter than the server.
      const emoji = '🙂🙃🙂🙃'
      expect(emoji.length).toBe(8)
      expect(confChatBodyLength(emoji)).toBe(4)
    })

    it('trims first, because the server trims before it counts', () => {
      expect(confChatBodyLength('   ok   ')).toBe(2)
      expect(confChatBodyLength('   \n  ')).toBe(0)
    })

    it('survives an empty draft', () => {
      expect(confChatBodyLength('')).toBe(0)
      expect(confChatBodyLength(null)).toBe(0)
      expect(confChatBodyLength(undefined)).toBe(0)
    })
  })

  describe('confChatBodyTooLong', () => {
    it('accepts a body exactly at the cap', () => {
      expect(confChatBodyTooLong('a'.repeat(CONF_CHAT_MAX_BODY))).toBe(false)
    })

    it('refuses one code point past it', () => {
      expect(confChatBodyTooLong('a'.repeat(CONF_CHAT_MAX_BODY + 1))).toBe(true)
    })

    it('does not count surrounding whitespace against the cap', () => {
      const body = `  ${'a'.repeat(CONF_CHAT_MAX_BODY)}  `
      expect(body.length).toBeGreaterThan(CONF_CHAT_MAX_BODY)
      expect(confChatBodyTooLong(body)).toBe(false)
    })

    it('measures an all-emoji body in runes, so 4000 of them still fit', () => {
      // 4000 runes = 8000 UTF-16 units. `String.length` would call this twice
      // the cap and refuse a message the server accepts.
      expect(confChatBodyTooLong('🙂'.repeat(CONF_CHAT_MAX_BODY))).toBe(false)
      expect(confChatBodyTooLong('🙂'.repeat(CONF_CHAT_MAX_BODY + 1))).toBe(true)
    })
  })

  describe('confChatCanSend', () => {
    // The rule that was missing from the web composer.
    it('refuses to send an over-long body', () => {
      expect(confChatCanSend('a'.repeat(CONF_CHAT_MAX_BODY + 1), 0, false)).toBe(false)
    })

    it('refuses an over-long body even when files are attached', () => {
      // The 400 rejects the whole message, files included — enabling the button
      // because something is attached would just lose the upload too.
      expect(confChatCanSend('a'.repeat(CONF_CHAT_MAX_BODY + 1), 2, false)).toBe(false)
    })

    it('allows a body exactly at the cap', () => {
      expect(confChatCanSend('a'.repeat(CONF_CHAT_MAX_BODY), 0, false)).toBe(true)
    })

    // Negative controls: the parts that already worked must keep working.
    it('allows ordinary text', () => {
      expect(confChatCanSend('ага', 0, false)).toBe(true)
    })

    it('allows a file with no text', () => {
      expect(confChatCanSend('', 1, false)).toBe(true)
      expect(confChatCanSend('   ', 1, false)).toBe(true)
    })

    it('refuses an empty message with nothing attached', () => {
      expect(confChatCanSend('', 0, false)).toBe(false)
      expect(confChatCanSend('   \n ', 0, false)).toBe(false)
    })

    it('refuses while a send is in flight', () => {
      expect(confChatCanSend('ага', 0, true)).toBe(false)
      expect(confChatCanSend('', 1, true)).toBe(false)
    })
  })
})
