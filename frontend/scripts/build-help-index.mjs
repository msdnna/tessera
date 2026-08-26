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
const ANDROID_TAG = '.android'

// Locale scoping (#2809). An article is written in Russian by default;
// `<slug>.en.md` next to it is the English translation of the same article —
// same slug, same order, same place in the nav, translated title/category/body.
// The locale tag sits at the very end of the name, after the optional platform
// tag, so the four shapes of one article are:
//   first-steps.md              ru · web
//   first-steps.android.md      ru · android
//   first-steps.en.md           en · web
//   first-steps.android.en.md   en · android
// The Russian article stays on the top level of the index; a translation goes
// into `locales[<lang>]` instead of replacing it — Gson on Android ignores the
// unknown field, so an already-installed APK with an older index keeps showing
// the Russian help rather than an empty screen.
const LOCALES = ['ru', 'en']
const DEFAULT_LOCALE = 'ru'
const TRANSLATED = LOCALES.filter((l) => l !== DEFAULT_LOCALE)

// classify splits a Markdown file name into { slug, platform, locale }. The
// locale tag is stripped first (it is outermost), then the platform tag; what
// remains is the slug. Slugs never contain a dot, so a trailing `.en` is
// unambiguously the locale and `.android` unambiguously the platform.
function classify(name) {
  let base = name.slice(0, -'.md'.length)
  let locale = DEFAULT_LOCALE
  const m = /\.([a-z]{2})$/.exec(base)
  if (m && TRANSLATED.includes(m[1])) {
    locale = m[1]
    base = base.slice(0, -m[0].length)
  }
  let platform = 'web'
  if (base.endsWith(ANDROID_TAG)) {
    platform = 'android'
    base = base.slice(0, -ANDROID_TAG.length)
  }
  return { slug: base, platform, locale }
}

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
      throw new Error(
        `${where}: неизвестная платформа «${p}» в «platforms» — можно ${PLATFORMS.join('/')}`,
      )
    }
  }
  if (!platforms.length)
    throw new Error(`${where}: «platforms» пустой — убери поле или укажи платформу`)
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

// bodySection is the part of an article that forks per platform and per locale:
// the body path, its headings and the flattened search corpus. The base article
// carries these at the top level; a mobile rewrite and a translation each get
// their own copy under `android` / `locales[<lang>]`.
function bodySection(rec) {
  return {
    path: rec.path,
    updated: rec.meta.updated,
    keywords: rec.meta.keywords,
    headings: collectHeadings(rec.meta.body),
    text: flatten(rec.meta.body),
  }
}

// samePlatforms compares two platform lists; both are already normalised to
// PLATFORMS order by parseFrontmatter, so a join is enough.
const samePlatforms = (a, b) => a.join(',') === b.join(',')

// attachAndroid folds a `<slug>.android[.<lang>].md` into `target` as its
// `android` section instead of a stand-alone article. `base` is always the
// Russian web article (order and platform scope are single across languages);
// `label` is the category the *same-locale* text is filed under, so a Russian
// rewrite is checked against the Russian category and an English one against the
// English caption. Every mismatch fails the build: a variant that disagrees
// about the category or order would ship as an article reachable on one
// platform/language and not the others — the kind of gap nobody notices until a
// user reports a dead link.
function attachAndroid(target, base, rec, label) {
  if (rec.meta.category !== label) {
    throw new Error(
      `${rec.path}: «category» расходится с ${target.path} — ` +
        `${rec.meta.category} и ${label}; навигация у платформ общая`,
    )
  }
  if (rec.meta.order !== base.order) {
    throw new Error(
      `${rec.path}: «order» расходится с ${base.path} — ` +
        `${rec.meta.order} и ${base.order}; навигация у платформ общая`,
    )
  }
  if (!rec.meta.platforms.includes('android')) {
    throw new Error(`${rec.path}: мобильный вариант с «platforms: ${rec.meta.platforms.join(',')}»`)
  }
  if (!base.platforms.includes('android')) {
    throw new Error(
      `${rec.path}: у ${base.path} стоит «platforms: ${base.platforms.join(',')}» — ` +
        `мобильный вариант некуда показывать`,
    )
  }
  target.android = bodySection(rec)
}

// `dir` is a parameter only so the suite can build a throwaway tree and assert
// the guards below actually fire; every caller in the repo builds docs/help.
export function buildIndex(dir = HELP_DIR) {
  // Bucket every file by slug, then by platform and locale, so the four shapes
  // of one article (ru/en × web/android) land together.
  const bySlug = new Map()
  for (const file of walk(dir)) {
    const rel = relative(dir, file).split(/[\\/]/).join('/')
    const meta = parseFrontmatter(readFileSync(file, 'utf8'), rel)
    const { slug, platform, locale } = classify(basename(file))
    let bucket = bySlug.get(slug)
    if (!bucket) bySlug.set(slug, (bucket = { web: {}, android: {} }))
    if (bucket[platform][locale]) {
      throw new Error(
        `две статьи с одним slug/платформой/локалью «${slug}·${platform}·${locale}»: ` +
          `${bucket[platform][locale].path} и ${rel}`,
      )
    }
    bucket[platform][locale] = { path: rel, meta }
  }

  // catLabels[ruCategory][lang] = the single caption that language files the
  // group under. A category is grouping identity in Russian and a translated
  // caption everywhere else; two articles of one Russian category disagreeing
  // about the English caption would split the nav into twin groups.
  const catLabels = new Map()
  const articles = []
  for (const [slug, bucket] of bySlug) {
    const baseRec = bucket.web[DEFAULT_LOCALE]
    if (!baseRec) {
      // A translation or a mobile rewrite hangs off the Russian web article;
      // without it there is nothing to fork from and the reader would reach the
      // topic on one platform/language and not the others.
      const orphan =
        bucket.android[DEFAULT_LOCALE] ||
        Object.values(bucket.web)[0] ||
        Object.values(bucket.android)[0]
      throw new Error(`${orphan.path}: вариант без базовой статьи — нужен ${slug}.md рядом`)
    }
    const base = {
      slug,
      path: baseRec.path,
      title: baseRec.meta.title,
      category: baseRec.meta.category,
      order: baseRec.meta.order,
      updated: baseRec.meta.updated,
      keywords: baseRec.meta.keywords,
      platforms: baseRec.meta.platforms,
      headings: collectHeadings(baseRec.meta.body),
      text: flatten(baseRec.meta.body),
    }
    if (bucket.android[DEFAULT_LOCALE]) {
      attachAndroid(base, base, bucket.android[DEFAULT_LOCALE], base.category)
    }

    const locales = {}
    for (const lang of TRANSLATED) {
      const webRec = bucket.web[lang]
      const androidRec = bucket.android[lang]
      if (!webRec) {
        if (androidRec) {
          throw new Error(
            `${androidRec.path}: перевод мобильного варианта без ${slug}.${lang}.md рядом`,
          )
        }
        // No translation for this language yet — the parity test (not the
        // builder) is what fails an article that should have one.
        continue
      }
      if (!samePlatforms(webRec.meta.platforms, base.platforms)) {
        throw new Error(
          `${webRec.path}: «platforms» расходится с ${base.path} — ` +
            `${webRec.meta.platforms.join(',')} и ${base.platforms.join(',')}; перевод не меняет охват`,
        )
      }
      if (webRec.meta.order !== base.order) {
        throw new Error(
          `${webRec.path}: «order» расходится с ${base.path} — ` +
            `${webRec.meta.order} и ${base.order}; порядок статей общий на всех языках`,
        )
      }
      let langLabels = catLabels.get(base.category)
      if (!langLabels) catLabels.set(base.category, (langLabels = new Map()))
      const seen = langLabels.get(lang)
      if (seen !== undefined && seen !== webRec.meta.category) {
        throw new Error(
          `${webRec.path}: категория «${webRec.meta.category}» ≠ «${seen}» для той же ` +
            `русской «${base.category}» — навигация ${lang} разъедется на две группы`,
        )
      }
      langLabels.set(lang, webRec.meta.category)

      const loc = {
        path: webRec.path,
        title: webRec.meta.title,
        category: webRec.meta.category,
        updated: webRec.meta.updated,
        keywords: webRec.meta.keywords,
        headings: collectHeadings(webRec.meta.body),
        text: flatten(webRec.meta.body),
      }
      // A translated mobile rewrite is checked against the translated caption,
      // not the Russian one. Its absence when the Russian article has one is a
      // parity gap left to the test, not a build error.
      if (androidRec) attachAndroid(loc, base, androidRec, webRec.meta.category)
      locales[lang] = loc
    }
    if (Object.keys(locales).length) base.locales = locales
    articles.push(base)
  }

  // Sorted here, once, so every consumer (nav, search, prev/next) sees the same
  // order without re-sorting: category by its lowest `order`, then article. The
  // order is computed from the Russian structure and never recomputed per
  // locale — the manual has one shape on every language, only the captions
  // differ.
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
