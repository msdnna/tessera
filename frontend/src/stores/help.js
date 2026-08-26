import { defineStore } from 'pinia'
import { ref, computed, shallowRef, watch } from 'vue'
import helpIndex from '@/data/helpIndex.json'
import { buildHelpSearch } from '@/utils/helpSearch'
import { i18n } from '@/i18n'

// Help centre state (#2792). Everything here is client-side: the articles are
// Markdown files from docs/help, compiled into the bundle as lazy `?raw` chunks,
// and the index (nav, TOC, search corpus) is the JSON built from them. No API,
// no store hydration, works offline and in the desktop build.
//
// The glob is eager:false — each article is its own chunk, so opening the help
// centre downloads the one article being read, not the whole manual. The keys
// are paths relative to this file; `bodyPath` in the localized article is
// relative to docs/help, hence the prefix when looking one up.
const RAW = import.meta.glob('../../../docs/help/**/*.md', { query: '?raw', import: 'default' })
const RAW_PREFIX = '../../../docs/help/'

// Platform scoping (#2795): the index carries the manual for both clients, and
// an article can be written for one of them only. The web shows the desktop
// text — including for a reader on a phone browser, because the split is
// «desktop site vs. app», not screen width: an Android screenshot on a page the
// reader is looking at in a browser would simply be a lie.
const WEB_ARTICLES = helpIndex.articles.filter((a) => (a.platforms || ['web']).includes('web'))

// Frontmatter is metadata for the index, not content — strip it before render.
const FRONTMATTER_RE = /^---\r?\n[\s\S]*?\r?\n---\r?\n?/

const DEFAULT_LOCALE = 'ru'

// localize resolves an article to the reader's language (#2809). A translation
// lives in `locales[<lang>]` beside the Russian original, not in its place, so
// the Russian article is always here even when a language has no translation
// yet — in that case the reader gets the Russian text with `translated: false`,
// and the UI says so, rather than a blank page. `bodyPath` is the Markdown this
// language renders: the translated web body, or the Russian one as a fallback.
function localize(article, lang) {
  const loc = lang !== DEFAULT_LOCALE ? article.locales?.[lang] : null
  if (!loc) {
    return { ...article, bodyPath: article.path, translated: lang === DEFAULT_LOCALE }
  }
  return {
    ...article,
    title: loc.title,
    category: loc.category,
    keywords: loc.keywords,
    headings: loc.headings,
    text: loc.text,
    updated: loc.updated || article.updated,
    bodyPath: loc.path,
    translated: true,
  }
}

export const useHelpStore = defineStore('help', () => {
  // The whole store follows the interface language: title, category, headings
  // and search corpus are the translated ones, and the body is fetched from the
  // translated file. Recomputed, not read once at import, so switching the
  // language in settings re-renders the manual without a reload.
  const lang = computed(() => i18n.global.locale.value)
  const localized = computed(() => WEB_ARTICLES.map((a) => localize(a, lang.value)))
  const searcher = computed(() => buildHelpSearch(localized.value))

  // shallowRef: the cache holds long strings keyed by `<lang>:<slug>` and is
  // replaced wholesale, so deep reactivity would only cost proxy overhead. The
  // language is part of the key — after switching it, the same slug must not
  // serve the body cached for the previous one.
  const cache = shallowRef(new Map())
  const current = ref(null) // slug of the open article
  const body = ref('') // its markdown, frontmatter stripped
  const loading = ref(false)
  const error = ref('')
  const query = ref('')

  const articles = computed(() => localized.value)

  // Nav tree: categories in index order (the builder already sorted articles by
  // category, then order — one shape on every language, translated captions), so
  // grouping the localized list by its category label keeps that order.
  const categories = computed(() => {
    const out = []
    for (const a of localized.value) {
      let cat = out[out.length - 1]
      if (!cat || cat.name !== a.category) out.push((cat = { name: a.category, articles: [] }))
      cat.articles.push(a)
    }
    return out
  })

  const results = computed(() => (query.value.trim() ? searcher.value(query.value) : []))

  // One-off search for callers with a query box of their own — global search
  // (#2794) runs it while the help page's own `query` keeps whatever the reader
  // last typed there.
  function find(term, limit) {
    return String(term || '').trim() ? searcher.value(term, limit) : []
  }

  const meta = computed(() => localized.value.find((a) => a.slug === current.value) || null)
  const headings = computed(() => meta.value?.headings || [])

  // True when the open article has no translation for the current language and
  // is showing the Russian fallback — the reader is told, not left guessing.
  const untranslated = computed(() => (meta.value ? meta.value.translated === false : false))

  // Language the screenshots follow (#2816). It tracks the article, not the
  // interface: on the Russian fallback the shots stay Russian, so the pictures
  // never disagree with the prose next to them.
  const bodyLang = computed(() => (untranslated.value ? DEFAULT_LOCALE : lang.value))

  const defaultSlug = computed(() => localized.value[0]?.slug || '')

  function bySlug(slug) {
    return localized.value.find((a) => a.slug === slug) || null
  }

  // Neighbours for the «дальше/назад» footer links, in reading order.
  const neighbours = computed(() => {
    const list = localized.value
    const i = list.findIndex((a) => a.slug === current.value)
    if (i < 0) return { prev: null, next: null }
    return { prev: list[i - 1] || null, next: list[i + 1] || null }
  })

  // Two readers at once (#2794): the /help page and the contextual drawer that
  // opens over a board. Each owns its own slug/body/loading/error, so opening a
  // hint from the board does not rewrite the article the help page is showing —
  // they share only the markdown cache, which is what actually costs anything.
  const page = { current, body, loading, error }

  const drawerSlug = ref('')
  const drawerBody = ref('')
  const drawerLoading = ref(false)
  const drawerError = ref('')
  const drawerShown = ref(false)
  const drawer = {
    current: drawerSlug,
    body: drawerBody,
    loading: drawerLoading,
    error: drawerError,
  }

  async function load(slug, into) {
    const article = bySlug(slug)
    if (!article) {
      into.current.value = slug
      into.body.value = ''
      into.error.value = i18n.global.t('help.error.notFound')
      return
    }
    into.current.value = slug
    into.error.value = ''
    const key = `${lang.value}:${slug}`
    const cached = cache.value.get(key)
    if (cached !== undefined) {
      into.body.value = cached
      return
    }
    const loader = RAW[RAW_PREFIX + article.bodyPath]
    if (!loader) {
      // The index and the files went out of sync — `make help-index` was not
      // re-run, or the file was deleted. Say so instead of rendering a blank.
      into.error.value = i18n.global.t('help.error.notInBundle')
      into.body.value = ''
      return
    }
    into.loading.value = true
    try {
      const raw = await loader()
      const md = String(raw).replace(FRONTMATTER_RE, '')
      const next = new Map(cache.value)
      next.set(key, md)
      cache.value = next
      // A slower load that finished after the reader moved on must not paint
      // over the article they are looking at now.
      if (into.current.value === slug) into.body.value = md
    } catch {
      if (into.current.value === slug) {
        into.error.value = i18n.global.t('help.error.loadFailed')
        into.body.value = ''
      }
    } finally {
      into.loading.value = false
    }
  }

  // Switching the interface language re-fetches whatever is open in the new
  // language: the cache is keyed by language, so without this the body would
  // keep the text of the language the reader just left.
  watch(lang, () => {
    if (current.value) load(current.value, page)
    if (drawerShown.value && drawerSlug.value) load(drawerSlug.value, drawer)
  })

  function open(slug) {
    return load(slug, page)
  }

  // The help centre is a modal over whatever the reader is doing (#2792), not a
  // page: opening it must not take anyone off their board. Shown first, loaded
  // after — the article is a lazy chunk, and on a cold cache waiting for it
  // would look like the menu item did nothing.
  const centerShown = ref(false)

  function openCenter(slug) {
    centerShown.value = true
    // No slug means «open the help centre», not «open the first article»: a
    // reader coming back mid-manual keeps their place.
    return open(slug || current.value || defaultSlug.value)
  }

  function closeCenter() {
    centerShown.value = false
  }

  const drawerMeta = computed(
    () => localized.value.find((a) => a.slug === drawerSlug.value) || null,
  )
  const drawerUntranslated = computed(() =>
    drawerMeta.value ? drawerMeta.value.translated === false : false,
  )
  const drawerBodyLang = computed(() =>
    drawerUntranslated.value ? DEFAULT_LOCALE : lang.value,
  )

  // Contextual help: show the panel first, then load. The article is a lazy
  // chunk, so waiting for it before opening would look like the ? button did
  // nothing on a cold cache.
  function openDrawer(slug) {
    drawerShown.value = true
    return load(slug || defaultSlug.value, drawer)
  }

  function closeDrawer() {
    drawerShown.value = false
  }

  return {
    articles,
    categories,
    current,
    body,
    loading,
    error,
    query,
    results,
    meta,
    headings,
    untranslated,
    bodyLang,
    neighbours,
    defaultSlug,
    bySlug,
    find,
    open,
    centerShown,
    openCenter,
    closeCenter,
    drawerShown,
    drawerSlug,
    drawerBody,
    drawerLoading,
    drawerError,
    drawerMeta,
    drawerUntranslated,
    drawerBodyLang,
    openDrawer,
    closeDrawer,
  }
})
