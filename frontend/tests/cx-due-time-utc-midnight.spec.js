// Время суток, которое в браузерной зоне попадает ровно на UTC-полночь (#2932).
//
// «Чистая UTC-полночь — это дата без времени» — сквозная договорённость всего
// приложения (utils/format `isDateOnly`, timeAxis, utils/dueModel `readEndpoint`):
// такой инстант читается в UTC в любой зоне, иначе пользователи восточнее и
// западнее UTC видели бы соседний день (грабли 7 из #2796). Договорённость нужна
// для дат, пришедших из GitLab и из легаси, — у них времени нет вовсе.
//
// Но редактор срока (тогда DueEditor, теперь DuePicker, #3118) ничего не делал, чтобы НЕ породить такой инстант самому. Он
// строит момент из локальных календарных полей и отправляет его как ISO, поэтому
// ровно одно время суток в каждой зоне со сдвигом на целый час (и на 30/45 минут)
// уезжает в date-only:
//
//   Нью-Йорк летом (UTC−4)  20:00 → 00:00Z следующего дня
//   Нью-Йорк зимой (UTC−5)  19:00 → 00:00Z следующего дня
//   Москва (UTC+3)          03:00 → 00:00Z того же дня
//   Калькутта (UTC+5:30)    05:30 → 00:00Z того же дня
//
// Последствие пользователь видит сразу и в нескольких местах: срок «15 июля,
// 20:00» превращается в «16 июля» без времени — на карточке, в календарной
// раскладке, в фасетах «Сегодня/Завтра», — а сам редактор при перечитывании
// пропса прыгает на 16-е, 00:00. Время, которое человек только что выбрал,
// потеряно молча.
//
// Зона прогона зафиксирована: на UTC-раннере сдвига нет и спека была бы зелёной.
const TZ_BEFORE = process.env.TZ
process.env.TZ = 'America/New_York' // −04:00 летом, −05:00 зимой

import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, reactive, nextTick } from 'vue'
import { loadPicker, day } from './helpers/duePicker'
import { setActivePinia, createPinia } from 'pinia'

afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
  projects: { milestones: vi.fn(() => Promise.resolve({ data: [] })) },
  milestones: {},
  gitlab: { projectConfig: vi.fn(() => Promise.reject(new Error('no integration'))) },
}))

const DuePicker = await loadPicker()
const { isDateOnly } = await import('@/utils/format')

let wrapper = null
let state = null

// Хост повторяет TaskModal.onDueApply: он кладёт эмитнутый патч обратно в пропсы,
// как это делает настоящий родитель после PATCH'а. Без этого круг «выбрал время →
// перечитал задачу» не воспроизводится.
function mountHost(initial = {}) {
  state = reactive({ due: null, start: null, recurrence: null, ...initial })
  wrapper = mount(
    {
      render: () =>
        h(DuePicker, {
          show: true,
          due: state.due,
          start: state.start,
          recurrence: state.recurrence,
          onApply: (p) => {
            state.due = p.due_date ? new Date(p.due_date).getTime() : null
            state.start = p.start_date ? new Date(p.start_date).getTime() : null
            state.recurrence = p.recurrence
          },
        }),
    },
    { attachTo: document.body },
  )
  return wrapper
}

const editor = (w) => w.findComponent(DuePicker)
const model = (w) => editor(w).vm.model

function lastApply(w) {
  const events = editor(w).emitted('apply') || []
  return events.length ? events[events.length - 1][0] : null
}

// Как приложение прочитает сохранённый срок: date-only — всегда в UTC, иначе в
// зоне пользователя (utils/format `zoneOf`). Здесь профиль совпадает с браузером.
function asUserSees(iso) {
  const d = new Date(iso)
  const zone = isDateOnly(d) ? 'UTC' : 'America/New_York'
  const f = new Intl.DateTimeFormat('en-GB', {
    timeZone: zone,
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  const p = {}
  for (const part of f.formatToParts(d)) if (part.type !== 'literal') p[part.type] = part.value
  return { day: `${p.day}.${p.month}`, clock: `${p.hour}:${p.minute}`, dateOnly: isDateOnly(d) }
}

// Время дня, как его показывает сам пикер в сегментах «Начало»/«Срок».
const shown = (ep) =>
  `${String(ep.day.day).padStart(2, '0')}.${String(ep.day.month + 1).padStart(2, '0')} ${ep.time || '—'}`
const localClock = (ms) => {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
const localDay = (ms) => {
  const d = new Date(ms)
  return `${String(d.getDate()).padStart(2, '0')}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

// Выбрать день, затем время суток — ровно то, что делает человек в поповере.
async function pick(w, date, hh, mm) {
  model(w).pickDay(day(date.getFullYear(), date.getMonth(), date.getDate()))
  await nextTick()
  model(w).setTime(`${String(hh).padStart(2, '0')}:${String(mm).padStart(2, '0')}`)
  await nextTick()
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  // Неразмонтированный mount оставляет вотчеры на общем сторе темы и глушит
  // планировщик следующей спеки.
  wrapper?.unmount()
  wrapper = null
  state = null
  document.body.innerHTML = ''
})

describe('срок, попадающий на UTC-полночь', () => {
  it('летнее 20:00 остаётся 20:00 того же дня, а не «датой без времени»', async () => {
    const w = mountHost()
    await pick(w, new Date(2026, 6, 15), 20, 0) // ср 15 июля 2026, EDT
    const seen = asUserSees(lastApply(w).due_date)
    expect(seen.dateOnly).toBe(false)
    expect(seen.day).toBe('15.07')
    expect(seen.clock).toBe('20:00')
  })

  it('зимнее 19:00 остаётся 19:00 того же дня', async () => {
    const w = mountHost()
    await pick(w, new Date(2026, 11, 10), 19, 0) // чт 10 декабря 2026, EST
    const seen = asUserSees(lastApply(w).due_date)
    expect(seen.dateOnly).toBe(false)
    expect(seen.day).toBe('10.12')
    expect(seen.clock).toBe('19:00')
  })

  it('перечитанный пропс не уводит редактор на следующий день', async () => {
    const w = mountHost()
    await pick(w, new Date(2026, 6, 15), 20, 0)
    // Родитель положил ответ сервера обратно в пропсы.
    await nextTick()
    expect(localDay(state.due)).toBe('15.07')
    expect(localClock(state.due)).toBe('20:00')
    // И сам пикер продолжает показывать то же самое.
    expect(shown(model(w).dueEp)).toBe('15.07 20:00')
  })

  it('повторная отправка не сдвигает срок дальше', async () => {
    const w = mountHost()
    await pick(w, new Date(2026, 6, 15), 20, 0)
    const first = lastApply(w).due_date
    model(w).setFreq('daily') // любая другая правка шлёт тот же срок ещё раз
    await nextTick()
    expect(lastApply(w).due_date).toBe(first)
  })

  it('то же самое для даты начала', async () => {
    const w = mountHost()
    model(w).setTarget('start')
    await nextTick()
    await pick(w, new Date(2026, 6, 15), 20, 0)
    const seen = asUserSees(lastApply(w).start_date)
    expect(seen.dateOnly).toBe(false)
    expect(seen.day).toBe('15.07')
    expect(seen.clock).toBe('20:00')
  })

  // ── контроли: всё остальное остаётся ровно как было ──

  it('обычное время суток уходит секунда-в-секунду', async () => {
    const w = mountHost()
    await pick(w, new Date(2026, 6, 15), 21, 30)
    const iso = lastApply(w).due_date
    expect(iso).toBe(new Date(2026, 6, 15, 21, 30, 0, 0).toISOString())
    expect(asUserSees(iso)).toMatchObject({ day: '15.07', clock: '21:30', dateOnly: false })
  })

  // День без времени — дата без времени, UTC-полночь (#3118: прежний редактор
  // слал локальную полночь, у которой для зоны профиля ≠ браузера был чужой день).
  it('день без времени уходит датой без времени', async () => {
    const w = mountHost()
    model(w).pickDay(day(2026, 6, 15))
    await nextTick()
    const iso = lastApply(w).due_date
    expect(iso).toBe(new Date(Date.UTC(2026, 6, 15)).toISOString())
    expect(asUserSees(iso)).toMatchObject({ day: '15.07', dateOnly: true })
  })

  it('«Без времени» снимает время, оставляя день', async () => {
    const w = mountHost()
    await pick(w, new Date(2026, 6, 15), 20, 0)
    model(w).setTime(null)
    await nextTick()
    expect(asUserSees(lastApply(w).due_date)).toMatchObject({ day: '15.07', dateOnly: true })
  })

  it('легаси-срок в локальную полночь читается как день без времени', async () => {
    const w = mountHost({ due: new Date(2026, 6, 15, 0, 0, 0, 0).getTime() })
    await nextTick()
    expect(shown(model(w).dueEp)).toBe('15.07 —')
  })

  it('пришедшую извне дату без времени редактор по-прежнему читает как дату', async () => {
    // GitLab-срок: чистая UTC-полночь — 15 июля без времени в любой зоне.
    const w = mountHost({ due: Date.UTC(2026, 6, 15, 0, 0, 0) })
    await nextTick()
    expect(shown(model(w).dueEp)).toBe('15.07 —')
  })

  it('снятие срока остаётся снятием срока', async () => {
    const w = mountHost({ due: Date.UTC(2026, 6, 15, 16, 0, 0) })
    await nextTick()
    model(w).clearActive()
    await nextTick()
    expect(lastApply(w).due_date).toBeNull()
    expect(state.due).toBeNull()
  })
})
