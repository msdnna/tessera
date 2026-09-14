import { describe, it, expect } from 'vitest'
import {
  filterBoardTasks,
  isTaskOverdue,
  isDueWithinDays,
  matchesDue,
  matchesHomeDue,
  matchesTask,
  hasSubtaskFacets,
  hasAnyFacets,
} from '@/utils/taskFilter'
import { createFormatters, DEFAULT_PREFS } from '@/utils/format'

const NOW = new Date('2026-08-03T12:00:00')

const EMPTY = {
  q: '',
  priorities: [],
  assignees: [],
  tags: [],
  statuses: [],
  milestones: [],
  due: '',
}
const f = (over) => ({ ...EMPTY, ...over })

// One parent in column c1 with two children; only the first is assigned to u1.
const parent = {
  id: 'p1',
  title: 'Родитель',
  number: 10,
  column_id: 'c1',
  priority: 0,
  assignee_ids: [],
}
const subMine = {
  id: 's1',
  parent_id: 'p1',
  title: 'Моя подзадача',
  number: 11,
  column_id: 'c1',
  priority: 3,
  assignee_ids: ['u1'],
  tag_ids: ['tagA'],
  // Наивно-локальная дата (без смещения), как и NOW выше: matchesDue сравнивает
  // локальные календарные дни, поэтому фикстура должна быть в той же TZ-базе, что
  // и `now`. С фиксированным `+03:00` тест падал в UTC-раннере CI (день съезжал).
  due_date: '2026-08-03T00:00:00',
}
const subOther = {
  id: 's2',
  parent_id: 'p1',
  title: 'Чужая подзадача',
  number: 12,
  column_id: 'c2',
  priority: 0,
  assignee_ids: ['u2'],
}
const board = { tasks: [parent], subtasksByParent: { p1: [subMine, subOther] }, now: NOW }

describe('filterBoardTasks — подзадача поднимает родителя', () => {
  it('фильтр по исполнителю показывает родителя и только совпавшую подзадачу', () => {
    const r = filterBoardTasks({ ...board, filters: f({ assignees: ['u1'] }) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
    expect(r.narrowedParents.has('p1')).toBe(true)
  })

  it('поиск по заголовку подзадачи поднимает родителя', () => {
    const r = filterBoardTasks({ ...board, filters: f({ q: 'моя подз' }) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
  })

  it('поиск по #номеру подзадачи поднимает родителя', () => {
    const r = filterBoardTasks({ ...board, filters: f({ q: '#11' }) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
  })

  it('теги / приоритет / срок подзадачи тоже поднимают родителя', () => {
    for (const flt of [{ tags: ['tagA'] }, { priorities: [3] }, { due: 'today' }]) {
      const r = filterBoardTasks({ ...board, filters: f(flt) })
      expect(
        r.tasks.map((t) => t.id),
        JSON.stringify(flt),
      ).toEqual(['p1'])
      expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
    }
  })

  it('gl:<login> подзадачи поднимает родителя', () => {
    const gl = { ...subMine, assignee_ids: [], gitlab_assignee_logins: ['msdnna'] }
    const r = filterBoardTasks({
      tasks: [parent],
      subtasksByParent: { p1: [gl, subOther] },
      filters: f({ assignees: ['gl:msdnna'] }),
      now: NOW,
    })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
  })

  it('ни родитель, ни дети не совпали — родителя нет', () => {
    const r = filterBoardTasks({ ...board, filters: f({ assignees: ['u9'] }) })
    expect(r.tasks).toEqual([])
  })
})

describe('filterBoardTasks — колонка (statuses) не пробрасывается', () => {
  it('подзадача из другой колонки НЕ затягивает родителя в этот фильтр', () => {
    // s2 живёт в c2, родитель — в c1: фильтр по колонке c2 не должен показать родителя.
    const r = filterBoardTasks({ ...board, filters: f({ statuses: ['c2'] }) })
    expect(r.tasks).toEqual([])
  })

  it('колонка родителя учитывается и при подъёме подзадачей', () => {
    const ok = filterBoardTasks({ ...board, filters: f({ statuses: ['c1'], assignees: ['u1'] }) })
    expect(ok.tasks.map((t) => t.id)).toEqual(['p1'])
    const no = filterBoardTasks({ ...board, filters: f({ statuses: ['c2'], assignees: ['u1'] }) })
    expect(no.tasks).toEqual([])
  })
})

describe('filterBoardTasks — родитель совпал сам', () => {
  it('все подзадачи остаются, родитель не помечен усечённым', () => {
    const r = filterBoardTasks({ ...board, filters: f({ q: 'родитель' }) })
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1', 's2'])
    expect(r.narrowedParents.size).toBe(0)
  })

  it('без фильтров доска не меняется', () => {
    const r = filterBoardTasks({ ...board, filters: f({}) })
    expect(r.tasks).toEqual([parent])
    expect(r.subtasksByParent.p1).toHaveLength(2)
    expect(r.narrowedParents.size).toBe(0)
  })

  it('совпали все дети — усечения нет', () => {
    const r = filterBoardTasks({
      tasks: [parent],
      subtasksByParent: { p1: [subMine, { ...subOther, assignee_ids: ['u1'] }] },
      filters: f({ assignees: ['u1'] }),
      now: NOW,
    })
    expect(r.subtasksByParent.p1).toHaveLength(2)
    expect(r.narrowedParents.size).toBe(0)
  })
})

describe('matchesDue', () => {
  const t = (d) => ({ due_date: d })
  it('none / has', () => {
    expect(matchesDue(t(null), 'none', NOW)).toBe(true)
    expect(matchesDue(t('2026-08-03'), 'none', NOW)).toBe(false)
    expect(matchesDue(t('2026-08-03'), 'has', NOW)).toBe(true)
    expect(matchesDue(t(null), 'has', NOW)).toBe(false)
  })
  it('today / week / overdue', () => {
    expect(matchesDue(t('2026-08-03T09:00:00'), 'today', NOW)).toBe(true)
    expect(matchesDue(t('2026-08-06T09:00:00'), 'week', NOW)).toBe(true)
    expect(matchesDue(t('2026-08-20T09:00:00'), 'week', NOW)).toBe(false)
    expect(matchesDue(t('2026-08-01T09:00:00'), 'overdue', NOW)).toBe(true)
    expect(matchesDue({ due_date: '2026-08-01', completed_at: '2026-08-02' }, 'overdue', NOW)).toBe(
      false,
    )
  })

  // The week window is a seven-day half-open range [today, today+7): the same one
  // WorkspaceTaskSummary counts in SQL (week_end exclusive) and Android's home
  // filters on. A day-8 task inside it is a card the tile over it doesn't count.
  it('неделя — семь дней, ровно через неделю уже вне окна', () => {
    expect(matchesDue(t('2026-08-09T23:00:00'), 'week', NOW)).toBe(true) // today+6
    expect(matchesDue(t('2026-08-10T00:00:00'), 'week', NOW)).toBe(false) // today+7
  })
})

// The Home screen's own predicate: same windows as the summary tiles above it,
// minus completed tasks (a closed task is not work that is due).
describe('matchesHomeDue', () => {
  const t = (d, done) => ({ due_date: d, completed_at: done || null })
  it('окно недели совпадает с плиткой: today+6 внутри, today+7 снаружи', () => {
    expect(matchesHomeDue(t('2026-08-09T23:00:00'), 'week', NOW)).toBe(true)
    expect(matchesHomeDue(t('2026-08-10T00:00:00'), 'week', NOW)).toBe(false)
  })
  it('закрытые задачи не попадают ни в один срок', () => {
    expect(matchesHomeDue(t('2026-08-03T09:00:00', '2026-08-03T10:00:00'), 'today', NOW)).toBe(
      false,
    )
    expect(matchesHomeDue(t('2026-08-05T09:00:00', '2026-08-03T10:00:00'), 'week', NOW)).toBe(false)
    expect(matchesHomeDue(t('2026-08-01T09:00:00', '2026-08-02T10:00:00'), 'overdue', NOW)).toBe(
      false,
    )
  })
  it('открытые попадают: сегодня, на неделе, просрочена', () => {
    expect(matchesHomeDue(t('2026-08-03T09:00:00'), 'today', NOW)).toBe(true)
    expect(matchesHomeDue(t('2026-08-05T09:00:00'), 'week', NOW)).toBe(true)
    expect(matchesHomeDue(t('2026-08-01T09:00:00'), 'overdue', NOW)).toBe(true)
    expect(matchesHomeDue(t(null), 'today', NOW)).toBe(false)
  })
})

// A due date has to be bucketed in the same calendar it is PRINTED in, because the
// same pill carries both: TaskCardPills renders the text with formatDue and paints
// it red with isTaskOverdue. Two rules formatDue follows and the filter must too:
// the user's `timezone` preference (not the browser's), and "a pure UTC midnight is
// a date-only value, read in UTC" (a GitLab issue/milestone due).
//
// Every case below is asserted in a pair of zones on opposite sides of any runner —
// Kamchatka (+12) and Honolulu (-10) — so the expectations cannot be satisfied by a
// single zone-blind answer no matter what TZ the test process runs in.
describe('срок читается в календаре пользователя, а не браузера', () => {
  const fmt = (timezone) => createFormatters({ ...DEFAULT_PREFS, timezone })
  const east = fmt('Asia/Kamchatka') // UTC+12
  const west = fmt('Pacific/Honolulu') // UTC-10
  const t = (d, done) => ({ due_date: d, completed_at: done || null })

  // 10:00Z is 22:00 on the 13th in Kamchatka and 00:00 on the 13th in Honolulu;
  // 14:00Z has already rolled over to the 14th in Kamchatka but not in Honolulu. So
  // the very same task is late for one user and due today for the other.
  it('профильная зона решает, просрочена ли задача', () => {
    const now = new Date('2026-09-13T14:00:00Z')
    const task = t('2026-09-13T10:00:00Z')
    expect(isTaskOverdue(task, now, east)).toBe(true)
    expect(isTaskOverdue(task, now, west)).toBe(false)
    expect(matchesDue(task, 'overdue', now, east)).toBe(true)
    expect(matchesDue(task, 'overdue', now, west)).toBe(false)
    expect(matchesDue(task, 'today', now, east)).toBe(false)
    expect(matchesDue(task, 'today', now, west)).toBe(true)
  })

  // The redness and the caption come from the same module pair, so they must agree:
  // a pill that reads «вчера» is red, one that reads «сегодня» is not.
  it('краснота согласована с подписью formatDue', () => {
    const now = new Date('2026-09-13T14:00:00Z')
    const task = t('2026-09-13T10:00:00Z')
    expect(east.formatRelative(task.due_date, { now })).toBe('вчера')
    expect(west.formatRelative(task.due_date, { now })).toBe('сегодня')
    expect(isTaskOverdue(task, now, east)).toBe(true)
    expect(isTaskOverdue(task, now, west)).toBe(false)
  })

  // A pure UTC midnight carries no time of day, so its calendar day is the UTC one
  // in every zone — the rule formatDue already applies via isDateOnly. Only "today"
  // moves with the user's zone.
  it('date-only срок (issue из GitLab) читается в UTC', () => {
    const dateOnly = t('2026-09-14T00:00:00Z')
    // Kamchatka has already entered the 14th; the due day is the 14th too.
    const nowEast = new Date('2026-09-13T14:00:00Z')
    expect(matchesDue(dateOnly, 'today', nowEast, east)).toBe(true)
    expect(matchesDue(dateOnly, 'overdue', nowEast, east)).toBe(false)
    // Honolulu is still on the 13th, so the same date-only due is tomorrow.
    const nowWest = new Date('2026-09-14T05:00:00Z')
    expect(matchesDue(dateOnly, 'today', nowWest, west)).toBe(false)
    expect(matchesDue(dateOnly, 'week', nowWest, west)).toBe(true)
  })

  it('домашний предикат берёт ту же зону', () => {
    const now = new Date('2026-09-13T14:00:00Z')
    const task = t('2026-09-13T10:00:00Z')
    expect(matchesHomeDue(task, 'overdue', now, east)).toBe(true)
    expect(matchesHomeDue(task, 'overdue', now, west)).toBe(false)
    expect(matchesHomeDue(task, 'today', now, west)).toBe(true)
  })

  it('фасет «Срок» на доске фильтрует в той же зоне', () => {
    const now = new Date('2026-09-13T14:00:00Z')
    const card = {
      id: 'z1',
      title: 'Задача со сроком',
      number: 30,
      column_id: 'c1',
      priority: 0,
      due_date: '2026-09-13T10:00:00Z',
    }
    const args = { tasks: [card], subtasksByParent: {}, filters: f({ due: 'overdue' }), now }
    expect(filterBoardTasks({ ...args, formatters: east }).tasks).toHaveLength(1)
    expect(filterBoardTasks({ ...args, formatters: west }).tasks).toHaveLength(0)
  })

  // Negative controls: the zone cannot invent a due date, revive a closed task, or
  // change an answer that does not depend on a calendar day.
  it('зона не меняет ответы, не зависящие от календарного дня', () => {
    const now = new Date('2026-09-13T14:00:00Z')
    for (const z of [east, west]) {
      expect(matchesDue(t(null), 'none', now, z)).toBe(true)
      expect(matchesDue(t(null), 'has', now, z)).toBe(false)
      expect(matchesDue(t('2026-09-13T10:00:00Z'), 'has', now, z)).toBe(true)
      expect(isTaskOverdue(t(null), now, z)).toBe(false)
      expect(isTaskOverdue(t('2026-01-01T10:00:00Z', '2026-01-02T10:00:00Z'), now, z)).toBe(false)
      expect(
        matchesHomeDue(t('2026-01-01T10:00:00Z', '2026-01-02T10:00:00Z'), 'overdue', now, z),
      ).toBe(false)
    }
  })
})

describe('matchesTask / hasSubtaskFacets', () => {
  it('facets ограничивают набор проверяемых фильтров', () => {
    const flt = f({ statuses: ['c9'], assignees: ['u1'] })
    expect(matchesTask(subMine, flt, { now: NOW })).toBe(false) // колонка не та
    expect(matchesTask(subMine, flt, { facets: ['assignees'], now: NOW })).toBe(true)
  })
  it('hasAnyFacets учитывает и колонку, которую подзадача поднять не может', () => {
    expect(hasAnyFacets(f({}))).toBe(false)
    expect(hasAnyFacets(f({ statuses: ['c1'] }))).toBe(true)
    expect(hasAnyFacets(f({ q: '  ' }))).toBe(false)
    expect(hasAnyFacets(f({ tags: ['tagA'] }))).toBe(true)
  })
  it('пустой композер: быстрый путь отдаёт то же, что и общий (#2658)', () => {
    const r = filterBoardTasks({ ...board, filters: f({}) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1', 's2'])
    expect(r.narrowedParents.size).toBe(0)
    // Родителей без детей в карте быть не должно — иначе счётчик «N из M» на карточке
    // увидит пустой список там, где детей просто нет.
    const lone = { id: 'p2', title: 'Один', number: 20, column_id: 'c1', priority: 0 }
    const r2 = filterBoardTasks({ tasks: [parent, lone], subtasksByParent: {}, filters: f({}) })
    expect(r2.tasks.map((t) => t.id)).toEqual(['p1', 'p2'])
    expect(Object.keys(r2.subtasksByParent)).toEqual([])
    // И массив задач — свежий, а не та же ссылка, что пришла на вход.
    const input = [parent, lone]
    expect(filterBoardTasks({ tasks: input, subtasksByParent: {}, filters: f({}) }).tasks).not.toBe(
      input,
    )
  })
  it('без «подзадачных» фасетов проход по детям не нужен', () => {
    expect(hasSubtaskFacets(f({}))).toBe(false)
    expect(hasSubtaskFacets(f({ statuses: ['c1'] }))).toBe(false)
    expect(hasSubtaskFacets(f({ q: '  ' }))).toBe(false)
    expect(hasSubtaskFacets(f({ q: 'x' }))).toBe(true)
    expect(hasSubtaskFacets(f({ tags: ['tagA'] }))).toBe(true)
  })
})

// Ось «Срочно» матрицы Эйзенхауэра. До этого она считала окно миллисекундами
// (`due <= Date.now() + 7 * 86400000`), а не календарными днями, и потому спорила
// с фасетом «Срок: неделя» на той же доске: у задачи со сроком ровно через семь
// дней время суток обычно раньше текущего — для date-only срока (UTC-полночь)
// всегда, — и инстант проскакивал под границу.
describe('isDueWithinDays — срочность по календарным дням', () => {
  const zone = (timezone) => createFormatters({ ...DEFAULT_PREFS, timezone })
  const east = zone('Asia/Kamchatka') // UTC+12
  const west = zone('Pacific/Honolulu') // UTC-10
  const d = (due) => ({ due_date: due })

  // now = 14:00Z 13 сентября: 14-е 02:00 на Камчатке, 13-е 04:00 в Гонолулу.
  const NOW_Z = new Date('2026-09-13T14:00:00Z')

  // Ключевой случай: инстант укладывается в now + 7×86400000 (20-е 14:00Z), но
  // календарный день срока — уже восьмой (21-е на Камчатке, 20-е в Гонолулу),
  // то есть ровно та граница, за которую окно не заходит.
  it('срок на восьмой день не срочный, хотя инстант влезает в 7×86400000', () => {
    const task = d('2026-09-20T13:00:00Z')
    expect(new Date(task.due_date).getTime() <= NOW_Z.getTime() + 7 * 86400000).toBe(true)
    for (const z of [east, west]) {
      expect(isDueWithinDays(task, 7, NOW_Z, z)).toBe(false)
      // И фасет «неделя» рядом на доске отвечает так же — в этом весь смысл.
      expect(matchesDue(task, 'week', NOW_Z, z)).toBe(false)
    }
  })

  it('седьмой день окна срочный, и фасет «неделя» с ним согласен', () => {
    const task = d('2026-09-19T13:00:00Z')
    for (const z of [east, west]) {
      expect(isDueWithinDays(task, 7, NOW_Z, z)).toBe(true)
      expect(matchesDue(task, 'week', NOW_Z, z)).toBe(true)
    }
  })

  // Тот же date-only срок (UTC-полночь) попадает в окно у одного пользователя и
  // не попадает у другого — окно отсчитывается от ЕГО «сегодня».
  it('date-only срок меряется от «сегодня» в зоне пользователя', () => {
    const task = d('2026-09-20T00:00:00Z')
    expect(isDueWithinDays(task, 7, NOW_Z, east)).toBe(true) // «сегодня» 14-е → до 21-го
    expect(isDueWithinDays(task, 7, NOW_Z, west)).toBe(false) // «сегодня» 13-е → до 20-го
    expect(matchesDue(task, 'week', NOW_Z, east)).toBe(true)
    expect(matchesDue(task, 'week', NOW_Z, west)).toBe(false)
  })

  // Окно, перешагивающее перевод часов, тоже календарное: 26-е 23:30Z — это уже
  // 27-е 00:30 в Берлине (CET с 25 октября), восьмой день.
  it('перевод часов не сдвигает границу окна', () => {
    const berlin = zone('Europe/Berlin')
    const now = new Date('2026-10-20T08:00:00Z') // 10:00 по Берлину, CEST
    expect(isDueWithinDays(d('2026-10-26T23:30:00Z'), 7, now, berlin)).toBe(false)
    expect(isDueWithinDays(d('2026-10-26T22:30:00Z'), 7, now, berlin)).toBe(true)
  })

  it('просроченное срочно, а задача без срока — нет', () => {
    for (const z of [east, west]) {
      expect(isDueWithinDays(d('2026-09-01T10:00:00Z'), 7, NOW_Z, z)).toBe(true)
      expect(isDueWithinDays(d(null), 7, NOW_Z, z)).toBe(false)
      expect(isDueWithinDays(undefined, 7, NOW_Z, z)).toBe(false)
      // Ширину окна задаёт вызывающий: с однодневным окном тот же срок уже не срочен.
      expect(isDueWithinDays(d('2026-09-19T13:00:00Z'), 1, NOW_Z, z)).toBe(false)
    }
  })
})
