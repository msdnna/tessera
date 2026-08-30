import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { readFileSync, existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { tmpdir } from 'node:os'
import { fileURLToPath } from 'node:url'
import helpIndex from '@/data/helpIndex.json'
import { buildIndex } from '../scripts/build-help-index.mjs'
import { uniqueHeadingId } from '@/utils/helpSlug'
import { useHelpStore } from '@/stores/help'
import { withoutCode } from './helpers/helpMarkdown'
import { i18n } from '@/i18n'

// Localisation of the help manual (#2809). The articles are hand-written in
// Russian; each has an English translation beside it (`<slug>.en.md`) that the
// index folds into `locales.en`. What breaks is content-shaped — a missing
// translation, a link that only resolves in one language, a category caption
// that drifts — and none of it shows until an English reader opens the page.

const HERE = dirname(fileURLToPath(import.meta.url))
const HELP_DIR = join(HERE, '../../docs/help')
const articles = helpIndex.articles

// Every Markdown file the English manual ships: the web translation and, where
// the article has a mobile rewrite, the translated rewrite too.
const enBodyPaths = articles.flatMap((a) => {
  const en = a.locales?.en
  if (!en) return []
  return en.android ? [en.path, en.android.path] : [en.path]
})

describe('help i18n: паритет переводов (#2809)', () => {
  // The ratchet: an article without a translation would silently show an English
  // reader the Russian text. Same mechanism as `ut-no-ru-literals` for the UI —
  // once green, a new untranslated article turns it red.
  it('у каждой статьи есть английский перевод', () => {
    for (const a of articles) {
      expect(a.locales?.en, `${a.slug}: нет locales.en`).toBeTruthy()
    }
  })

  it('у каждого мобильного варианта есть английский мобильный вариант', () => {
    for (const a of articles.filter((x) => x.android)) {
      expect(a.locales?.en?.android, `${a.slug}: нет locales.en.android`).toBeTruthy()
    }
  })

  it('у перевода есть заголовок, категория, текст и файл на диске', () => {
    for (const a of articles) {
      const en = a.locales.en
      expect(en.title, a.slug).toBeTruthy()
      expect(en.category, a.slug).toBeTruthy()
      expect(en.text, a.slug).toBeTruthy()
      expect(existsSync(join(HELP_DIR, en.path)), en.path).toBe(true)
    }
  })

  it('перевод отличается от оригинала — это не копия', () => {
    for (const a of articles) {
      expect(a.locales.en.title !== a.title || a.locales.en.text !== a.text, a.slug).toBe(true)
    }
  })

  it('английская категория одна на всю русскую категорию', () => {
    // The build already fails on a split, but the invariant is worth pinning:
    // one Russian category must map to exactly one English caption, or the nav
    // grows twin groups.
    const map = new Map()
    for (const a of articles) {
      const ru = a.category
      const en = a.locales.en.category
      if (map.has(ru)) expect(map.get(ru), ru).toBe(en)
      else map.set(ru, en)
    }
  })

  it('якоря английских заголовков уникальны и воспроизводятся тем же slugify', () => {
    for (const a of articles) {
      for (const section of [a.locales.en, a.locales.en.android].filter(Boolean)) {
        const seen = new Map()
        for (const h of section.headings) {
          expect(uniqueHeadingId(h.text, seen), `${a.slug}#${h.id}`).toBe(h.id)
        }
      }
    }
  })

  it('внутренние ссылки в переводах ведут на существующие статьи', () => {
    const slugs = new Set(articles.map((a) => a.slug))
    for (const path of enBodyPaths) {
      const md = readFileSync(join(HELP_DIR, path), 'utf8')
      for (const m of md.matchAll(/\]\((\/help\/[^)#\s]+)(#[^)\s]*)?\)/g)) {
        const target = m[1].replace(/^\/help\//, '')
        expect(slugs.has(target), `${path} → ${m[1]}`).toBe(true)
      }
    }
  })

  it('ссылки на скриншоты в переводах указывают на существующие файлы', () => {
    for (const path of enBodyPaths) {
      const md = withoutCode(readFileSync(join(HELP_DIR, path), 'utf8'))
      for (const m of md.matchAll(/!\[[^\]]*\]\(([^)\s]+)/g)) {
        const src = m[1]
        if (/^(https?:)?\/\//.test(src)) continue
        expect(existsSync(join(HELP_DIR, dirname(path), src)), `${path} → ${src}`).toBe(true)
      }
    }
  })
})

// The generator's locale guards, asserted by building a throwaway tree that
// trips each one — the same seam `cx-help-index.spec.js` uses for the platform
// guards.
describe('help i18n: сборка падает на расхождениях перевода (#2809)', () => {
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

  const ru = { title: 'Доски', category: 'Работа', order: 20, updated: '2026-08-24' }
  const en = { title: 'Boards', category: 'Work', order: 20, updated: '2026-08-24' }

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), 'help-i18n-'))
  })
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true })
  })

  it('складывает перевод в locales.en базовой статьи', () => {
    write('boards/boards.md', article(ru, 'Перетащите мышью.\n'))
    write('boards/boards.en.md', article(en, 'Drag with the mouse.\n'))
    const { articles: built } = buildIndex(dir)
    expect(built).toHaveLength(1)
    expect(built[0].locales.en.title).toBe('Boards')
    expect(built[0].locales.en.category).toBe('Work')
    expect(built[0].locales.en.text).toContain('mouse')
  })

  it('складывает переведённый мобильный вариант в locales.en.android', () => {
    write('boards/boards.md', article(ru, 'Мышью.\n'))
    write('boards/boards.android.md', article(ru, 'Пальцем.\n'))
    write('boards/boards.en.md', article(en, 'Mouse.\n'))
    write('boards/boards.android.en.md', article(en, 'Finger.\n'))
    const { articles: built } = buildIndex(dir)
    expect(built[0].locales.en.android.path).toBe('boards/boards.android.en.md')
    expect(built[0].locales.en.android.text).toContain('finger')
  })

  it('перевод без базовой статьи', () => {
    write('boards/boards.en.md', article(en))
    expect(() => buildIndex(dir)).toThrow(/без базовой статьи/)
  })

  it('переведённый мобильный вариант без переведённой базовой', () => {
    write('boards/boards.md', article(ru))
    write('boards/boards.android.md', article(ru))
    write('boards/boards.android.en.md', article(en))
    expect(() => buildIndex(dir)).toThrow(/перевод мобильного варианта без/)
  })

  it('перевод с другим order', () => {
    write('boards/boards.md', article(ru))
    write('boards/boards.en.md', article({ ...en, order: 30 }))
    expect(() => buildIndex(dir)).toThrow(/order/)
  })

  it('перевод, сужающий платформы', () => {
    write('boards/boards.md', article(ru))
    write('boards/boards.en.md', article({ ...en, platforms: 'web' }))
    expect(() => buildIndex(dir)).toThrow(/platforms/)
  })

  it('две статьи одной русской категории с разными английскими подписями', () => {
    write('a/a.md', article({ ...ru, order: 10 }))
    write('a/a.en.md', article({ ...en, category: 'Work', order: 10 }))
    write('b/b.md', article({ ...ru, order: 12 }))
    write('b/b.en.md', article({ ...en, category: 'Labour', order: 12 }))
    expect(() => buildIndex(dir)).toThrow(/разъедется/)
  })
})

// The store resolves the article to the interface language and caches its body
// by `<lang>:<slug>` — the pitfall being a cache keyed by the bare slug, which
// would serve the previous language's text after a switch (#2809).
describe('help i18n: стор следует языку интерфейса (#2809)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    i18n.global.locale.value = 'ru'
  })
  afterEach(() => {
    i18n.global.locale.value = 'ru'
  })

  it('заголовок и категория берутся из перевода', async () => {
    const help = useHelpStore()
    const ruMeta = help.bySlug('first-steps')
    i18n.global.locale.value = 'en'
    const enMeta = help.bySlug('first-steps')
    expect(enMeta.title).not.toBe(ruMeta.title)
    expect(enMeta.translated).toBe(true)
  })

  it('смена языка перезагружает тело, а не отдаёт кеш прошлого языка', async () => {
    const help = useHelpStore()
    await help.open('first-steps')
    const ruBody = help.body
    expect(ruBody).toBeTruthy()

    i18n.global.locale.value = 'en'
    // The watcher reloads asynchronously; give it a tick.
    await Promise.resolve()
    await help.open('first-steps')
    const enBody = help.body
    expect(enBody).not.toBe(ruBody)

    // Back to Russian must restore the Russian body, not keep the English one.
    i18n.global.locale.value = 'ru'
    await help.open('first-steps')
    expect(help.body).toBe(ruBody)
  })
})
