<script setup>
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSpin } from 'naive-ui'
import { ArrowBackOutline, ArrowForwardOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import { pickActive, measureHeadings } from '@/utils/helpToc'
import HelpNav from './HelpNav.vue'
import HelpArticle from './HelpArticle.vue'
import HelpFeedback from './HelpFeedback.vue'

// The working area of the help centre (#2792): navigation, article, table of
// contents. The search moved into the window's head (v2, #3107). Lives inside the modal (HelpCenterModal) rather than on a route —
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
      <HelpNav />
    </aside>

    <main ref="root" class="hc-main" @scroll.passive="onScroll">
      <div class="hc-body">
        <header v-if="help.meta" class="hc-head">
          <div class="hc-crumb">
            <span>{{ help.meta.category }}</span>
            <template v-if="help.meta.title !== help.meta.category">
              <n-icon :component="ChevronForwardOutline" :size="11" />
              <span class="hc-crumb-cur">{{ help.meta.title }}</span>
            </template>
          </div>
          <h1 class="hc-h1">{{ help.meta.title }}</h1>
          <div v-if="help.meta.updated" class="hc-updated">
            {{ t('help.updated', { date: help.meta.updated }) }}
          </div>
        </header>

        <p v-if="help.untranslated" class="hc-untranslated">{{ t('help.notTranslated') }}</p>

        <n-spin v-if="help.loading && !help.body" size="small" />
        <p v-else-if="help.error" class="hc-error">{{ help.error }}</p>
        <!-- inline: a cross-link to a neighbouring article swaps this pane
             instead of navigating; a link into the app closes the window. -->
        <HelpArticle
          v-else
          :source="help.body"
          :lang="help.bodyLang"
          inline
          @open-slug="help.open($event)"
          @navigate="help.closeCenter()"
        />

        <footer v-if="help.meta && help.body" class="hc-foot">
          <HelpFeedback :slug="help.meta.slug" :lang="help.bodyLang" />
          <span class="hc-foot-spacer" />
          <button
            v-if="help.neighbours.next || help.neighbours.prev"
            type="button"
            class="hc-neighbour"
            :data-dir="help.neighbours.next ? 'next' : 'prev'"
            @click="help.open((help.neighbours.next || help.neighbours.prev).slug)"
          >
            <template v-if="help.neighbours.next">
              {{ t('help.next', { title: help.neighbours.next.title }) }}
              <n-icon :component="ArrowForwardOutline" :size="13" />
            </template>
            <template v-else>
              <n-icon :component="ArrowBackOutline" :size="13" />
              {{ t('help.prev', { title: help.neighbours.prev.title }) }}
            </template>
          </button>
        </footer>
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
/* v2 (#3107 — HelpWin): the navigation is a sunken panel inset from the window
   edge, the article a column up to 560px, «На странице» on the right. No lines. */
.hc {
  display: grid;
  grid-template-columns: 256px minmax(0, 1fr);
  height: 100%;
  min-height: 0;
}
.hc-side {
  margin: 0 0 14px 14px;
  padding: 10px 8px;
  border-radius: 16px;
  background: var(--t-sub);
  overflow-y: auto;
}
.hc-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 200px;
  overflow-y: auto;
  min-width: 0;
}
.hc-body {
  display: flex;
  flex-direction: column;
  padding: 10px 32px 28px;
  min-width: 0;
  min-height: 100%;
  box-sizing: border-box;
}
.hc-body > * {
  max-width: 560px;
  width: 100%;
}
.hc-crumb {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text3);
}
.hc-crumb-cur {
  color: var(--t-text2);
}
.hc-h1 {
  font-size: 26px;
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: -0.01em;
  margin: 14px 0 4px;
  color: var(--t-text1);
}
.hc-updated {
  font-size: 12px;
  color: var(--t-text3);
  margin-bottom: 18px;
}
.hc-error {
  color: var(--t-text2);
}
.hc-untranslated {
  margin: 0 0 16px;
  padding: 9px 12px;
  border-radius: 10px;
  background: var(--t-ctl);
  color: var(--t-text3);
  font-size: 13px;
}
/* The article's foot: «Статья помогла?» on the left, the next one on the
   right. Pushed to the bottom of a short article. */
.hc-foot {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: auto;
  padding-top: 28px;
}
.hc-foot-spacer {
  flex: 1;
}
.hc-neighbour {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.hc-neighbour:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.hc-toc {
  position: sticky;
  top: 0;
  align-self: start;
  padding: 44px 18px 20px 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.hc-toc-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
  margin-bottom: 8px;
}
.hc-toc-item {
  border: none;
  background: transparent;
  padding: 4px 0;
  font: inherit;
  font-size: 12.5px;
  color: var(--t-text2);
  text-align: left;
  cursor: pointer;
  line-height: 1.4;
}
.hc-toc-item:hover {
  color: var(--t-text1);
}
.hc-toc-sub {
  padding-left: 12px;
}
.hc-toc-active {
  color: var(--t-text1);
  font-weight: 650;
}

/* Below these widths the window itself is near-fullscreen, and the side columns
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
    padding: 8px 16px 28px;
  }
}
</style>
