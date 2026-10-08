// The password rule, mirrored from the server (#2932).
//
// Two bounds in two different units, which is the whole reason this is a module
// and not an inline `length >= 8`: the minimum is characters (gin's validator
// counts runes) and the ceiling is bytes (bcrypt's own, 72). Before this existed
// three views each knew the minimum, none knew the ceiling, and a 37-letter
// Russian passphrase — 74 bytes, a perfectly ordinary password — came back from
// the server as "internal error".
import { describe, it, expect } from 'vitest'
import {
  MAX_PASSWORD_BYTES,
  MIN_PASSWORD_CHARS,
  isPasswordValid,
  passwordBytes,
  passwordChars,
  passwordError,
  passwordProblem,
  passwordStrength,
} from '@/utils/password'

// 37 Cyrillic letters = 74 bytes. The reachable case, and the one every inline
// rule accepted.
const cyrillic = (n) => 'п'.repeat(n)

describe('passwordProblem — the ceiling nobody had written down', () => {
  it('refuses a password bcrypt cannot hash', () => {
    expect(passwordProblem('a'.repeat(73))).toBe('long')
    expect(passwordProblem(cyrillic(37))).toBe('long')
    expect(passwordProblem('correct-horse-'.repeat(8))).toBe('long')
  })

  it('accepts a password exactly at the ceiling', () => {
    // The negative control for the rule above: refusing 72 bytes would lock out
    // passwords the server stores happily.
    expect(passwordProblem('a'.repeat(72))).toBeNull()
    expect(passwordProblem(cyrillic(36))).toBeNull()
  })

  it('counts the ceiling in bytes, not in characters', () => {
    // 40 Cyrillic letters are well under any character count one might pick and
    // still over the ceiling — this is what a `max=72` rune rule would miss.
    expect(passwordChars(cyrillic(40))).toBe(40)
    expect(passwordBytes(cyrillic(40))).toBe(80)
    expect(passwordProblem(cyrillic(40))).toBe('long')
  })
})

describe('passwordProblem — the minimum', () => {
  it('refuses anything shorter than the minimum', () => {
    expect(passwordProblem('short')).toBe('short')
    expect(passwordProblem('')).toBe('short')
    expect(passwordProblem(cyrillic(7))).toBe('short')
  })

  it('accepts the minimum exactly', () => {
    expect(passwordProblem('password')).toBeNull()
    expect(passwordProblem(cyrillic(8))).toBeNull()
  })

  it('counts the minimum in characters, not in UTF-16 units', () => {
    // Four astral characters are eight UTF-16 units, so `.length >= 8` called
    // this long enough while the server counted four runes and refused it.
    const emoji = '🔐'.repeat(4)
    expect(emoji.length).toBe(8)
    expect(passwordChars(emoji)).toBe(4)
    expect(passwordProblem(emoji)).toBe('short')
  })

  it('does not trim — spaces are part of a passphrase', () => {
    expect(passwordProblem('   a    ')).toBeNull()
  })
})

describe('passwordProblem — inputs a form can hand it', () => {
  it('treats nothing as too short rather than throwing', () => {
    expect(passwordProblem(null)).toBe('short')
    expect(passwordProblem(undefined)).toBe('short')
  })
})

describe('isPasswordValid', () => {
  it('is the boolean the form fields need', () => {
    expect(isPasswordValid('password-123')).toBe(true)
    expect(isPasswordValid('short')).toBe(false)
    expect(isPasswordValid(cyrillic(37))).toBe(false)
  })
})

describe('passwordError', () => {
  it('says nothing about a password that is fine', () => {
    expect(passwordError('password-123')).toBe('')
  })

  it('tells the two ends apart', () => {
    const short = passwordError('abc')
    const long = passwordError(cyrillic(37))
    expect(short).toBeTruthy()
    expect(long).toBeTruthy()
    expect(short).not.toBe(long)
    // The message must be a real translation, not a missing-key echo — that is
    // the failure mode a new locale key has.
    expect(long).not.toContain('passwordTooLong')
  })

  it('states the ceiling in the unit it is measured in', () => {
    expect(passwordError(cyrillic(37))).toContain(String(MAX_PASSWORD_BYTES))
  })
})

describe('the constants are the server-side ones', () => {
  it('pins both bounds so a drift is a failing test, not a 500', () => {
    expect(MIN_PASSWORD_CHARS).toBe(8)
    expect(MAX_PASSWORD_BYTES).toBe(72)
  })
})

// The meter under a new-password field (#3107): advice only, never a rule.
describe('passwordStrength', () => {
  it('is 0 for nothing and 1 for a password the server would refuse', () => {
    expect(passwordStrength('')).toBe(0)
    expect(passwordStrength('short')).toBe(1)
  })

  it('grows with length and with a mix of kinds', () => {
    expect(passwordStrength('abcdefgh')).toBe(1)
    expect(passwordStrength('abcdefghijkl')).toBe(2)
    expect(passwordStrength('abcdefghijklmnop')).toBe(3)
    expect(passwordStrength('Abcdefgh1')).toBe(2)
    expect(passwordStrength('Sprint42-design!')).toBe(4)
  })

  it('counts Cyrillic letters by case like any other alphabet', () => {
    expect(passwordStrength('Пароль2026')).toBe(2)
  })
})
