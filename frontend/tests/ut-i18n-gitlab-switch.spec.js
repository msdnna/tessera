import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setI18nLocale } from '@/i18n'
import { humanizeError } from '@/utils/errors'
import { formatElapsed } from '@/utils/duration'
import { eventText } from '@/utils/taskFeed'
import BoardActivityToasts from '@/components/BoardActivityToasts.vue'

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

    await setI18nLocale('en')
    expect(formatElapsed(45_000)).toBe('45 s')
    expect(formatElapsed(72_000)).toBe('1 m 12 s')
    expect(formatElapsed(3_780_000)).toBe('1 h 3 m')
  })

  // A finished run never reads as having taken no time at all.
  it('rounds a sub-second span up instead of to zero', () => {
    expect(formatElapsed(120)).toBe('1 с')
    expect(formatElapsed(-1)).toBe('')
    expect(formatElapsed(NaN)).toBe('')
  })
})

// The sharpest form of the frozen-label trap in this wave: a toast used to be
// stamped with its wording at push() time. A toast only lives 6.5 s, but the
// stack is mounted for as long as the board is — so a label captured on push
// outlives a language switch that happens while it is on screen.
describe('board activity toasts follow a language switch', () => {
  const activity = { id: 't1', number: 7, title: 'Задача', verb: 'created', actorName: 'Ада' }

  it('re-renders a toast that is already on screen', async () => {
    const w = mount(BoardActivityToasts)
    w.vm.push(activity)
    await w.vm.$nextTick()
    expect(w.text()).toContain('создал(а) задачу')
    expect(w.text()).toContain('Открыть')

    await setI18nLocale('en')
    expect(w.text()).toContain('created a task')
    expect(w.text()).toContain('Open')
    // The task's own title is user content and is never translated.
    expect(w.text()).toContain('Задача')
    w.unmount()
  })

  it('falls back to the generic verb for an event kind it does not know', async () => {
    const w = mount(BoardActivityToasts)
    w.vm.push({ ...activity, verb: 'teleported' })
    await w.vm.$nextTick()
    expect(w.text()).toContain('изменил(а) задачу')
    w.unmount()
  })

  it('names an anonymous actor and the current user', async () => {
    const w = mount(BoardActivityToasts)
    w.vm.push({ ...activity, actorName: null })
    w.vm.push({ ...activity, self: true })
    await w.vm.$nextTick()
    expect(w.text()).toContain('Кто-то')
    expect(w.text()).toContain('Вы')

    await setI18nLocale('en')
    expect(w.text()).toContain('Someone')
    expect(w.text()).toContain('You')
    w.unmount()
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
