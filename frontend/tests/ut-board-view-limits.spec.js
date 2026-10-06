import { describe, it, expect, afterEach } from 'vitest'
import { setI18nLocale } from '@/i18n'
import { humanizeError } from '@/utils/errors'

// A saved board view is bounded now — fifty per user and board, and a ceiling on
// the config blob (#2932). No client mirrors either rule, so the toolbar's
// «Сохранить вид» is where an ordinary user meets them, and `saveView` prints the
// refusal straight into a toast. Without a catalog entry that toast is an English
// sentence in a Russian interface.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('saved board view refusals', () => {
  it('words the count ceiling', () => {
    expect(humanizeError('view limit reached')).toBe(
      'Сохранённых видов доски слишком много — удалите ненужный, чтобы добавить новый',
    )
  })

  it('words the config ceiling', () => {
    expect(humanizeError('view config is too large')).toBe(
      'Настройки вида не помещаются в сохранение',
    )
  })

  it('follows a language switch like the rest of the catalog', async () => {
    await setI18nLocale('en')
    expect(humanizeError('view limit reached')).toBe(
      'Too many saved views on this board — delete one to add another',
    )
  })

  // Negative control: the entries are exact sentences, not substrings, so a
  // message that merely mentions a view passes through untouched.
  it('passes unrelated messages through', () => {
    expect(humanizeError('the view limit reached last week')).toBe(
      'the view limit reached last week',
    )
  })
})
