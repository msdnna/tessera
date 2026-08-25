#!/usr/bin/env node
// Builds src/data/helpIndex.json from the Markdown articles in docs/help (#2792).
//
// Why an index at all: the help centre needs a nav tree, a table of contents and
// a client-side search over every article — but the articles themselves are
// lazy `?raw` chunks, so none of that content is in the main bundle. The index
// carries just the metadata, the h2/h3 headings and a flattened body, so search
// and navigation work before a single article is fetched.
//
// Deliberately dependency-free: `--check` runs inside `make lint-frontend`, and
// pulling a YAML/Markdown parser in for a dozen `key: value` lines would be
// weight for nothing. Anchor ids come from utils/helpSlug.js — the same function
// the renderer uses, so TOC links and rendered headings can never drift apart.

import { readFileSync, readdirSync, writeFileSync, statSync } from 'node:fs'
import { join, relative, dirname, basename } from 'node:path'
import { fileURLToPath } from 'node:url'
import { format, resolveConfig } from 'prettier'
import { uniqueHeadingId } from '../src/utils/helpSlug.js'

const HERE = dirname(fileURLToPath(import.meta.url))
const HELP_DIR = join(HERE, '../../docs/help')
const OUT_FILE = join(HERE, '../src/data/helpIndex.json')

const REQUIRED = ['title', 'category', 'order']

// Platform scoping (#2795). An article is written for the desktop web by
// default; `<slug>.android.md` next to it is the mobile rewrite of the same
// article — same slug, same place in the nav, different body and screenshots.
// `platforms:` narrows an article to one client entirely, for a topic the other
// one does not have.
const PLATFORMS = ['web', 'android']
const ANDROID_SUFFIX = '.android.md'

function walk(dir) {
  const out = []
  for (const name of readdirSync(dir).sort()) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) {
      if (name === 'assets') continue // images, not articles
      out.push(...walk(full))
    } else if (name.endsWith('.md')) {
      out.push(full)
    }
  }
  return out
}

// parseFrontmatter reads the leading `---` block. Values are plain scalars;
// `keywords` is the one comma-separated list, split here so search does not have
// to re-parse it in the browser.
function parseFrontmatter(src, where) {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(src)
  if (!m) throw new Error(`${where}: нет frontmatter-блока (--- … ---) в начале файла`)
  const meta = {}
  for (const line of m[1].split(/\r?\n/)) {
    if (!line.trim()) continue
    const at = line.indexOf(':')
    if (at < 0) throw new Error(`${where}: строка frontmatter без «ключ: значение» — ${line}`)
    meta[line.slice(0, at).trim()] = line.slice(at + 1).trim()
  }
  for (const key of REQUIRED) {
    if (!meta[key]) throw new Error(`${where}: во frontmatter не хватает поля «${key}»`)
  }
  const order = Number(meta.order)
  if (!Number.isFinite(order)) throw new Error(`${where}: «order» не число — ${meta.order}`)
  const platforms = (meta.platforms || PLATFORMS.join(','))
    .split(',')
    .map((p) => p.trim().toLowerCase())
    .filter(Boolean)
  for (const p of platforms) {
    if (!PLATFORMS.includes(p)) {
      throw new Error(`${where}: неизвестная платформа «${p}» в «platforms» — можно ${PLATFORMS.join('/')}`)
    }
  }
  if (!platforms.length) throw new Error(`${where}: «platforms» пустой — убери поле или укажи платформу`)
  return {
    title: meta.title,
    category: meta.category,
    order,
    updated: meta.updated || '',
    keywords: (meta.keywords || '')
      .split(',')
      .map((k) => k.trim())
      .filter(Boolean),
    // Sorted so the index does not churn on the order the author typed them in.
    platforms: PLATFORMS.filter((p) => platforms.includes(p)),
    body: src.slice(m[0].length),
  }
}

// Fenced code is dropped whole: a shell snippet in an article is not prose, and
// leaving it in would let a stray flag name outrank a real heading match.
const FENCE_RE = /^```[\s\S]*?^```/gm

function collectHeadings(body) {
  const seen = new Map()
  const headings = []
  for (const line of body.replace(FENCE_RE, '').split(/\r?\n/)) {
    const m = /^(#{2,3})\s+(.+?)\s*#*\s*$/.exec(line)
    if (!m) continue
    const text = stripInline(m[2])
    headings.push({ id: uniqueHeadingId(text, seen), text, level: m[1].length })
  }
  return headings
}

// stripInline removes the markdown that would otherwise end up inside a search
// term or a TOC label: emphasis markers, inline code ticks, and link syntax
// (keeping the visible label, dropping the target).
function stripInline(s) {
  return s
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/[`*_~]/g, '')
    .trim()
}

// flatten turns the article into one lowercase line of prose for the search
// index — no headings markup, no lists, no tables, collapsed whitespace.
function flatten(body) {
  return body
    .replace(FENCE_RE, ' ')
    .split(/\r?\n/)
    .map((line) =>
      stripInline(
        line
          .replace(/^\s{0,3}#{1,6}\s+/, '')
          .replace(/^\s{0,3}[-*+]\s+/, '')
          .replace(/^\s{0,3}>\s?/, '')
          .replace(/^\s*\|/, ' ')
          .replace(/\|/g, ' '),
      ),
    )
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase()
}

// attachVariants folds every `<slug>.android.md` into its base article as an
// `android` section instead of listing it as an article of its own. The nav, the
// slug and the reading order stay single — only the body, the headings and the
// search corpus fork, which is the whole point: the app must not grow a second
// table of contents that drifts from the site's.
//
// Every mismatch here fails the build. A variant whose base is gone, or one that
// disagrees about the category, would otherwise ship as an article the reader
// can reach on one platform and not the other — the kind of gap nobody notices
// until a user reports a dead link.
function attachVariants(articles, slugs, variants) {
  const bySlug = new Map(articles.map((a) => [a.slug, a]))
  for (const [slug, variant] of variants) {
    const base = bySlug.get(slug)
    if (!base) {
      throw new Error(
        `${variant.path}: мобильный вариант без базовой статьи — нужен ${slug}.md рядом`,
      )
    }
    for (const field of ['category', 'order']) {
      if (variant.meta[field] !== base[field]) {
        throw new Error(
          `${variant.path}: «${field}» расходится с ${slugs.get(slug)} — ` +
            `${variant.meta[field]} и ${base[field]}; навигация у платформ общая`,
        )
      }
    }
    if (!variant.meta.platforms.includes('android')) {
      throw new Error(`${variant.path}: мобильный вариант с «platforms: ${variant.meta.platforms.join(',')}»`)
    }
    if (!base.platforms.includes('android')) {
      throw new Error(
        `${variant.path}: у ${slugs.get(slug)} стоит «platforms: ${base.platforms.join(',')}» — ` +
          `мобильный вариант некуда показывать`,
      )
    }
    base.android = {
      path: variant.path,
      updated: variant.meta.updated,
      keywords: variant.meta.keywords,
      headings: collectHeadings(variant.meta.body),
      text: flatten(variant.meta.body),
    }
  }
}

// `dir` is a parameter only so the suite can build a throwaway tree and assert
// the guards below actually fire; every caller in the repo builds docs/help.
export function buildIndex(dir = HELP_DIR) {
  const articles = []
  const slugs = new Map()
  const variants = new Map() // slug → the parsed `<slug>.android.md`
  for (const file of walk(dir)) {
    const rel = relative(dir, file).split(/[\\/]/).join('/')
    const meta = parseFrontmatter(readFileSync(file, 'utf8'), rel)
    const name = basename(file)
    if (name.endsWith(ANDROID_SUFFIX)) {
      const slug = name.slice(0, -ANDROID_SUFFIX.length)
      if (variants.has(slug)) {
        throw new Error(`два мобильных варианта одного slug «${slug}»: ${variants.get(slug).path} и ${rel}`)
      }
      variants.set(slug, { path: rel, meta })
      continue
    }
    const slug = basename(file, '.md')
    if (slugs.has(slug)) {
      // Slugs are the URL (/help/<slug>), so a duplicate would make one of the
      // two articles unreachable — fail the build rather than pick a winner.
      throw new Error(`дублирующийся slug «${slug}»: ${slugs.get(slug)} и ${rel}`)
    }
    slugs.set(slug, rel)
    articles.push({
      slug,
      path: rel,
      title: meta.title,
      category: meta.category,
      order: meta.order,
      updated: meta.updated,
      keywords: meta.keywords,
      platforms: meta.platforms,
      headings: collectHeadings(meta.body),
      text: flatten(meta.body),
    })
  }
  attachVariants(articles, slugs, variants)
  // Sorted here, once, so every consumer (nav, search, prev/next) sees the same
  // order without re-sorting: category by its lowest `order`, then article.
  const catOrder = new Map()
  for (const a of articles) {
    const cur = catOrder.get(a.category)
    if (cur === undefined || a.order < cur) catOrder.set(a.category, a.order)
  }
  articles.sort(
    (a, b) =>
      catOrder.get(a.category) - catOrder.get(b.category) ||
      a.category.localeCompare(b.category, 'ru') ||
      a.order - b.order ||
      a.title.localeCompare(b.title, 'ru'),
  )
  return { articles }
}

// The index is committed and `make lint-frontend` runs `prettier --check` over
// the whole repo, so the generator has to emit exactly what prettier would.
// Plain JSON.stringify puts every array item on its own line; prettier keeps a
// short array inline. The two agreed by luck until an article turned up with a
// `keywords` list short enough to collapse (#2793) — after that `make
// help-index` and `make lint-frontend` disagreed about the same file.
async function serialize(index) {
  // resolveConfig, not just `filepath`: the programmatic API does not read
  // .prettierrc on its own, and with the default printWidth of 80 (ours is 100)
  // it would produce yet a third formatting of the same file.
  const options = (await resolveConfig(OUT_FILE)) || {}
  return format(JSON.stringify(index, null, 2), { ...options, parser: 'json' })
}

// CLI: `node build-help-index.mjs` writes the file, `--check` only verifies that
// the committed one still matches the sources (that is the lint gate).
if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) {
  const check = process.argv.includes('--check')
  let next
  try {
    next = await serialize(buildIndex())
  } catch (err) {
    console.error(`help-index: ${err.message}`)
    process.exit(1)
  }
  if (check) {
    let current = ''
    try {
      current = readFileSync(OUT_FILE, 'utf8')
    } catch {
      /* missing file — reported as stale below */
    }
    if (current !== next) {
      console.error('help-index: src/data/helpIndex.json протух — запусти `make help-index`')
      process.exit(1)
    }
    console.log('help-index: актуален')
  } else {
    writeFileSync(OUT_FILE, next)
    console.log(`help-index: ${JSON.parse(next).articles.length} статей → src/data/helpIndex.json`)
  }
}
