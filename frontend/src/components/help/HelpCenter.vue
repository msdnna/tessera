<script setup>
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSpin } from 'naive-ui'
import { ArrowBackOutline, ArrowForwardOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import { pickActive, measureHeadings } from '@/utils/helpToc'
import HelpNav from './HelpNav.vue'
import HelpSearch from './HelpSearch.vue'
import HelpArticle from './HelpArticle.vue'

// The working area of the help centre (#2792): navigation, article, table of
// contents. Lives inside the modal (HelpCenterModal) rather than on a route —
// the reader asks a question without leaving the board they are working on.
// Content is docs-as-code, compiled into the bundle, so this makes no requests.
const { t } = useI18n()
const help = useHelpStore()

const root = ref(null) // the scrolling column
const activeHeading = ref('')

// Highlights the section being read, computed from the scroll offset of the
// reading column (#2811). This used to be an IntersectionObserver watching a
// band at the top of the column, which cannot answer the question at the end of
// an article: the last headings never reach that band, so the highlight stuck on
// a section already scrolled past. One source of truth — see utils/helpToc.
const HEADINGS = '.h-article h2, .h-article h3'

let frame = 0

// Measured on every recompute rather than cached: the screenshots in an article
// load after it paints and push every heading below them down, so a cached set
// of offsets highlights a section the reader is nowhere near. A handful of
// getBoundingClientRect reads, once per frame, is cheaper than being wrong.
function update() {
  frame = 0
  const el = root.value
  if (!el) return
  activeHeading.value = pickActive(measureHeadings(el, HEADINGS), {
    scrollTop: el.scrollTop,
    clientHeight: el.clientHeight,
    scrollHeight: el.scrollHeight,
  })
}

// Scroll fires far more often than the highlight can change; one recompute per
// frame is enough and keeps a long drag off the layout path.
function onScroll() {
  if (!frame) frame = requestAnimationFrame(update)
}

onMounted(() => {
  watch(
    () => help.body,
    async () => {
      await nextTick() // let the article paint
      update()
    },
    { immediate: true },
  )
})

onBeforeUnmount(() => {
  if (frame) cancelAnimationFrame(frame)
})

// A new article starts at its own beginning: the modal body keeps its scroll
// offset otherwise, dropping the reader into the middle of the next page.
watch(
  () => help.current,
  () => root.value?.scrollTo({ top: 0 }),
)

function goToHeading(id) {
  // Scoped to this modal: the contextual drawer (#2794) can be showing another
  // article whose headings carry the very same ids.
  root.value?.querySelector(`[id="${CSS.escape(id)}"]`)?.scrollIntoView({
    behavior: 'smooth',
    block: 'start',
  })
  activeHeading.value = id
}
</script>

<template>
  <div class="hc">
    <aside class="hc-side">
      <HelpSearch />
      <HelpNav />
    </aside>

    <main ref="root" class="hc-main" @scroll.passive="onScroll">
      <div class="hc-body">
        <header v-if="help.meta" class="hc-head">
          <div class="hc-crumb">{{ help.meta.category }}</div>
          <h1 class="hc-h1">{{ help.meta.title }}</h1>
          <div v-if="help.meta.updated" class="hc-updated">
            {{ t('help.updated', { date: help.meta.updated }) }}
          </div>
        </header>

        <n-spin v-if="help.loading && !help.body" size="small" />
        <p v-else-if="help.error" class="hc-error">{{ help.error }}</p>
        <!-- inline: a cross-link to a neighbouring article swaps this pane
             instead of navigating; a link into the app closes the modal. -->
        <HelpArticle
          v-else
          :source="help.body"
          inline
          @open-slug="help.open($event)"
          @navigate="help.closeCenter()"
        />

        <nav v-if="help.neighbours.prev || help.neighbours.next" class="hc-neighbours">
          <button
            v-if="help.neighbours.prev"
            type="button"
            class="hc-neighbour"
            @click="help.open(help.neighbours.prev.slug)"
          >
            <n-icon :component="ArrowBackOutline" :size="14" />
            <span>{{ help.neighbours.prev.title }}</span>
          </button>
          <span v-else />
          <button
            v-if="help.neighbours.next"
            type="button"
            class="hc-neighbour hc-neighbour-next"
            @click="help.open(help.neighbours.next.slug)"
          >
            <span>{{ help.neighbours.next.title }}</span>
            <n-icon :component="ArrowForwardOutline" :size="14" />
          </button>
        </nav>
      </div>

      <aside v-if="help.headings.length" class="hc-toc">
        <div class="hc-toc-head">{{ t('help.onThisPage') }}</div>
        <button
          v-for="h in help.headings"
          :key="h.id"
          type="button"
          class="hc-toc-item"
          :class="{
            'hc-toc-sub': h.level === 3,
            'hc-toc-active': h.id === activeHeading,
          }"
          @click="goToHeading(h.id)"
        >
          {{ h.text }}
        </button>
      </aside>
    </main>
  </div>
</template>

<style scoped>
.hc {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  height: 100%;
  min-height: 0;
}
.hc-side {
  border-right: 1px solid var(--t-border);
  padding: 16px 12px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.hc-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 200px;
  overflow-y: auto;
  min-width: 0;
}
.hc-body {
  padding: 24px 28px 48px;
  min-width: 0;
}
.hc-crumb {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.hc-h1 {
  font-size: 24px;
  font-weight: 700;
  margin: 6px 0 4px;
  color: var(--t-text1);
}
.hc-updated {
  font-size: 12px;
  color: var(--t-text3);
  margin-bottom: 20px;
}
.hc-error {
  color: var(--t-text2);
}
.hc-neighbours {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 36px;
  padding-top: 20px;
  border-top: 1px solid var(--t-border);
}
.hc-neighbour {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  text-align: left;
}
.hc-neighbour:hover {
  background: var(--t-hover);
}
.hc-neighbour-next {
  justify-content: flex-end;
  text-align: right;
}
.hc-toc {
  position: sticky;
  top: 0;
  align-self: start;
  padding: 28px 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.hc-toc-head {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
  margin-bottom: 6px;
}
.hc-toc-item {
  border: none;
  background: transparent;
  border-left: 2px solid var(--t-border);
  padding: 4px 10px;
  font: inherit;
  font-size: 12px;
  color: var(--t-text2);
  text-align: left;
  cursor: pointer;
  line-height: 1.4;
}
.hc-toc-item:hover {
  color: var(--t-text1);
}
.hc-toc-sub {
  padding-left: 22px;
}
.hc-toc-active {
  border-left-color: var(--t-primary);
  color: var(--t-primary);
}

/* Below these widths the modal itself is near-fullscreen, and the side columns
   stop earning their space: the TOC is a second copy of links already scrolled
   past, and the nav is one tap away behind the search box. */
@media (max-width: 1100px) {
  .hc-main {
    grid-template-columns: minmax(0, 1fr);
  }
  .hc-toc {
    display: none;
  }
}
@media (max-width: 760px) {
  .hc {
    grid-template-columns: minmax(0, 1fr);
  }
  .hc-side {
    display: none;
  }
  .hc-body {
    padding: 18px 16px 40px;
  }
}
</style>
