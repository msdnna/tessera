<script setup>
import { computed, watch, ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, NSpin } from 'naive-ui'
import { HelpCircleOutline, ArrowBackOutline, ArrowForwardOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import HelpNav from '@/components/help/HelpNav.vue'
import HelpSearch from '@/components/help/HelpSearch.vue'
import HelpArticle from '@/components/help/HelpArticle.vue'

// The help centre (#2792): navigation, article, table of contents. Content is
// docs-as-code — Markdown in docs/help, compiled into the bundle — so this view
// makes no requests and works offline.
const route = useRoute()
const router = useRouter()
const help = useHelpStore()

// /help with no slug opens the first article rather than an empty pane: there is
// no landing page to show, and the first article is «Первые шаги» by design.
const slug = computed(() => route.params.slug || help.defaultSlug)

watch(slug, (s) => s && help.open(s), { immediate: true })

const activeHeading = ref('')

// Highlights the section being read. IntersectionObserver rather than a scroll
// handler: it fires only when a heading actually crosses the line, instead of on
// every frame of a long scroll.
let observer = null
onMounted(() => {
  if (typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) activeHeading.value = e.target.id
      }
    },
    // Only the band just under the sticky header counts as "current".
    { rootMargin: '-72px 0px -70% 0px' },
  )
  watch(
    () => help.body,
    async () => {
      await new Promise((r) => setTimeout(r, 0)) // let the article paint
      observer.disconnect()
      activeHeading.value = ''
      for (const h of document.querySelectorAll('.h-article h2, .h-article h3')) observer.observe(h)
    },
    { immediate: true },
  )
})

function goToHeading(id) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  activeHeading.value = id
}

function go(article) {
  if (article) router.push(`/help/${article.slug}`)
}
</script>

<template>
  <div class="help">
    <aside class="help-side">
      <div class="help-side-head">
        <n-icon :component="HelpCircleOutline" :size="18" class="grad-icon" />
        <span class="help-title">Справка</span>
      </div>
      <HelpSearch />
      <HelpNav />
    </aside>

    <main class="help-main">
      <div class="help-body">
        <header v-if="help.meta" class="help-head">
          <div class="help-crumb">{{ help.meta.category }}</div>
          <h1 class="help-h1">{{ help.meta.title }}</h1>
          <div v-if="help.meta.updated" class="help-updated">Обновлено {{ help.meta.updated }}</div>
        </header>

        <n-spin v-if="help.loading && !help.body" size="small" />
        <p v-else-if="help.error" class="help-error">{{ help.error }}</p>
        <HelpArticle v-else :source="help.body" />

        <nav v-if="help.neighbours.prev || help.neighbours.next" class="help-neighbours">
          <button
            v-if="help.neighbours.prev"
            type="button"
            class="help-neighbour"
            @click="go(help.neighbours.prev)"
          >
            <n-icon :component="ArrowBackOutline" :size="14" />
            <span>{{ help.neighbours.prev.title }}</span>
          </button>
          <span v-else />
          <button
            v-if="help.neighbours.next"
            type="button"
            class="help-neighbour help-neighbour-next"
            @click="go(help.neighbours.next)"
          >
            <span>{{ help.neighbours.next.title }}</span>
            <n-icon :component="ArrowForwardOutline" :size="14" />
          </button>
        </nav>
      </div>

      <aside v-if="help.headings.length" class="help-toc">
        <div class="help-toc-head">На этой странице</div>
        <button
          v-for="h in help.headings"
          :key="h.id"
          type="button"
          class="help-toc-item"
          :class="{
            'help-toc-sub': h.level === 3,
            'help-toc-active': h.id === activeHeading,
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
.help {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  height: 100%;
  min-height: 0;
}
.help-side {
  border-right: 1px solid var(--t-border);
  padding: 18px 12px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.help-side-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
}
.help-title {
  font-size: 15px;
  font-weight: 650;
  color: var(--t-text1);
}
.help-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  overflow-y: auto;
  min-width: 0;
}
.help-body {
  padding: 28px 32px 64px;
  min-width: 0;
}
.help-crumb {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.help-h1 {
  font-size: 26px;
  font-weight: 700;
  margin: 6px 0 4px;
  color: var(--t-text1);
}
.help-updated {
  font-size: 12px;
  color: var(--t-text3);
  margin-bottom: 22px;
}
.help-error {
  color: var(--t-text2);
}
.help-neighbours {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 40px;
  padding-top: 20px;
  border-top: 1px solid var(--t-border);
}
.help-neighbour {
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
.help-neighbour:hover {
  background: var(--t-hover);
}
.help-neighbour-next {
  justify-content: flex-end;
  text-align: right;
}
.help-toc {
  position: sticky;
  top: 0;
  align-self: start;
  padding: 32px 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.help-toc-head {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
  margin-bottom: 6px;
}
.help-toc-item {
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
.help-toc-item:hover {
  color: var(--t-text1);
}
.help-toc-sub {
  padding-left: 22px;
}
.help-toc-active {
  border-left-color: var(--t-primary);
  color: var(--t-primary);
}

/* Below the two-column breakpoint the table of contents is dropped rather than
   stacked: on a narrow screen it is a second list of the same links the reader
   already scrolled past. */
@media (max-width: 1100px) {
  .help-main {
    grid-template-columns: minmax(0, 1fr);
  }
  .help-toc {
    display: none;
  }
}
@media (max-width: 760px) {
  .help {
    grid-template-columns: minmax(0, 1fr);
  }
  .help-side {
    display: none;
  }
  .help-body {
    padding: 20px 16px 48px;
  }
}
</style>
