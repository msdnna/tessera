import { describe, it, expect, afterEach } from 'vitest'
import { setI18nLocale } from '@/i18n'
import { humanizeError } from '@/utils/errors'

// The rate limiter's refusal (backend middleware.RateLimit). Unlike most of the
// sentinel catalogue this one is not a stale-tab rarity: the login route is
// throttled at ten attempts a minute keyed by IP *and* by email, so an office
// behind one address shares a single budget and an ordinary mistyped password
// reaches it. It used to fall past the table and be printed verbatim — a bare
// English "too many requests" on a Russian login form, which does not even say
// that waiting fixes it (#2932).

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('rate-limit refusal', () => {
  it('says what to do about it, not just that it happened', () => {
    expect(humanizeError('too many requests')).toBe(
      'Слишком много попыток — подождите минуту и попробуйте снова',
    )
  })

  it('is matched case-insensitively like every other sentinel', () => {
    expect(humanizeError('Too Many Requests')).toBe(
      'Слишком много попыток — подождите минуту и попробуйте снова',
    )
  })

  it('follows a language switch like the rest of the catalog', async () => {
    await setI18nLocale('en')
    expect(humanizeError('too many requests')).toBe(
      'Too many attempts — wait a minute and try again',
    )
  })

  it('keeps the two saved-view bounds translated too', () => {
    // Same class, same reason — no client mirrors either rule, so «Сохранить вид»
    // is where they are met.
    expect(humanizeError('view limit reached')).toBe(
      'Сохранённых видов доски слишком много — удалите ненужный, чтобы добавить новый',
    )
    expect(humanizeError('view config is too large')).toBe(
      'Настройки вида не помещаются в сохранение',
    )
  })
})
