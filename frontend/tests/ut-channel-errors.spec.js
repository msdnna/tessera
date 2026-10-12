import { describe, it, expect, afterEach } from 'vitest'
import { setI18nLocale } from '@/i18n'
import { humanizeError } from '@/utils/errors'

// The notification-channel editor is the one place where a user meets backend
// refusals *while the form is open* — no client mirrors the channel rules, so
// every one of them comes from the server. They used to arrive as hardcoded
// Russian sentences and were printed verbatim under the form (chErr), which is
// how an English UI ended up with Russian lines it could not translate (#2932).
// They now travel as English sentinels, like every other refusal on this API.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('notification channel refusals', () => {
  it('names the channel type the server does not know', () => {
    expect(humanizeError('unknown channel type: "carrier-pigeon"')).toBe(
      'Сервер не умеет каналы типа «carrier-pigeon»',
    )
  })

  it('names the config field a channel is missing', () => {
    expect(humanizeError('email channel requires address')).toBe(
      'Каналу «email» нужно поле address',
    )
    expect(humanizeError('telegram channel requires bot_token')).toBe(
      'Каналу «telegram» нужно поле bot_token',
    )
  })

  it('words the template ceiling', () => {
    expect(humanizeError('template exceeds 8192 characters')).toBe(
      'Шаблон длиннее 8192 символов — сократите его',
    )
  })

  it("keeps the parser's own words but labels them", () => {
    // The tail names the offending action and has no translation — and it keeps
    // its original case, which a lowercased match would have destroyed.
    expect(humanizeError('template error: template: msg:1: unclosed action')).toBe(
      'Ошибка в шаблоне: template: msg:1: unclosed action',
    )
  })

  it('translates the SMTP warning that rides a successful probe', () => {
    expect(humanizeError('smtp is not configured')).toBe(
      'SMTP на сервере не настроен — письмо записано в лог, но не отправлено',
    )
  })

  it('follows a language switch like the rest of the catalog', async () => {
    await setI18nLocale('en')
    expect(humanizeError('email channel requires address')).toBe(
      'The “email” channel needs address',
    )
    expect(humanizeError('unknown channel type: "carrier-pigeon"')).toBe(
      'The server has no channel of type “carrier-pigeon”',
    )
    expect(humanizeError('smtp is not configured')).toBe(
      'SMTP is not configured on the server — the email was logged, not sent',
    )
  })

  // Negative controls: the patterns are narrow enough not to swallow a message
  // that merely looks like one of them.
  it('passes unrelated messages through', () => {
    expect(humanizeError('the channel requires maintenance')).toBe(
      'the channel requires maintenance',
    )
    expect(humanizeError('template exceeds expectations')).toBe('template exceeds expectations')
    expect(humanizeError('unknown channel type')).toBe('unknown channel type')
  })
})
