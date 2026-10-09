import { describe, it, expect, afterEach, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { setI18nLocale } from '@/i18n'
import { humanizeError } from '@/utils/errors'
import { formatElapsed } from '@/utils/duration'
import { eventText } from '@/utils/taskFeed'
import Toast from '@/components/toast/Toast.vue'
import { useToastsStore } from '@/stores/toasts'

// Wave 6 of #2799 — the GitLab journal / conflict resolver / background jobs and
// the three pure helpers behind them (errors, duration, task journal).
//
// All three helpers live OUTSIDE a setup context: they are plain modules called
// from the axios interceptor and from render code. Before this wave their text
// was module-level Russian, which reads correctly right up until the first
// language switch and then stays frozen. Everything below switches the locale at
// runtime and demands that the wording follows.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('humanizeError follows a language switch', () => {
  it('re-reads the sentinel table', async () => {
    expect(humanizeError('invalid credentials')).toBe('Неверный email или пароль')
    expect(humanizeError('FORBIDDEN')).toBe('Недостаточно прав')

    await setI18nLocale('en')
    expect(humanizeError('invalid credentials')).toBe('Wrong email or password')
    expect(humanizeError('FORBIDDEN')).toBe('Not enough permissions')
  })

  it('re-reads the validator and network fallbacks', async () => {
    const validator = "Key: 'Email' Error:Field validation for 'Email' failed on the 'email' tag"
    expect(humanizeError(validator)).toBe('Введите корректный email')
    expect(humanizeError('Network Error')).toBe('Нет связи с сервером. Проверьте подключение.')
    expect(humanizeError('')).toBe('Что-то пошло не так')

    await setI18nLocale('en')
    expect(humanizeError(validator)).toBe('Enter a valid email')
    expect(humanizeError('Network Error')).toBe('No connection to the server. Check your network.')
    expect(humanizeError('')).toBe('Something went wrong')
  })

  // The keys of the sentinel table are the wire format. A message the backend
  // has not been taught yet passes through as it came, in either language.
  it('passes an unrecognised message through untouched', async () => {
    expect(humanizeError('quota exceeded')).toBe('quota exceeded')
    await setI18nLocale('en')
    expect(humanizeError('quota exceeded')).toBe('quota exceeded')
  })

  // `low` comes straight off the server. Without an own-property check
  // "constructor" would find Object.prototype and be mapped as a sentinel.
  it('does not mistake a prototype property for a sentinel', () => {
    expect(humanizeError('constructor')).toBe('constructor')
    expect(humanizeError('toString')).toBe('toString')
  })
})

describe('formatElapsed follows a language switch', () => {
  it('re-reads the hour / minute / second units', async () => {
    expect(formatElapsed(45_000)).toBe('45 с')
    expect(formatElapsed(72_000)).toBe('1 м 12 с')
    expect(formatElapsed(3_780_000)).toBe('1 ч 3 м')

    // The tight English form is the one the backend words a sync duration in
    // (internal/notify/locale.go) and the one the bell feed and Android print —
    // see ut-notification-catalog.spec.js, which pins the two catalogs together.
    await setI18nLocale('en')
    expect(formatElapsed(45_000)).toBe('45s')
    expect(formatElapsed(72_000)).toBe('1m 12s')
    expect(formatElapsed(3_780_000)).toBe('1h 3m')
  })

  // A finished run never reads as having taken no time at all.
  it('rounds a sub-second span up instead of to zero', () => {
    expect(formatElapsed(120)).toBe('1 с')
    expect(formatElapsed(-1)).toBe('')
    expect(formatElapsed(NaN)).toBe('')
  })
})

// The sharpest form of the frozen-label trap in this wave: a toast used to be
// stamped with its wording at push() time. The activity toast (#3117) keeps
// functions instead of strings, so one already on screen speaks the new language.
describe('board activity toasts follow a language switch', () => {
  beforeEach(() => setActivePinia(createPinia()))

  function spec(over = {}) {
    const store = useToastsStore()
    return store.make({
      lane: 'top',
      kind: 'activity',
      look: 'act',
      lead: { avatar: { userId: '', name: 'Ада' } },
      bold: () => 'Ада',
      rest: () => 'x',
      ...over,
    })
  }

  it('re-renders a toast that is already on screen', async () => {
    const { i18n } = await import('@/i18n')
    const t = i18n.global.t
    const x = spec({
      rest: () => t('toasts.activity.created', { ref: 7, title: 'Задача', column: '' }),
    })
    const w = mount(Toast, { props: { toast: x } })
    expect(w.text()).toContain('создал(а) #7')
    await setI18nLocale('en')
    await w.vm.$nextTick()
    expect(w.text()).toContain('created #7')
    // The task's own title is user content and is never translated.
    expect(w.text()).toContain('Задача')
    w.unmount()
  })

  it('names an anonymous actor and the current user', async () => {
    const { i18n } = await import('@/i18n')
    const t = i18n.global.t
    const anon = mount(Toast, { props: { toast: spec({ bold: () => t('toasts.act.someone') }) } })
    const self = mount(Toast, { props: { toast: spec({ bold: () => t('toasts.act.you') }) } })
    expect(anon.text()).toContain('Кто-то')
    expect(self.text()).toContain('Вы')
    await setI18nLocale('en')
    await anon.vm.$nextTick()
    await self.vm.$nextTick()
    expect(anon.text()).toContain('Someone')
    expect(self.text()).toContain('You')
    anon.unmount()
    self.unmount()
  })
})

describe('task journal follows a language switch', () => {
  it('re-reads every event kind', async () => {
    expect(eventText({ kind: 'created' })).toBe('создал(а) задачу')
    expect(eventText({ kind: 'moved', data: { to: 'Готово' } })).toBe('переместил(а) → «Готово»')
    expect(eventText({ kind: 'relation', data: { related: 42 } })).toBe('добавил(а) связь с #42')

    await setI18nLocale('en')
    expect(eventText({ kind: 'created' })).toBe('created the task')
    expect(eventText({ kind: 'moved', data: { to: 'Готово' } })).toBe('moved it to “Готово”')
    expect(eventText({ kind: 'relation', data: { related: 42 } })).toBe('linked it to #42')
  })

  // `moved` and `attachment` have a bare and a named form. They are two keys,
  // not one plus an interpolated tail: where the name attaches is not the same
  // sentence shape in every language.
  it('keeps the bare and the named form apart in both locales', async () => {
    expect(eventText({ kind: 'moved' })).toBe('переместил(а)')
    expect(eventText({ kind: 'attachment' })).toBe('прикрепил(а) файл')

    await setI18nLocale('en')
    expect(eventText({ kind: 'moved' })).toBe('moved it')
    expect(eventText({ kind: 'attachment' })).toBe('attached a file')
    expect(eventText({ kind: 'attachment', data: { filename: 'a.pdf' } })).toBe(
      'attached the file “a.pdf”',
    )
  })

  it('names the priority in the active language', async () => {
    expect(eventText({ kind: 'priority', data: { to: 3 } })).toBe('изменил(а) приоритет → Высокий')
    await setI18nLocale('en')
    expect(eventText({ kind: 'priority', data: { to: 3 } })).toBe('changed the priority to High')
  })

  // An out-of-range level keeps printing the raw value: a number the user can
  // quote back beats silently calling it "no priority".
  it('still falls back to the raw level out of range', async () => {
    expect(eventText({ kind: 'priority', data: { to: 99 } })).toBe('изменил(а) приоритет → 99')
    await setI18nLocale('en')
    expect(eventText({ kind: 'priority', data: { to: 99 } })).toBe('changed the priority to 99')
  })
})
