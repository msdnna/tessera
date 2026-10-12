import { describe, it, expect } from 'vitest'
import helpIndex from '@/data/helpIndex.json'
import { categoryCaptions, localizeArticles, groupCategories } from '@/utils/helpNav'

// The nav tree of the help centre (#2932).
//
// The index is sorted by the Russian category and the builder says so in as many
// words — «the manual has one shape on every language, only the captions
// differ». The nav used to group by the printed caption instead, which is the
// same string only while every article is translated.

const ru = (over = {}) => ({
  slug: 'a',
  path: 'a/a.md',
  title: 'Статья',
  category: 'Доски',
  order: 10,
  text: 'текст',
  keywords: [],
  headings: [],
  ...over,
})

const withEn = (article, over = {}) => ({
  ...article,
  locales: {
    en: {
      path: article.path.replace('.md', '.en.md'),
      title: 'Article',
      category: 'Boards',
      text: 'text',
      keywords: [],
      headings: [],
      ...over,
    },
  },
})

describe('help nav: категория опознаётся ключом, а не подписью (#2932)', () => {
  it('непереведённая статья не разрывает категорию на две группы', () => {
    // Без этого английский читатель видел Boards / Доски / Boards — три
    // заголовка там, где в индексе одна категория, и два из них одинаковые.
    const articles = [
      withEn(ru({ slug: 'a', order: 10 })),
      ru({ slug: 'b', path: 'b/b.md', order: 12, title: 'Только по-русски' }),
      withEn(ru({ slug: 'c', path: 'c/c.md', order: 14 })),
    ]
    const groups = groupCategories(localizeArticles(articles, 'en'))

    expect(groups).toHaveLength(1)
    expect(groups[0].name).toBe('Boards')
    expect(groups[0].key).toBe('Доски')
    expect(groups[0].articles.map((a) => a.slug)).toEqual(['a', 'b', 'c'])
  })

  it('непереведённая статья берёт подпись группы, но не перевод', () => {
    const articles = [withEn(ru({ slug: 'a' })), ru({ slug: 'b', path: 'b/b.md' })]
    const [, untranslated] = localizeArticles(articles, 'en')

    expect(untranslated.category).toBe('Boards')
    expect(untranslated.categoryKey).toBe('Доски')
    // Подпись — свойство категории; текст статьи остаётся русским, и флаг это
    // говорит, иначе экран не покажет пометку о непереведённой статье.
    expect(untranslated.translated).toBe(false)
    expect(untranslated.title).toBe('Статья')
    expect(untranslated.bodyPath).toBe('b/b.md')
  })

  it('на русском подписи свои, карта подписей пуста', () => {
    const articles = [withEn(ru({ slug: 'a' })), ru({ slug: 'b', path: 'b/b.md' })]
    expect(categoryCaptions(articles, 'ru').size).toBe(0)

    const groups = groupCategories(localizeArticles(articles, 'ru'))
    expect(groups).toHaveLength(1)
    expect(groups[0].name).toBe('Доски')
    expect(groups[0].articles).toHaveLength(2)
  })

  it('разные категории остаются разными группами', () => {
    const articles = [
      withEn(ru({ slug: 'a', category: 'Доски' })),
      withEn(ru({ slug: 'b', path: 'b/b.md', category: 'Задачи' }), { category: 'Tasks' }),
    ]
    const groups = groupCategories(localizeArticles(articles, 'en'))

    expect(groups.map((g) => g.name)).toEqual(['Boards', 'Tasks'])
    expect(groups.map((g) => g.key)).toEqual(['Доски', 'Задачи'])
  })

  it('на настоящем руководстве ключи групп уникальны в каждом языке', () => {
    // Ключ группы — это `:key` в HelpNav.vue: повтор здесь означает
    // дублирующийся ключ v-for (и дублирующийся ключ LazyColumn на Android,
    // где это уже не косметика, а падение).
    for (const lang of ['ru', 'en']) {
      const groups = groupCategories(localizeArticles(helpIndex.articles, lang))
      const keys = groups.map((g) => g.key)
      expect(new Set(keys).size, lang).toBe(keys.length)
      expect(new Set(groups.map((g) => g.name)).size, lang).toBe(groups.length)
    }
  })
})
