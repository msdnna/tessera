<script setup>
// «Что нового» in the v2 language (#3000, mockup WhatsNewV2) — in place of
// WhatsNewModal. Three faces of one window (radius 18, --t-pop, a tinted hero):
//
//   the v2 card — once, for an account that used v1: the five things that
//     changed most and «Показать, что изменилось» (↵), which walks the «Что
//     изменилось» mini-tour; «Позже» (Esc) sends the same points to the
//     spotlight queue instead (stores/whatsNew closeV2Intro, stores/hints);
//   the release list — what the user just updated into (data/whatsNew.js,
//     #2749), «Понятно» marks it read;
//   the history — every release, opened from the version stamp (#2812);
//     closing it marks nothing.
// Brand-new accounts see none of it — they get the tour.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NIcon } from 'naive-ui'
import {
  FileTrayOutline,
  FlagOutline,
  OptionsOutline,
  PricetagsOutline,
  SearchOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import ShortcutKeys from '../ShortcutKeys.vue'
import { renderMarkdown } from '@/utils/markdown'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { useTourLauncher } from '@/composables/useTourLauncher'

const { t } = useI18n()
const store = useWhatsNewStore()
const { isMobile } = useResponsive()
const { launch } = useTourLauncher()

const version = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : ''

const intro = computed(() => store.showV2Intro)
const show = computed({
  get: () => intro.value || store.entries.length > 0,
  set: (v) => {
    if (!v) later()
  },
})

// The five points of the v2 card, each with its key from the catalog.
const FEATURES = [
  { key: 'inbox', icon: FileTrayOutline, hue: '#f0a020', keys: 'nav.goInbox' },
  { key: 'stage', icon: FlagOutline, hue: '#2f80ed', keys: 'board.stage' },
  { key: 'view', icon: OptionsOutline, hue: '#0eb0a9', keys: 'view.panel' },
  { key: 'project', icon: PricetagsOutline, hue: '#eb2f96', keys: 'project.window' },
  { key: 'palette', icon: SearchOutline, hue: '#7c5cff', keys: 'nav.palette' },
]
const tileBg = (c) =>
  `linear-gradient(to top right, color-mix(in srgb, ${c} 16%, var(--t-pop)), color-mix(in srgb, ${c} 28%, var(--t-pop)))`

// The release list / history wording.
const pill = computed(() => {
  if (intro.value) return t('app.whatsNew.v2.pill', { v: version })
  const v = store.entries[0]?.version
  return v ? t('app.whatsNew.v2.versionPill', { v }) : ''
})
const title = computed(() => {
  if (intro.value) return t('app.whatsNew.v2.title')
  return t(store.isHistory ? 'app.whatsNew.historyTitle' : 'app.whatsNew.title')
})
const sub = computed(() => {
  if (intro.value) return t('app.whatsNew.v2.sub')
  return t(store.isHistory ? 'app.whatsNew.historySub' : 'app.whatsNew.sub')
})
const primaryLabel = computed(() => {
  if (intro.value) return isMobile.value ? t('app.whatsNew.gotIt') : t('app.whatsNew.v2.showMe')
  return t(store.isHistory ? 'app.whatsNew.close' : 'app.whatsNew.gotIt')
})

function itemsHtml(itemKeys) {
  // One Markdown list (sanitized by renderMarkdown); the entries carry catalogue
  // keys (#2800), so the notes follow the interface language.
  return renderMarkdown(itemKeys.map((key) => `- ${t(key)}`).join('\n'))
}

// Esc, a click outside, «Позже».
function later() {
  if (intro.value) store.closeV2Intro({ tour: false })
  else if (store.isHistory) store.closeHistory()
  else store.dismissModal()
}
// ↵ and the main button.
async function go() {
  if (intro.value) {
    // A phone has no mini-tour (its anchors are the desktop's): «Понятно».
    const walk = !isMobile.value
    await store.closeV2Intro({ tour: walk })
    if (walk) launch('whatschanged')
    return
  }
  later()
}
useShortcuts({ 'whatsnew.go': go }, { when: () => show.value })
</script>

<template>
  <n-modal v-model:show="show" :auto-focus="false">
    <div
      class="wv"
      :class="{ mobile: isMobile }"
      data-testid="whats-new-modal"
      :data-face="intro ? 'v2' : store.isHistory ? 'history' : 'release'"
      role="dialog"
      :aria-label="title"
    >
      <header class="wv-hero">
        <span v-if="pill" class="wv-pill">{{ pill }}</span>
        <h2 class="wv-title">{{ title }}</h2>
        <p class="wv-sub">{{ sub }}</p>
      </header>

      <div v-if="intro" class="wv-list" data-testid="whats-new-v2">
        <div v-for="f in FEATURES" :key="f.key" class="wv-row">
          <span class="wv-tile" :style="{ background: tileBg(f.hue), color: f.hue }">
            <n-icon :component="f.icon" :size="18" />
          </span>
          <span class="wv-rt">
            <span class="wv-rtitle">{{ t(`app.whatsNew.v2.items.${f.key}.title`) }}</span>
            <span class="wv-rsub">{{ t(`app.whatsNew.v2.items.${f.key}.sub`) }}</span>
          </span>
          <ShortcutKeys v-if="!isMobile" :id="f.keys" bare />
        </div>
      </div>
      <div v-else class="wv-list rel">
        <section v-for="rel in store.entries" :key="rel.version" class="wv-rel">
          <header class="wv-rel-head">
            <span class="wv-rel-ic"><n-icon :component="SparklesOutline" :size="14" /></span>
            <span class="wv-rel-title">{{ t(rel.titleKey) }}</span>
            <span class="wv-rel-ver">{{ rel.version }}</span>
          </header>
          <!-- eslint-disable-next-line vue/no-v-html -- sanitized by renderMarkdown -->
          <div class="wv-md" v-html="itemsHtml(rel.itemKeys)" />
        </section>
      </div>

      <footer class="wv-foot">
        <span class="wv-note">{{ intro && !isMobile ? t('app.whatsNew.v2.note') : '' }}</span>
        <button
          v-if="intro && !isMobile"
          type="button"
          class="wv-later"
          data-testid="whats-new-later"
          @click="later"
        >
          {{ t('app.whatsNew.v2.later') }} <ShortcutKeys id="whatsnew.later" />
        </button>
        <button type="button" class="wv-go" data-testid="whats-new-go" @click="go">
          {{ primaryLabel }} <ShortcutKeys v-if="!isMobile" id="whatsnew.go" />
        </button>
      </footer>
    </div>
  </n-modal>
</template>

<style scoped>
.wv {
  width: 580px;
  max-width: calc(100vw - 32px);
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  border-radius: 18px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  overflow: hidden;
  color: var(--t-text1);
}
.wv-hero {
  flex: none;
  padding: 26px 28px 20px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 32%, var(--t-pop))
  );
}
.wv-pill {
  height: 24px;
  width: max-content;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
}
.wv-title {
  margin: 12px 0 0;
  font-size: 25px;
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: 1.2;
}
.wv-sub {
  margin: 6px 0 0;
  max-width: 470px;
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--t-text2);
  text-wrap: pretty;
}
.wv-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 24px 8px;
  overflow-y: auto;
  min-height: 0;
}
.wv-list.rel {
  gap: 16px;
  padding: 16px 28px 12px;
}
.wv-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 9px 4px;
}
.wv-tile {
  flex: 0 0 36px;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.wv-rt {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.wv-rtitle {
  font-size: 14.5px;
  font-weight: 650;
}
.wv-rsub {
  font-size: 13px;
  color: var(--t-text2);
}
.wv-rel-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.wv-rel-ic {
  flex: 0 0 22px;
  width: 22px;
  height: 22px;
  border-radius: 7px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: #fff;
}
.wv-rel-title {
  font-size: 14.5px;
  font-weight: 650;
}
.wv-rel-ver {
  height: 20px;
  display: inline-flex;
  align-items: center;
  padding: 0 7px;
  border-radius: 999px;
  background: var(--t-pill);
  font-size: 11px;
  font-weight: 600;
  color: var(--t-text3);
}
.wv-md {
  padding-left: 30px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
}
.wv-md :deep(ul) {
  margin: 0;
  padding-left: 16px;
}
.wv-md :deep(li) {
  margin: 3px 0;
}
.wv-foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 60px;
  padding: 0 14px 0 28px;
  background: var(--t-sub);
}
.wv-note {
  flex: 1;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text3);
}
.wv-foot button {
  font: inherit;
  border: none;
  cursor: pointer;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 7px;
}
.wv-later {
  height: 32px;
  padding: 0 5px 0 11px;
  border-radius: 9px;
  background: transparent;
  font-size: 13px;
  color: var(--t-text2);
}
.wv-later:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.wv-go {
  height: 34px;
  padding: 0 5px 0 13px;
  border-radius: 9px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 13.5px;
  font-weight: 600;
  box-shadow: 0 2px 10px color-mix(in srgb, var(--t-primary) 34%, transparent);
}
.wv-go :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.wv-foot button:focus {
  outline: none;
}
.wv-foot button:focus-visible {
  box-shadow:
    0 0 0 2px var(--t-sub),
    0 0 0 4px color-mix(in srgb, var(--t-primary) 75%, transparent);
}
.wv.mobile .wv-hero {
  padding: 22px 20px 16px;
}
.wv.mobile .wv-list {
  padding: 8px 14px;
}
.wv.mobile .wv-foot {
  padding: 12px 16px;
}
.wv.mobile .wv-go {
  flex: 1;
  height: 48px;
  justify-content: center;
  border-radius: 14px;
  font-size: 15px;
}
</style>
