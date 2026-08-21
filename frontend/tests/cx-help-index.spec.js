import { describe, it, expect } from 'vitest'
import { readFileSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import helpIndex from '@/data/helpIndex.json'
import { buildIndex } from '../scripts/build-help-index.mjs'
import { uniqueHeadingId } from '@/utils/helpSlug'

// Guards the help content itself (#2792). The articles are hand-written
// Markdown, so the things that break are content mistakes — a link to a renamed
// article, a screenshot that was never committed, a missing frontmatter field —
// and none of those show up until someone opens the page.

const HERE = dirname(fileURLToPath(import.meta.url))
const HELP_DIR = join(HERE, '../../docs/help')

const articles = helpIndex.articles
const slugs = new Set(articles.map((a) => a.slug))

describe('help index', () => {
  it('не пустой', () => {
    expect(articles.length).toBeGreaterThan(0)
  })

  it('совпадает с тем, что собирается из docs/help прямо сейчас', () => {
    // The committed JSON ships in the bundle; `make lint-frontend` runs the same
    // comparison, this keeps it failing in the test suite too.
    expect(buildIndex()).toEqual(helpIndex)
  })

  it('у каждой статьи есть title, category и файл на диске', () => {
    for (const a of articles) {
      expect(a.title, a.slug).toBeTruthy()
      expect(a.category, a.slug).toBeTruthy()
      expect(existsSync(join(HELP_DIR, a.path)), a.path).toBe(true)
    }
  })

  it('slug-и уникальны', () => {
    expect(slugs.size).toBe(articles.length)
  })

  it('якоря заголовков уникальны внутри статьи', () => {
    for (const a of articles) {
      const ids = a.headings.map((h) => h.id)
      expect(new Set(ids).size, a.slug).toBe(ids.length)
    }
  })

  it('якоря воспроизводятся из текста заголовка тем же slugify', () => {
    // Same invariant the renderer relies on: it re-derives ids from the rendered
    // <h2>/<h3> text, so if these two ever disagreed every TOC link would scroll
    // nowhere.
    for (const a of articles) {
      const seen = new Map()
      for (const h of a.headings) {
        expect(uniqueHeadingId(h.text, seen), `${a.slug}#${h.id}`).toBe(h.id)
      }
    }
  })

  it('внутренние ссылки /help/<slug> ведут на существующие статьи', () => {
    for (const a of articles) {
      const md = readFileSync(join(HELP_DIR, a.path), 'utf8')
      for (const m of md.matchAll(/\]\((\/help\/[^)#\s]+)(#[^)\s]*)?\)/g)) {
        const target = m[1].replace(/^\/help\//, '')
        expect(slugs.has(target), `${a.path} → ${m[1]}`).toBe(true)
      }
    }
  })

  it('ссылки на скриншоты указывают на существующие файлы', () => {
    for (const a of articles) {
      const md = readFileSync(join(HELP_DIR, a.path), 'utf8')
      for (const m of md.matchAll(/!\[[^\]]*\]\(([^)\s]+)/g)) {
        const src = m[1]
        if (/^(https?:)?\/\//.test(src)) continue // external image
        expect(existsSync(join(HELP_DIR, dirname(a.path), src)), `${a.path} → ${src}`).toBe(true)
      }
    }
  })
})
