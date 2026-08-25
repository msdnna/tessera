import { describe, it, expect, beforeEach } from 'vitest'
import { i18n, setI18nLocale } from '@/i18n'
import { workspaceName, columnName, taskColumnName } from '@/utils/defaultNames'
import { columnStatusName } from '@/utils/columnStatus'

// Доработки 1–2 of #2800: the seeded personal workspace and the four seeded board
// columns arrived from the server as finished Russian strings. They now carry a
// name_key, and these specs pin the two halves of the deal — the key wins, and the
// server string stays as the fallback — plus the line that matters most: a name the
// user chose has no key and is never redrawn.

const ctx = () => ({ t: i18n.global.t, te: i18n.global.te })

const personal = (over = {}) => ({ name_key: 'personal', name: 'Личное пространство', ...over })

describe('workspaceName', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('renders the seeded personal workspace in the active language', async () => {
    expect(workspaceName(personal(), ctx())).toBe('Личное пространство')
    await setI18nLocale('en')
    expect(workspaceName(personal(), ctx())).toBe('Personal space')
  })

  it('shows a user-named workspace verbatim in every language', async () => {
    // No key — the server drops it on rename, which is what makes this a chosen
    // name rather than a default one.
    const mine = { name_key: null, name: 'Мой хлам' }
    expect(workspaceName(mine, ctx())).toBe('Мой хлам')
    await setI18nLocale('en')
    expect(workspaceName(mine, ctx())).toBe('Мой хлам')
  })

  it('falls back to the server string for an unknown key, and is empty without one', () => {
    expect(workspaceName(personal({ name_key: 'shared_holodeck' }), ctx())).toBe(
      'Личное пространство',
    )
    expect(workspaceName(null, ctx())).toBe('')
  })
})

describe('columnName', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('renders the four seeded columns in the active language', async () => {
    const seeded = [
      { name_key: 'todo', name: 'К работе' },
      { name_key: 'in_progress', name: 'В процессе' },
      { name_key: 'review', name: 'На рассмотрении' },
      { name_key: 'done', name: 'Готово' },
    ]
    expect(seeded.map((c) => columnName(c, ctx()))).toEqual([
      'К работе',
      'В процессе',
      'На рассмотрении',
      'Готово',
    ])
    await setI18nLocale('en')
    expect(seeded.map((c) => columnName(c, ctx()))).toEqual([
      'To do',
      'In progress',
      'In review',
      'Done',
    ])
  })

  it('shows a column the user added or renamed verbatim', async () => {
    const own = { name_key: null, name: 'Бэклог' }
    expect(columnName(own, ctx())).toBe('Бэклог')
    await setI18nLocale('en')
    expect(columnName(own, ctx())).toBe('Бэклог')
  })

  it('reads the same column flattened into a task row', async () => {
    const task = { column_name_key: 'review', column_name: 'На рассмотрении' }
    expect(taskColumnName(task, ctx())).toBe('На рассмотрении')
    await setI18nLocale('en')
    expect(taskColumnName(task, ctx())).toBe('In review')
    expect(taskColumnName({ column_name: 'Бэклог' }, ctx())).toBe('Бэклог')
  })
})

describe('columnStatusName', () => {
  // The status glyph used to be picked by matching the column's name against a
  // word list. A seeded column now says which one it is outright, so the caption's
  // language stops mattering; a keyless column still goes through the words.
  it('prefers the key over the name', () => {
    expect(columnStatusName({ nameKey: 'review', name: 'In review' })).toBe('status-review')
    expect(columnStatusName({ nameKey: 'in_progress', name: 'На рассмотрении' })).toBe(
      'status-progress',
    )
  })

  it('still matches by name when the column has no key', () => {
    expect(columnStatusName({ name: 'Ревью' })).toBe('status-review')
    expect(columnStatusName({ name: 'Бэклог' })).toBe('status-progress')
    expect(columnStatusName({ isDone: true, name: 'Бэклог' })).toBe('status-done')
  })
})
