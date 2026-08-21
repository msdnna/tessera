import { defineStore } from 'pinia'
import { ref, computed, shallowRef } from 'vue'
import helpIndex from '@/data/helpIndex.json'
import { buildHelpSearch } from '@/utils/helpSearch'

// Help centre state (#2792). Everything here is client-side: the articles are
// Markdown files from docs/help, compiled into the bundle as lazy `?raw` chunks,
// and the index (nav, TOC, search corpus) is the JSON built from them. No API,
// no store hydration, works offline and in the desktop build.
//
// The glob is eager:false — each article is its own chunk, so opening the help
// centre downloads the one article being read, not the whole manual. The keys
// are paths relative to this file; `path` in the index is relative to docs/help,
// hence the prefix when looking one up.
const RAW = import.meta.glob('../../../docs/help/**/*.md', { query: '?raw', import: 'default' })
const RAW_PREFIX = '../../../docs/help/'

const ARTICLES = helpIndex.articles

// Frontmatter is metadata for the index, not content — strip it before render.
const FRONTMATTER_RE = /^---\r?\n[\s\S]*?\r?\n---\r?\n?/

export const useHelpStore = defineStore('help', () => {
  const search = buildHelpSearch(ARTICLES)
  // shallowRef: the cache holds long strings keyed by slug and is replaced
  // wholesale, so deep reactivity would only cost proxy overhead.
  const cache = shallowRef(new Map())
  const current = ref(null) // slug of the open article
  const body = ref('') // its markdown, frontmatter stripped
  const loading = ref(false)
  const error = ref('')
  const query = ref('')

  const articles = computed(() => ARTICLES)

  // Nav tree: categories in index order (the builder already sorted articles by
  // category, then order), each with its articles.
  const categories = computed(() => {
    const out = []
    for (const a of ARTICLES) {
      let cat = out[out.length - 1]
      if (!cat || cat.name !== a.category) out.push((cat = { name: a.category, articles: [] }))
      cat.articles.push(a)
    }
    return out
  })

  const results = computed(() => (query.value.trim() ? search(query.value) : []))

  const meta = computed(() => ARTICLES.find((a) => a.slug === current.value) || null)
  const headings = computed(() => meta.value?.headings || [])

  const defaultSlug = computed(() => ARTICLES[0]?.slug || '')

  function bySlug(slug) {
    return ARTICLES.find((a) => a.slug === slug) || null
  }

  // Neighbours for the «дальше/назад» footer links, in reading order.
  const neighbours = computed(() => {
    const i = ARTICLES.findIndex((a) => a.slug === current.value)
    if (i < 0) return { prev: null, next: null }
    return { prev: ARTICLES[i - 1] || null, next: ARTICLES[i + 1] || null }
  })

  async function open(slug) {
    const article = bySlug(slug)
    if (!article) {
      current.value = slug
      body.value = ''
      error.value = 'Статья не найдена'
      return
    }
    current.value = slug
    error.value = ''
    const cached = cache.value.get(slug)
    if (cached !== undefined) {
      body.value = cached
      return
    }
    const loader = RAW[RAW_PREFIX + article.path]
    if (!loader) {
      // The index and the files went out of sync — `make help-index` was not
      // re-run, or the file was deleted. Say so instead of rendering a blank.
      error.value = 'Статья не найдена в сборке'
      body.value = ''
      return
    }
    loading.value = true
    try {
      const raw = await loader()
      const md = String(raw).replace(FRONTMATTER_RE, '')
      const next = new Map(cache.value)
      next.set(slug, md)
      cache.value = next
      // A slower load that finished after the reader moved on must not paint
      // over the article they are looking at now.
      if (current.value === slug) body.value = md
    } catch {
      if (current.value === slug) {
        error.value = 'Не удалось загрузить статью'
        body.value = ''
      }
    } finally {
      loading.value = false
    }
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
    neighbours,
    defaultSlug,
    bySlug,
    open,
  }
})
