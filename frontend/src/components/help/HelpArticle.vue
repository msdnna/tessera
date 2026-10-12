<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { renderMarkdown } from '@/utils/markdown'
import { uniqueHeadingId } from '@/utils/helpSlug'
import { resolveHelpImages } from '@/utils/helpAssets'
import { useThemeStore } from '@/stores/theme'

// Renders one help article (#2792). Markdown → sanitised HTML via the existing
// renderMarkdown (marked + DOMPurify), then h2/h3 get the same ids the build
// script put in the index, so the table of contents links actually land.
const props = defineProps({
  source: { type: String, default: '' },
  // Rendered inside the contextual drawer (#2794) rather than on the help page:
  // links to other articles stay in the drawer instead of navigating away.
  inline: { type: Boolean, default: false },
  // Language of the *text* being rendered, not of the interface (#2816): a
  // reader on English who opens an article nobody translated yet sees Russian
  // prose, and Russian screenshots belong with it.
  lang: { type: String, default: 'ru' },
})

const emit = defineEmits(['open-slug', 'navigate'])

const router = useRouter()
const theme = useThemeStore()

// Text of a heading as the TOC sees it: the builder slugifies the plain
// Markdown text, so tags stripped and entities decoded must produce the same
// string here. Both sides then run it through uniqueHeadingId.
//
// Parsed, not regexed: a DOMParser document is inert (no scripts run, nothing
// loads), and its textContent drops every tag and decodes every entity exactly
// once. A replace chain did neither reliably — a tag split by another survived
// the strip, and "&amp;lt;" (the text "&lt;") came out as "<".
function headingText(inner) {
  return new DOMParser().parseFromString(inner, 'text/html').body.textContent.trim()
}

const html = computed(() => {
  const seen = new Map()
  const withAnchors = renderMarkdown(props.source).replace(
    /<h([23])>([\s\S]*?)<\/h\1>/g,
    (_, level, inner) =>
      `<h${level} id="${uniqueHeadingId(headingText(inner), seen)}">${inner}</h${level}>`,
  )
  // Screenshots are `?raw` text as far as the bundler is concerned, so their
  // links have to be pointed at the built assets here — and the dark reader gets
  // the dark twin of each shot (utils/helpAssets.js).
  return resolveHelpImages(withAnchors, theme.isDark, props.lang)
})

// Cross-links between articles are written as ordinary Markdown links to
// /help/<slug>. Left alone they would be plain anchors and reload the whole SPA,
// so in-app targets are handed to the router instead. External links keep the
// target="_blank" renderMarkdown gives them.
function onClick(e) {
  const a = e.target.closest?.('a[href]')
  if (!a) return
  const href = a.getAttribute('href') || ''
  if (!href.startsWith('/')) return
  e.preventDefault()
  // In the drawer a link to a neighbouring article swaps the drawer's content:
  // the whole point of contextual help is not leaving the screen you are on.
  // Anything else (a link into the app) does navigate, and the drawer closes.
  if (props.inline && href.startsWith('/help/')) {
    emit('open-slug', href.slice('/help/'.length).split('#')[0])
    return
  }
  emit('navigate', href)
  router.push(href)
}
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- sanitized by renderMarkdown -->
  <article class="h-article" @click="onClick" v-html="html" />
</template>

<style scoped>
/* v2 (#3107 — HelpWin): text 15/1.65, no lines — callouts, code and tables are
   surfaces; key names written as <kbd> read as key caps. */
.h-article {
  color: var(--t-text1);
  line-height: 1.65;
  font-size: 15px;
  max-width: 74ch;
}
/* :deep — the content is v-html, so it carries no scope attribute of its own. */
.h-article :deep(h2) {
  font-size: 19px;
  font-weight: 700;
  margin: 30px 0 10px;
  /* The header is sticky above the article, so an anchor jump would otherwise
     park the heading underneath it. */
  scroll-margin-top: 72px;
}
.h-article :deep(h3) {
  font-size: 15.5px;
  font-weight: 700;
  margin: 22px 0 8px;
  scroll-margin-top: 72px;
}
.h-article :deep(h2:first-child),
.h-article :deep(h3:first-child) {
  margin-top: 0;
}
.h-article :deep(p) {
  margin: 0 0 14px;
}
.h-article :deep(ul),
.h-article :deep(ol) {
  margin: 0 0 14px;
  padding-left: 22px;
}
.h-article :deep(li) {
  margin-bottom: 6px;
}
.h-article :deep(a) {
  color: var(--t-primary);
  text-decoration: none;
}
.h-article :deep(a:hover) {
  text-decoration: underline;
}
.h-article :deep(code) {
  background: var(--t-ctl);
  border-radius: 5px;
  padding: 1px 5px;
  font-size: 0.88em;
}
.h-article :deep(kbd) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: var(--t-r-kbd, 5px);
  background: var(--t-kbd);
  color: var(--t-text2);
  font: inherit;
  font-size: 11.5px;
  font-weight: 600;
  line-height: 1;
  vertical-align: 1px;
  box-shadow: 0 1px 1px rgba(0, 0, 0, 0.22);
}
.h-article :deep(pre) {
  background: var(--t-sub);
  border-radius: 12px;
  padding: 12px 14px;
  overflow-x: auto;
  margin: 0 0 16px;
}
.h-article :deep(pre code) {
  background: none;
  padding: 0;
}
/* A quote is the callout: the soft accent, the card radius. */
.h-article :deep(blockquote) {
  margin: 0 0 16px;
  padding: 12px 16px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--t-primary) 13%, transparent);
  color: var(--t-text1);
}
.h-article :deep(blockquote p:last-child) {
  margin-bottom: 0;
}
.h-article :deep(img) {
  max-width: 100%;
  border-radius: 12px;
  box-shadow: var(--t-sh-card);
  display: block;
  margin: 6px 0 18px;
}
/* Tables: a header on the control level and banded rows — no rules. */
.h-article :deep(table) {
  border-collapse: separate;
  border-spacing: 0;
  margin: 0 0 16px;
  font-size: 13.5px;
  border-radius: 12px;
  overflow: hidden;
}
.h-article :deep(th),
.h-article :deep(td) {
  padding: 7px 12px;
  text-align: left;
  vertical-align: top;
}
.h-article :deep(th) {
  background: var(--t-ctl);
  font-weight: 650;
}
.h-article :deep(tr:nth-child(even) td) {
  background: var(--t-sub);
}
.h-article :deep(hr) {
  border: none;
  height: 0;
  margin: 26px 0;
}
</style>
