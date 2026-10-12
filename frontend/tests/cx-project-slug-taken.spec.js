// #3013 / #3014: reading a taken project address and naming what a delete
// takes with it.
import { describe, expect, it, afterEach } from 'vitest'
import { i18n, setI18nLocale } from '@/i18n'
import { slugTaken } from '@/utils/slugTaken'
import { countsPhrase } from '@/utils/deleteCounts'
import { withProjectSlug } from '@/utils/projectRoute'

const t = (...a) => i18n.global.t(...a)
const conflict = (data) => Object.assign(new Error('x'), { status: 409, data })

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('slugTaken', () => {
  it('names the holder and passes the free variant on', () => {
    const r = slugTaken(
      conflict({
        error: 'slug already taken',
        suggestion: 'site-2',
        taken_by: { name: 'Сайт', group_name: 'Маркетинг' },
      }),
      t,
    )
    expect(r).toEqual({ error: 'Адрес занят: его использует проект «Сайт».', suggestion: 'site-2' })
  })

  it('speaks of the group in the create form, as the mockup does', () => {
    const r = slugTaken(
      conflict({ suggestion: 'tessera-web-2', taken_by: { name: 'Web', group_name: 'Маркетинг' } }),
      t,
      { inGroup: true },
    )
    expect(r.error).toBe('Адрес занят: так уже называется проект в группе «Маркетинг».')
    // Without a group the create form names the project instead.
    const r2 = slugTaken(conflict({ suggestion: 'x-2', taken_by: { name: 'Web' } }), t, {
      inGroup: true,
    })
    expect(r2.error).toBe('Адрес занят: его использует проект «Web».')
  })

  it('says only «taken» when the holder is not the caller’s to know', () => {
    const r = slugTaken(conflict({ error: 'slug already taken', suggestion: 'site-3' }), t)
    expect(r).toEqual({ error: 'Адрес занят.', suggestion: 'site-3' })
  })

  it('copes with a body that is not an object', () => {
    expect(slugTaken(conflict('slug already taken'), t)).toEqual({
      error: 'Адрес занят.',
      suggestion: '',
    })
  })

  it('leaves anything but a 409 to the caller', () => {
    expect(slugTaken(Object.assign(new Error('x'), { status: 400 }), t)).toBeNull()
    expect(slugTaken(null, t)).toBeNull()
  })
})

describe('countsPhrase', () => {
  it('lists the parts with Russian plurals and «и» before the last', () => {
    expect(countsPhrase(t, { projects: 4, boards: 23, tasks: 612 })).toBe(
      '4 проекта, 23 доски и 612 задач',
    )
    expect(countsPhrase(t, { boards: 3, tasks: 148 })).toBe('3 доски и 148 задач')
    expect(countsPhrase(t, { boards: 1, tasks: 21 })).toBe('1 доска и 21 задача')
    expect(countsPhrase(t, { projects: 5, boards: 11, tasks: 2 })).toBe(
      '5 проектов, 11 досок и 2 задачи',
    )
  })

  it('leaves zero parts out and returns nothing for an empty container', () => {
    expect(countsPhrase(t, { boards: 2, tasks: 0 })).toBe('2 доски')
    expect(countsPhrase(t, { projects: 0, boards: 0, tasks: 0 })).toBe('')
    expect(countsPhrase(t, null)).toBe('')
  })

  it('speaks English too', async () => {
    await setI18nLocale('en')
    expect(countsPhrase(t, { projects: 1, boards: 2, tasks: 1 })).toBe(
      '1 project, 2 boards and 1 task',
    )
  })
})

// #3012: the open screen under the project's current address. A `path` next to
// `params` makes vue-router drop the params — the old spelling went nowhere.
describe('withProjectSlug', () => {
  it('swaps the project segment and keeps the rest', () => {
    const route = {
      path: '/project/old/board/doska',
      params: { projectSlug: 'old', boardSlug: 'doska' },
      query: { task: '7' },
      hash: '#c',
    }
    expect(withProjectSlug(route, 'new')).toEqual({
      path: '/project/new/board/doska',
      query: { task: '7' },
      hash: '#c',
    })
    const stage = { path: '/project/a/stage/s1', query: {}, hash: '' }
    expect(withProjectSlug(stage, 'b').path).toBe('/project/b/stage/s1')
  })
})
