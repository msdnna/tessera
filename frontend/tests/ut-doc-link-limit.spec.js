import { describe, it, expect, afterEach } from 'vitest'
import { setI18nLocale } from '@/i18n'
import { humanizeError } from '@/utils/errors'

// A document's task links are bounded now — five hundred per document (#2932).
// No client mirrors the rule, so the links panel is where an ordinary reader
// meets it: `onLink` in DocumentsView prints `e.message` into a toast, and that
// message is whatever the API interceptor made of the server's sentinel. Without
// a catalog entry the toast is `link limit reached` in a Russian interface.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('document link ceiling refusal', () => {
  it('words the ceiling', () => {
    expect(humanizeError('link limit reached')).toBe(
      'К документу привязано слишком много задач — отвяжите ненужные',
    )
  })

  it('follows a language switch like the rest of the catalog', async () => {
    await setI18nLocale('en')
    expect(humanizeError('link limit reached')).toBe(
      'Too many tasks linked to this document — unlink some first',
    )
  })

  // The anchor bound's refusal is deliberately NOT in the catalog: the block id
  // is drawn by the editor and never typed, so a client that trips it is broken
  // rather than full. Pinned as a decision — an entry appearing here later should
  // be a choice, not a reflex.
  it('leaves the anchor refusal to the generic line', () => {
    expect(humanizeError('block id is too long')).toBe('block id is too long')
  })

  // Negative control: entries are exact sentences, not substrings.
  it('passes unrelated messages through', () => {
    expect(humanizeError('the link limit reached last week')).toBe(
      'the link limit reached last week',
    )
  })
})
