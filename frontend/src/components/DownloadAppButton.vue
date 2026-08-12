<script setup>
// App-download control on the login screen (web only). Two shapes:
//
//  - The visitor's platform HAS a build → a full split button
//    «⤓ Загрузить для <ОС> | ▾»: the labelled part downloads that platform's
//    recommended build directly (AppImage on Linux), the caret opens the full
//    per-platform menu.
//  - No build for this platform (mac / iOS / unknown) → a round icon button that
//    just opens the menu; no caret is drawn.
//
// Rounded ends match the sibling tool buttons. Links are version-correct URLs
// pulled from the published manifests at runtime (see composables/useDownloads.js
// and DownloadMenu.vue).
import { computed } from 'vue'
import { NIcon, NPopover } from 'naive-ui'
import {
  LogoWindows,
  LogoTux,
  LogoAndroid,
  DownloadOutline,
  ChevronDownOutline,
} from '@vicons/ionicons5'
import { useDownloads } from '@/composables/useDownloads'
import DownloadMenu from '@/components/DownloadMenu.vue'

const { detected, android, windows, linux } = useDownloads()

const META = {
  windows: { name: 'Windows', icon: LogoWindows, recommend: 'exe' },
  linux: { name: 'Linux', icon: LogoTux, recommend: 'appimage' },
  android: { name: 'Android', icon: LogoAndroid, recommend: 'apk' },
}
const DATA = { windows, linux, android }
const DEFAULT_ORDER = ['windows', 'linux', 'android']

// Detected platform first, then the rest in a stable order; drop platforms with
// no published build.
const groups = computed(() => {
  const order = [detected.value, ...DEFAULT_ORDER.filter((k) => k !== detected.value)]
  return order
    .filter((key) => META[key] && DATA[key]?.value)
    .map((key) => {
      const data = DATA[key].value
      const meta = META[key]
      // Only badge the recommended variant when there's a choice to guide (Linux
      // has AppImage/.deb/.rpm); a lone variant needs no "recommended" tag.
      const multi = data.variants.length > 1
      return {
        key,
        name: meta.name,
        icon: meta.icon,
        version: data.version,
        single: !multi,
        recommend: meta.recommend,
        variants: data.variants.map((v) => ({
          ...v,
          recommended: multi && v.format === meta.recommend,
        })),
      }
    })
})

// The group for the visitor's own platform, if we ship a build for it.
const detectedGroup = computed(() => groups.value.find((g) => g.key === detected.value) || null)

// The one-click download for the split button: the platform's recommended
// variant (AppImage on Linux), else its sole build.
const primary = computed(() => {
  const g = detectedGroup.value
  if (!g) return null
  return g.variants.find((v) => v.format === g.recommend) || g.variants[0]
})
</script>

<template>
  <!-- Current platform has a build → labelled split button. -->
  <div v-if="detectedGroup" class="dl-split">
    <a
      class="dl-split-main"
      :href="primary.url"
      download
      rel="noopener noreferrer"
      :title="`Загрузить для ${detectedGroup.name}`"
    >
      <n-icon :component="detectedGroup.icon" :size="18" />
      <span class="dl-split-label">Загрузить для {{ detectedGroup.name }}</span>
    </a>
    <span class="dl-split-sep" aria-hidden="true" />
    <n-popover trigger="click" placement="bottom-end">
      <template #trigger>
        <button class="dl-split-caret" type="button" aria-label="Другие платформы">
          <n-icon :component="ChevronDownOutline" :size="14" />
        </button>
      </template>
      <DownloadMenu :groups="groups" />
    </n-popover>
  </div>

  <!-- No build for this platform → round icon button, dropdown only, no caret. -->
  <n-popover v-else-if="groups.length" trigger="click" placement="bottom-end">
    <template #trigger>
      <button
        class="dl-round"
        type="button"
        title="Скачать приложение"
        aria-label="Скачать приложение"
      >
        <n-icon :component="DownloadOutline" :size="18" />
      </button>
    </template>
    <DownloadMenu :groups="groups" />
  </n-popover>
</template>

<style scoped>
/* Split button — neutral, fully-rounded ends to echo the circular tool buttons. */
.dl-split {
  display: inline-flex;
  align-items: stretch;
  height: 40px;
  border: 1px solid var(--t-border);
  border-radius: 20px;
  background: var(--t-surface);
  overflow: hidden;
}
.dl-split-main {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px 0 15px;
  color: var(--t-text1);
  text-decoration: none;
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.dl-split-main:hover {
  background: var(--t-hover, rgba(124, 108, 255, 0.1));
  color: var(--t-primary);
}
.dl-split-sep {
  width: 1px;
  background: var(--t-border);
  flex: none;
}
.dl-split-caret {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  border: none;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.dl-split-caret:hover {
  background: var(--t-hover, rgba(124, 108, 255, 0.1));
  color: var(--t-primary);
}

/* Round icon button — same footprint/style as the theme & server tool buttons
   (their `.auth-tool-btn` is scoped to AuthLayout, so it's restated here). */
.dl-round {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  cursor: pointer;
  transition:
    background 0.15s ease,
    color 0.15s ease,
    border-color 0.15s ease;
}
.dl-round:hover {
  color: var(--t-primary);
  border-color: var(--t-primary);
}
</style>
