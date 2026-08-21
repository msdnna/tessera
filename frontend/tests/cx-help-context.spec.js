import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { helpSlugForPath } from '@/utils/helpContext'
import { useHelpStore } from '@/stores/help'

// Contextual help (#2794): the route→article map and the drawer half of the help
// store. The store reads the real docs/help index and the real Markdown files —
// the point of these assertions is the *plumbing* (which slug lands where, whose
// state gets written), so they name only the slugs, never article prose.

describe('helpSlugForPath', () => {
  it('доски — по обоим маршрутам доски, включая legacy /board/<id>', () => {
    expect(helpSlugForPath('/project/tessera/board/main')).toBe('boards-and-tasks')
    expect(helpSlugForPath('/board/2b0f8c1e')).toBe('boards-and-tasks')
  })

  it('разделы приложения ведут на свои статьи', () => {
    expect(helpSlugForPath('/documents')).toBe('documents')
    expect(helpSlugForPath('/documents/plan-2026')).toBe('documents')
    expect(helpSlugForPath('/notes')).toBe('notes')
    expect(helpSlugForPath('/reminders')).toBe('reminders')
    expect(helpSlugForPath('/milestones')).toBe('milestones')
    expect(helpSlugForPath('/settings')).toBe('faq')
  })

  it('на самой справке подсказки нет', () => {
    expect(helpSlugForPath('/help')).toBeNull()
    expect(helpSlugForPath('/help/first-steps')).toBeNull()
  })

  it('незнакомый экран получает первую статью, а не пустоту', () => {
    expect(helpSlugForPath('/')).toBe('first-steps')
    expect(helpSlugForPath('/some/new/screen')).toBe('first-steps')
    expect(helpSlugForPath(undefined)).toBe('first-steps')
  })

  it('не путает префикс с отдельным словом', () => {
    // /notesomething is not the notes screen; /boardgames is not a board.
    expect(helpSlugForPath('/notesomething')).toBe('first-steps')
    expect(helpSlugForPath('/boardgames')).toBe('first-steps')
  })
})

describe('help store — контекстная панель', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('панель и страница читают разные статьи одновременно', async () => {
    const help = useHelpStore()
    await help.open('first-steps')
    const pageBody = help.body
    expect(pageBody).not.toBe('')

    await help.openDrawer('documents')

    expect(help.drawerShown).toBe(true)
    expect(help.drawerSlug).toBe('documents')
    expect(help.drawerBody).not.toBe('')
    // The screen the reader left open is untouched — that is the whole promise
    // of contextual help.
    expect(help.current).toBe('first-steps')
    expect(help.body).toBe(pageBody)
  })

  it('заголовок панели берётся из индекса', async () => {
    const help = useHelpStore()
    await help.openDrawer('milestones')
    expect(help.drawerMeta?.slug).toBe('milestones')
    expect(help.drawerMeta?.title).toBe(help.bySlug('milestones').title)
  })

  it('без slug открывается первая статья', async () => {
    const help = useHelpStore()
    await help.openDrawer('')
    expect(help.drawerSlug).toBe(help.defaultSlug)
  })

  it('несуществующая статья — ошибка в панели, страница не портится', async () => {
    const help = useHelpStore()
    await help.open('faq')
    await help.openDrawer('нет-такой-статьи')
    expect(help.drawerError).toBeTruthy()
    expect(help.drawerBody).toBe('')
    expect(help.error).toBe('')
    expect(help.current).toBe('faq')
  })

  it('closeDrawer прячет панель, но текст остаётся в кэше для следующего показа', async () => {
    const help = useHelpStore()
    await help.openDrawer('notes')
    const body = help.drawerBody
    help.closeDrawer()
    expect(help.drawerShown).toBe(false)
    // Reopening the same article is synchronous off the cache: no await, the
    // body is already there.
    help.openDrawer('notes')
    expect(help.drawerShown).toBe(true)
    expect(help.drawerBody).toBe(body)
  })

  it('find ищет по индексу, не трогая строку поиска на странице справки', () => {
    const help = useHelpStore()
    help.query = 'заметки'
    const hits = help.find('доски', 3)
    expect(hits.length).toBeGreaterThan(0)
    expect(hits.length).toBeLessThanOrEqual(3)
    expect(hits[0]).toHaveProperty('slug')
    expect(help.query).toBe('заметки')
    expect(help.find('   ')).toEqual([])
  })
})
