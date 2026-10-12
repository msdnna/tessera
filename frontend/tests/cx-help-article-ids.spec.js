import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

// HelpArticle stamps h2/h3 with the ids the index builder computed from the
// Markdown text, so the TOC links land. The rendered heading has to give back
// that same text: tags dropped, every entity decoded exactly once.
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

const HelpArticle = (await import('@/components/help/HelpArticle.vue')).default

const ids = (source) =>
  mount(HelpArticle, { props: { source } })
    .findAll('h2, h3')
    .map((h) => h.attributes('id'))

describe('HelpArticle heading ids', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('matches the builder on plain, emphasised and code headings', () => {
    expect(ids('## Как это работает\n\n### Клавиши **доски** и `⌘K`')).toEqual([
      'как-это-работает',
      'клавиши-доски-и-k',
    ])
  })

  it('decodes entities once: "&amp;lt;" is the text "&lt;", not "<"', () => {
    // The old replace chain unescaped &amp; first and then read the "&lt;" it
    // had just produced as "<", dropping "lt" from the id.
    expect(ids('## a &amp;lt; b')).toEqual(['a-lt-b'])
  })

  it('keeps ids unique within the article', () => {
    expect(ids('## Итог\n\n## Итог')).toEqual(['итог', 'итог-2'])
  })
})
