import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Platform scoping in the web store (#2795). The index is one file for both
// clients: it carries the desktop article, the mobile rewrite folded in as an
// `android` section, and the odd article that exists on one platform only. The
// site has to read exactly the desktop half of that.
//
// A fixture index rather than the real one: today every article is written for
// both clients, so asserting the filter against docs/help would pass with the
// filter deleted.
vi.mock('@/data/helpIndex.json', () => ({
  default: {
    articles: [
      {
        slug: 'first-steps',
        path: 'start/first-steps.md',
        title: 'Первые шаги',
        category: 'Начало',
        order: 10,
        updated: '2026-08-24',
        keywords: [],
        platforms: ['web', 'android'],
        headings: [],
        text: 'проект перетаскивается в группу мышью',
        android: {
          path: 'start/first-steps.android.md',
          updated: '2026-08-24',
          keywords: ['телефон'],
          headings: [],
          text: 'проект переносится долгим тапом',
        },
      },
      {
        slug: 'shortcuts',
        path: 'start/shortcuts.md',
        title: 'Горячие клавиши',
        category: 'Начало',
        order: 20,
        updated: '2026-08-24',
        keywords: [],
        platforms: ['web'],
        headings: [],
        text: 'сочетания клавиш',
      },
      {
        slug: 'push',
        path: 'start/push.md',
        title: 'Пуш-уведомления',
        category: 'Начало',
        order: 30,
        updated: '2026-08-24',
        keywords: [],
        platforms: ['android'],
        headings: [],
        text: 'уведомления приходят на телефон',
      },
    ],
  },
}))

const { useHelpStore } = await import('@/stores/help')

describe('Справка: разграничение платформ (#2795)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('статья только для приложения в вебе не показывается', () => {
    const help = useHelpStore()
    expect(help.articles.map((a) => a.slug)).toEqual(['first-steps', 'shortcuts'])
    expect(help.bySlug('push')).toBeNull()
  })

  it('её нет ни в навигации, ни в поиске', () => {
    const help = useHelpStore()
    expect(help.categories[0].articles.map((a) => a.slug)).toEqual(['first-steps', 'shortcuts'])
    // Would be worse than a missing article: a hit that opens nothing.
    expect(help.find('уведомления')).toEqual([])
  })

  it('веб читает десктопный текст, а не мобильный вариант', () => {
    const help = useHelpStore()
    // The variant's own words must not make an article findable on the site —
    // the reader would open it and not find them.
    expect(help.find('мышью').map((h) => h.slug)).toEqual(['first-steps'])
    expect(help.find('тапом')).toEqual([])
    expect(help.bySlug('first-steps').path).toBe('start/first-steps.md')
  })

  it('сосед по чтению не перепрыгивает через скрытую статью в пустоту', () => {
    const help = useHelpStore()
    help.current = 'shortcuts'
    expect(help.neighbours.prev.slug).toBe('first-steps')
    expect(help.neighbours.next).toBeNull()
  })
})
