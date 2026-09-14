// Resolving the help manual to the reader's language, and the nav tree that
// comes out of it (#2809, #2932).
//
// Pure and index-shaped on purpose: the store holds the reactive state, but what
// breaks here is data-shaped — a caption that belongs to the category rather
// than to the article, an article that has no translation yet — and none of it
// needs a store to reproduce.

export const DEFAULT_LOCALE = 'ru'

// categoryCaptions maps each Russian category to its caption in `lang`.
//
// A caption belongs to the category, not to the article. An article with no
// translation yet is a supported state — it shows the Russian text with a note —
// but letting it name the group split «Boards» into `Boards / Доски / Boards`
// for an English reader: three headings where the index has one, two of them
// alike. Resolved across the whole index before any article is localized, so an
// untranslated article joins the group instead of interrupting it.
export function categoryCaptions(articles, lang) {
  const map = new Map()
  if (lang === DEFAULT_LOCALE) return map
  for (const a of articles) {
    const caption = a.locales?.[lang]?.category
    if (caption && !map.has(a.category)) map.set(a.category, caption)
  }
  return map
}

// localizeArticle resolves an article to the reader's language (#2809). A
// translation lives in `locales[<lang>]` beside the Russian original, not in its
// place, so the Russian article is always here even when a language has no
// translation yet — in that case the reader gets the Russian text with
// `translated: false`, and the UI says so, rather than a blank page. `bodyPath`
// is the Markdown this language renders: the translated web body, or the Russian
// one as a fallback.
//
// `categoryKey` is the category's identity — the Russian caption the manual is
// authored with, the same string on every language. The nav groups by it; the
// builder sorts by it and calls the contract out in so many words («one shape on
// every language, only the captions differ»).
export function localizeArticle(article, lang, captions) {
  const loc = lang !== DEFAULT_LOCALE ? article.locales?.[lang] : null
  const categoryKey = article.category
  const caption = captions?.get(categoryKey)
  if (!loc) {
    return {
      ...article,
      category: caption || article.category,
      categoryKey,
      bodyPath: article.path,
      translated: lang === DEFAULT_LOCALE,
    }
  }
  return {
    ...article,
    title: loc.title,
    category: caption || loc.category,
    categoryKey,
    keywords: loc.keywords,
    headings: loc.headings,
    text: loc.text,
    updated: loc.updated || article.updated,
    bodyPath: loc.path,
    translated: true,
  }
}

// localizeArticles collapses a whole index to one language, captions first.
export function localizeArticles(articles, lang) {
  const captions = categoryCaptions(articles, lang)
  return articles.map((a) => localizeArticle(a, lang, captions))
}

// groupCategories builds the nav tree: categories in index order (the builder
// already sorted articles by category, then order), so walking the list and
// breaking on a new category keeps that order. The break is on `categoryKey`,
// never on the printed caption — the two differ for an article that has no
// translation yet, and grouping by the caption turned one category into two
// under the same name.
export function groupCategories(articles) {
  const out = []
  for (const a of articles) {
    let cat = out[out.length - 1]
    if (!cat || cat.key !== a.categoryKey) {
      out.push((cat = { key: a.categoryKey, name: a.category, articles: [] }))
    }
    cat.articles.push(a)
  }
  return out
}
