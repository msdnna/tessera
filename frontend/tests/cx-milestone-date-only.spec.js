// Дата этапа — date-only: utils/milestones.js читает её через formatDate, а тот
// (format.js isDateOnly/zoneOf) уходит в UTC только для чистой UTC-полуночи.
// Этапы из GitLab такими и приходят (client.go parseDate «2006-01-02»), а родные
// сохранялись из пикера как ЛОКАЛЬНАЯ полночь в UTC — то есть моментом.
//
// Отсюда два разных вреда:
//   1) один и тот же этап показывает соседний день коллеге в другой зоне;
//   2) push в GitLab печатает дату через dateStr → t.UTC().Format («2006-01-02»,
//      backend/handlers/gitlab_conflict.go), так что этап, назначенный в МСК на
//      15.07, приезжает в GitLab как 2026-07-14, а следующий pull
//      (ensureGitlabMilestone → UpdateMilestone) кладёт этот сдвинутый день
//      обратно поверх родной строки — уже для всех.
//
// Зона прогона зафиксирована: без этого спека была бы зелёной на UTC-раннере,
// где локальная полночь и есть UTC-полночь. Восстанавливаем в afterAll — пул
// vitest переиспользует процесс между файлами.
const TZ_BEFORE = process.env.TZ
process.env.TZ = 'Asia/Tokyo' // +09:00, локальная полночь = 15:00 UTC предыдущего дня

import { describe, it, expect, vi, beforeEach, afterAll } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { toDateOnlyISO, pickerValue } from '@/utils/milestones'

afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const createMilestone = vi.fn(() => Promise.resolve({ data: { id: 'm-new' } }))
const updateMilestone = vi.fn(() => Promise.resolve({ data: { id: 'm-1' } }))
const listMilestones = vi.fn(() => Promise.resolve({ data: [] }))

vi.mock('@/api', () => ({
  projects: {
    milestones: (...a) => listMilestones(...a),
    createMilestone: (...a) => createMilestone(...a),
  },
  milestones: {
    update: (...a) => updateMilestone(...a),
    remove: vi.fn(),
    pushToGitlab: vi.fn(),
  },
  gitlab: { listIntegrations: vi.fn(() => Promise.resolve({ data: { integrations: [] } })) },
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

// Локальная полночь 15 июля 2026 — ровно то, что кладёт в v-model n-date-picker
// с type="date" (он строит момент из календарных полей браузера).
const PICKED_15_JUL = new Date(2026, 6, 15, 0, 0, 0, 0).getTime()

describe('дата этапа хранится как date-only', () => {
  it('локальная полночь из пикера превращается в UTC-полночь того же дня', () => {
    const iso = toDateOnlyISO(PICKED_15_JUL)
    expect(iso).toBe('2026-07-15T00:00:00.000Z')
    const d = new Date(iso)
    expect(d.getUTCHours()).toBe(0)
    expect(d.getUTCMinutes()).toBe(0)
  })

  it('пустое значение остаётся пустым', () => {
    expect(toDateOnlyISO(null)).toBeNull()
    expect(toDateOnlyISO(undefined)).toBeNull()
    expect(toDateOnlyISO('')).toBeNull()
  })

  it('date-only значение возвращается в пикер своим календарным днём', () => {
    // 2026-07-15T00:00:00Z в Asia/Tokyo — это 09:00 15 июля, но полагаться на это
    // нельзя: западнее UTC тот же момент попадает на 14-е. Читаем день из UTC.
    expect(pickerValue('2026-07-15T00:00:00Z')).toBe(new Date(2026, 6, 15).getTime())
  })

  it('легаси-значение (не полночь) читается как момент, чтобы пикер не разошёлся с подписью', () => {
    // Так выглядят этапы, сохранённые до этой правки: локальная полночь МСК.
    const legacy = '2026-07-14T21:00:00Z'
    expect(pickerValue(legacy)).toBe(Date.parse(legacy))
  })
})

describe('MilestoneManager отправляет date-only', () => {
  let MilestoneManager
  beforeEach(async () => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    MilestoneManager = (await import('@/components/MilestoneManager.vue')).default
  })

  const mountManager = () =>
    mount(MilestoneManager, {
      props: { show: true, projectId: 'p-1', projectName: 'Tessera', wsId: 'w-1' },
      global: { stubs },
    })

  it('создание: срок уходит на сервер UTC-полночью выбранного дня', async () => {
    const w = mountManager()
    await w.vm.$nextTick()
    w.vm.newTitle = 'Sprint 1'
    w.vm.newDue = PICKED_15_JUL
    await w.vm.create()

    expect(createMilestone).toHaveBeenCalledTimes(1)
    const [, body] = createMilestone.mock.calls[0]
    expect(body.due_date).toBe('2026-07-15T00:00:00.000Z')
  })

  it('правка: срок из пикера тоже уходит UTC-полночью', async () => {
    const w = mountManager()
    await w.vm.$nextTick()
    w.vm.editBuf = {
      title: 'Sprint 1',
      start_date: null,
      due_date: PICKED_15_JUL,
      state: 'active',
    }
    await w.vm.saveEdit({ id: 'm-1', description: '' })

    expect(updateMilestone).toHaveBeenCalledTimes(1)
    const [, body] = updateMilestone.mock.calls[0]
    expect(body.due_date).toBe('2026-07-15T00:00:00.000Z')
  })

  it('открытие на правку: date-only с сервера ложится в пикер тем же днём', async () => {
    const w = mountManager()
    await w.vm.$nextTick()
    w.vm.startEdit({
      id: 'm-1',
      title: 'Sprint 1',
      due_date: '2026-07-15T00:00:00Z',
      state: 'active',
    })
    expect(w.vm.editBuf.due_date).toBe(new Date(2026, 6, 15).getTime())
  })
})
