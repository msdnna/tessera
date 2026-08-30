import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { readFileSync, existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { tmpdir } from 'node:os'
import { fileURLToPath } from 'node:url'
import helpIndex from '@/data/helpIndex.json'
import { buildIndex } from '../scripts/build-help-index.mjs'
import { uniqueHeadingId } from '@/utils/helpSlug'
import { withoutCode } from './helpers/helpMarkdown'

// Guards the help content itself (#2792). The articles are hand-written
// Markdown, so the things that break are content mistakes — a link to a renamed
// article, a screenshot that was never committed, a missing frontmatter field —
// and none of those show up until someone opens the page.

const HERE = dirname(fileURLToPath(import.meta.url))
const HELP_DIR = join(HERE, '../../docs/help')

const articles = helpIndex.articles
const slugs = new Set(articles.map((a) => a.slug))
// Every Markdown file the manual ships, base and mobile rewrite alike (#2795):
// a dead link or a missing screenshot is no less dead for being in the variant.
const bodyPaths = articles.flatMap((a) => (a.android ? [a.path, a.android.path] : [a.path]))

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
    for (const path of bodyPaths) {
      const md = readFileSync(join(HELP_DIR, path), 'utf8')
      for (const m of md.matchAll(/\]\((\/help\/[^)#\s]+)(#[^)\s]*)?\)/g)) {
        const target = m[1].replace(/^\/help\//, '')
        expect(slugs.has(target), `${path} → ${m[1]}`).toBe(true)
      }
    }
  })

  it('ссылки на скриншоты указывают на существующие файлы', () => {
    for (const path of bodyPaths) {
      const md = withoutCode(readFileSync(join(HELP_DIR, path), 'utf8'))
      for (const m of md.matchAll(/!\[[^\]]*\]\(([^)\s]+)/g)) {
        const src = m[1]
        if (/^(https?:)?\/\//.test(src)) continue // external image
        expect(existsSync(join(HELP_DIR, dirname(path), src)), `${path} → ${src}`).toBe(true)
      }
    }
  })
})

// Platform scoping (#2795). The manual is written twice where the two clients
// differ: `<slug>.android.md` next to the article is the mobile rewrite, folded
// into the base article as its `android` section. What is worth guarding is not
// the wording but the shape — a variant that the generator quietly failed to
// attach ships as an article the app renders in desktop wording, which is the
// exact bug this task is fixing.
describe('help index: платформы', () => {
  it('у каждой статьи есть список платформ', () => {
    for (const a of articles) {
      expect(a.platforms, a.slug).toContain('web')
    }
  })

  it('мобильный вариант не становится отдельной статьёй', () => {
    // `.android.md` is a body, not an entry: a slug like `faq.android` in the
    // nav would show the reader the same article twice.
    for (const a of articles) {
      expect(a.slug.endsWith('.android'), a.slug).toBe(false)
    }
  })

  it('у мобильного варианта есть файл, свой текст и свои заголовки', () => {
    const withVariant = articles.filter((a) => a.android)
    expect(withVariant.length).toBeGreaterThan(0)
    for (const a of withVariant) {
      expect(a.android.path, a.slug).toBe(a.path.replace(/\.md$/, '.android.md'))
      expect(existsSync(join(HELP_DIR, a.android.path)), a.android.path).toBe(true)
      expect(a.android.text, a.slug).toBeTruthy()
      expect(a.android.text, a.slug).not.toBe(a.text)
      const ids = a.android.headings.map((h) => h.id)
      expect(new Set(ids).size, a.slug).toBe(ids.length)
    }
  })

  it('якоря мобильного варианта воспроизводятся тем же slugify', () => {
    for (const a of articles.filter((x) => x.android)) {
      const seen = new Map()
      for (const h of a.android.headings) {
        expect(uniqueHeadingId(h.text, seen), `${a.slug}.android#${h.id}`).toBe(h.id)
      }
    }
  })
})

// The guards live in the generator, so the only way to assert they fire is to
// build a tree that trips them. A throwaway directory rather than docs/help:
// a suite that needs a broken article committed to the repo to prove the build
// breaks on broken articles is its own kind of broken.
describe('help index: сборка падает на расхождениях (#2795)', () => {
  let dir

  const article = (meta, body = 'Текст статьи.\n') => {
    const front = Object.entries(meta)
      .map(([k, v]) => `${k}: ${v}`)
      .join('\n')
    return `---\n${front}\n---\n\n${body}`
  }

  const write = (rel, content) => {
    mkdirSync(join(dir, dirname(rel)), { recursive: true })
    writeFileSync(join(dir, rel), content)
  }

  const base = { title: 'Доски', category: 'Работа', order: 20, updated: '2026-08-24' }

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), 'help-index-'))
  })

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true })
  })

  it('собирает вариант в секцию android базовой статьи', () => {
    write('boards/boards.md', article(base, 'Перетащите карточку мышью.\n'))
    write('boards/boards.android.md', article(base, 'Задержите палец на карточке.\n'))

    const { articles: built } = buildIndex(dir)
    expect(built).toHaveLength(1)
    expect(built[0].slug).toBe('boards')
    expect(built[0].android.path).toBe('boards/boards.android.md')
    expect(built[0].android.text).toContain('палец')
    expect(built[0].text).toContain('мышью')
  })

  it('вариант без базовой статьи', () => {
    write('boards/boards.android.md', article(base))
    expect(() => buildIndex(dir)).toThrow(/без базовой статьи/)
  })

  it('вариант с другой категорией — навигация у платформ общая', () => {
    write('boards/boards.md', article(base))
    write('boards/boards.android.md', article({ ...base, category: 'Мобильное' }))
    expect(() => buildIndex(dir)).toThrow(/category/)
  })

  it('вариант с другим order', () => {
    write('boards/boards.md', article(base))
    write('boards/boards.android.md', article({ ...base, order: 30 }))
    expect(() => buildIndex(dir)).toThrow(/order/)
  })

  it('вариант у статьи, которой нет в приложении', () => {
    write('boards/boards.md', article({ ...base, platforms: 'web' }))
    write('boards/boards.android.md', article(base))
    expect(() => buildIndex(dir)).toThrow(/некуда показывать/)
  })

  it('неизвестная платформа во фронтматтере', () => {
    write('boards/boards.md', article({ ...base, platforms: 'ios' }))
    expect(() => buildIndex(dir)).toThrow(/неизвестная платформа/)
  })

  it('platforms: android убирает статью из веба, но оставляет в индексе', () => {
    // The index is one file for both clients — the filtering is the consumer's
    // job, so the entry has to survive the build.
    write('boards/boards.md', article({ ...base, platforms: 'android' }))
    const { articles: built } = buildIndex(dir)
    expect(built[0].platforms).toEqual(['android'])
  })
})
