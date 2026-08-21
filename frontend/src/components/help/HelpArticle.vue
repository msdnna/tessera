<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { renderMarkdown } from '@/utils/markdown'
import { uniqueHeadingId } from '@/utils/helpSlug'

// Renders one help article (#2792). Markdown → sanitised HTML via the existing
// renderMarkdown (marked + DOMPurify), then h2/h3 get the same ids the build
// script put in the index, so the table of contents links actually land.
const props = defineProps({
  source: { type: String, default: '' },
})

const router = useRouter()

// Text of a heading as the TOC sees it: the builder slugifies the plain
// Markdown text, so tags stripped and entities decoded must produce the same
// string here. Both sides then run it through uniqueHeadingId.
function headingText(inner) {
  return inner
    .replace(/<[^>]*>/g, '')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .trim()
}

const html = computed(() => {
  const seen = new Map()
  return renderMarkdown(props.source).replace(
    /<h([23])>([\s\S]*?)<\/h\1>/g,
    (_, level, inner) =>
      `<h${level} id="${uniqueHeadingId(headingText(inner), seen)}">${inner}</h${level}>`,
  )
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
  router.push(href)
}
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- sanitized by renderMarkdown -->
  <article class="h-article" @click="onClick" v-html="html" />
</template>

<style scoped>
.h-article {
  color: var(--t-text1);
  line-height: 1.65;
  font-size: 14px;
  max-width: 74ch;
}
/* :deep — the content is v-html, so it carries no scope attribute of its own. */
.h-article :deep(h2) {
  font-size: 19px;
  font-weight: 650;
  margin: 32px 0 12px;
  /* The header is sticky above the article, so an anchor jump would otherwise
     park the heading underneath it. */
  scroll-margin-top: 72px;
}
.h-article :deep(h3) {
  font-size: 15px;
  font-weight: 650;
  margin: 24px 0 8px;
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
  background: var(--t-surface-alt);
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 0.9em;
}
.h-article :deep(pre) {
  background: var(--t-surface-alt);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  padding: 12px 14px;
  overflow-x: auto;
  margin: 0 0 16px;
}
.h-article :deep(pre code) {
  background: none;
  padding: 0;
}
.h-article :deep(blockquote) {
  margin: 0 0 16px;
  padding: 2px 14px;
  border-left: 3px solid var(--t-border);
  color: var(--t-text2);
}
.h-article :deep(img) {
  max-width: 100%;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  display: block;
  margin: 4px 0 18px;
}
.h-article :deep(table) {
  border-collapse: collapse;
  margin: 0 0 16px;
  font-size: 13px;
}
.h-article :deep(th),
.h-article :deep(td) {
  border: 1px solid var(--t-border);
  padding: 6px 10px;
  text-align: left;
}
.h-article :deep(th) {
  background: var(--t-surface-alt);
}
.h-article :deep(hr) {
  border: none;
  border-top: 1px solid var(--t-border);
  margin: 24px 0;
}
</style>
