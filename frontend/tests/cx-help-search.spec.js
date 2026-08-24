import { describe, it, expect } from 'vitest'
import { buildHelpSearch, tokenize, excerpt } from '@/utils/helpSearch'

// A fixture rather than the real index: these assertions are about ranking
// rules, and pinning them to article text that is meant to be rewritten would
// make the suite fail on every edit to the docs.
const ARTICLES = [
  {
    slug: 'boards',
    title: 'Доски и задачи',
    category: 'Работа',
    keywords: ['канбан', 'колонки'],
    headings: [{ id: 'группировка', text: 'Группировка по тегам', level: 2 }],
    text: 'задача переносится между колонками перетаскиванием мышью',
  },
  {
    slug: 'faq',
    title: 'Частые вопросы',
    category: 'Частые вопросы',
    keywords: ['faq'],
    headings: [{ id: 'тема', text: 'Как включить тёмную тему', level: 2 }],
    text: 'доски настраиваются в настройках доски; тёмная тема включается там же',
  },
]

describe('tokenize', () => {
  it('разбивает кириллицу и цифры, приводит к нижнему регистру', () => {
    expect(tokenize('Доски И Задачи 2026')).toEqual(['доски', 'и', 'задачи', '2026'])
  })

  it('режет по дефису и пунктуации', () => {
    expect(tokenize('QR-код, тест.')).toEqual(['qr', 'код', 'тест'])
  })

  it('пустой ввод — пустой список, без падения на null', () => {
    expect(tokenize('')).toEqual([])
    expect(tokenize(null)).toEqual([])
  })
})

describe('buildHelpSearch', () => {
  const search = buildHelpSearch(ARTICLES)

  it('пустой запрос не выдаёт результатов', () => {
    expect(search('')).toEqual([])
    expect(search('   ')).toEqual([])
  })

  it('находит по префиксу — «доск» достаёт «Доски и задачи»', () => {
    expect(search('доск').map((r) => r.slug)).toContain('boards')
  })

  it('совпадение в заголовке ранжируется выше совпадения в тексте', () => {
    // «доски» есть и в заголовке первой статьи, и в теле второй.
    const [first] = search('доски')
    expect(first.slug).toBe('boards')
  })

  it('несколько слов — только статьи, где нашлись все', () => {
    expect(search('тёмная тема').map((r) => r.slug)).toEqual(['faq'])
    expect(search('тёмная перетаскиванием')).toEqual([])
  })

  it('находит по ключевым словам из фронтматтера', () => {
    expect(search('канбан').map((r) => r.slug)).toEqual(['boards'])
  })

  it('находит по подзаголовку', () => {
    expect(search('группировка').map((r) => r.slug)).toEqual(['boards'])
  })

  it('несуществующее слово — пусто', () => {
    expect(search('криптовалюта')).toEqual([])
  })

  it('отдаёт заголовок, категорию и фрагмент', () => {
    const [r] = search('колонками')
    expect(r).toMatchObject({ slug: 'boards', title: 'Доски и задачи', category: 'Работа' })
    expect(r.excerpt).toContain('колонками')
  })

  it('уважает limit', () => {
    expect(search('доски', 1)).toHaveLength(1)
  })
})

describe('excerpt', () => {
  it('вырезает окрестность найденного слова', () => {
    const long = 'а'.repeat(200) + ' искомое ' + 'б'.repeat(200)
    const out = excerpt(long, ['искомое'])
    expect(out).toContain('искомое')
    expect(out.startsWith('…')).toBe(true)
    expect(out.endsWith('…')).toBe(true)
  })

  it('без совпадения в теле берёт начало статьи', () => {
    expect(excerpt('короткий текст', ['заголовочное'])).toBe('короткий текст')
  })

  it('пустой текст — пустая строка', () => {
    expect(excerpt('', ['что-то'])).toBe('')
  })
})
