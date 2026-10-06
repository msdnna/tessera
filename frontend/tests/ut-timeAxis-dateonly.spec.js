// Ось «Таймлайн»/«Гант» и date-only значения.
//
// Ось раскладывается по ЛОКАЛЬНЫМ календарным дням (timeAxis.startOfDay), и для
// значения с временем это правильно — час дня и есть смысл такого значения.
// Но часть дат в Tessera — date-only: они означают КАЛЕНДАРНУЮ ДАТУ и хранятся
// чистой UTC-полуночью. Этапы пишутся так намеренно (utils/milestones
// toDateOnlyISO, #2798), а задачи и этапы из GitLab приезжают такими сами
// (client.go parseDate «2006-01-02»).
//
// Восточнее UTC это незаметно: UTC-полночь — это утро того же местного дня.
// Западнее — вечер ПРЕДЫДУЩЕГО, поэтому startOfDay отдаёт вчерашний день, и
// пунктир этапа с полосой задачи встают на сутки раньше подписи рядом: сама
// подпись идёт через formatDate, а тот (format.js zoneOf/isDateOnly) читает
// date-only в UTC и называет верное число.
//
// Зона прогона зафиксирована: на UTC-раннере и в МСК спека была бы зелёной —
// там сдвига нет вовсе. Восстанавливаем в afterAll: пул vitest переиспользует
// процесс между файлами.
const TZ_BEFORE = process.env.TZ
process.env.TZ = 'America/New_York' // −04:00 летом: UTC-полночь = 20:00 прошлого дня

import { describe, it, expect, afterAll, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { ref, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { DAY_MS, dayStartOf, startOfDay, spanOf, barSpan, anchorMs } from '@/utils/timeAxis'
import { useChartTimeline } from '@/composables/useChartTimeline'

afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

// Местная полночь 15.07.2026 — та ячейка оси, под которой стоит «15 июля».
const LOCAL_JUL15 = new Date(2026, 6, 15).getTime()
const LOCAL_JUL14 = new Date(2026, 6, 14).getTime()
// Как date-only дата 15.07 лежит в базе и приходит клиенту.
const ALL_DAY_JUL15 = '2026-07-15T00:00:00Z'

describe('dayStartOf', () => {
  it('кладёт date-only значение на его же календарный день, а не на предыдущий', () => {
    expect(startOfDay(Date.parse(ALL_DAY_JUL15))).toBe(LOCAL_JUL14) // прежнее поведение
    expect(dayStartOf(Date.parse(ALL_DAY_JUL15))).toBe(LOCAL_JUL15)
  })

  it('значение со временем читает по-местному — час дня в нём и есть смысл', () => {
    // 15.07 09:30 по Нью-Йорку: не UTC-полночь, значит это момент, а не дата.
    const timed = new Date(2026, 6, 15, 9, 30).getTime()
    expect(dayStartOf(timed)).toBe(LOCAL_JUL15)
    expect(dayStartOf(timed)).toBe(startOfDay(timed))
  })
})

describe('полоса задачи с date-only сроком', () => {
  it('spanOf не уводит концы на сутки назад', () => {
    const s = spanOf({ start_date: ALL_DAY_JUL15, due_date: ALL_DAY_JUL15 })
    expect(s.a).toBe(LOCAL_JUL15)
    expect(s.b).toBe(LOCAL_JUL15)
  })

  it('barSpan рисует её ровно на своей ячейке шириной в день', () => {
    const rangeStart = new Date(2026, 6, 10).getTime()
    const dayW = 32
    const b = barSpan({
      start: Date.parse(ALL_DAY_JUL15),
      due: Date.parse(ALL_DAY_JUL15),
      tier: 'days',
      rangeStart,
      dayW,
    })
    expect(b.left).toBe(5 * dayW) // 15.07 — шестая ячейка от 10.07
    expect(b.width).toBe(dayW)
  })

  it('anchorMs (привязка «призрака» оценки) идёт за полосой', () => {
    expect(anchorMs(Date.parse(ALL_DAY_JUL15), 'days')).toBe(LOCAL_JUL15)
    expect(anchorMs(Date.parse(ALL_DAY_JUL15), 'hours')).toBe(LOCAL_JUL15)
  })
})

function host(setup) {
  let api
  mount(
    defineComponent({
      setup() {
        api = setup()
        return () => h('div')
      },
    }),
  )
  return api
}

describe('пунктир этапа на оси', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('стоит на дне этапа, а не на предыдущем', () => {
    const { milestoneMarkers, range } = host(() =>
      useChartTimeline({
        scheduled: computed(() => [
          { id: 'a', title: 'a', start_date: ALL_DAY_JUL15, due_date: ALL_DAY_JUL15 },
        ]),
        milestones: ref([{ id: 'm1', title: 'v1', due_date: ALL_DAY_JUL15 }]),
        estCfg: ref({ unit: 'hours' }),
        leftW: ref(224),
      }),
    )
    const di = Math.round((LOCAL_JUL15 - range.value.start) / DAY_MS)
    expect(milestoneMarkers.value).toHaveLength(1)
    expect(milestoneMarkers.value[0].di).toBe(di)
  })
})
