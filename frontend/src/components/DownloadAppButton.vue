<script setup>
// App-download control on the login screen (web only). Two shapes:
//
//  - The visitor's platform HAS a build → a full split button
//    «⤓ Download for <OS> | ▾»: the labelled part downloads that platform's
//    recommended build directly (AppImage on Linux), the caret opens the full
//    per-platform menu.
//  - No build for this platform (mac / iOS / unknown) → a round icon button that
//    just opens the menu; no caret is drawn.
//
// Styled as the bar's other quiet controls (v2, #3107). Links are version-correct URLs
// pulled from the published manifests at runtime (see composables/useDownloads.js
// and DownloadMenu.vue).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()
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
      :title="t('app.download.forPlatform', { platform: detectedGroup.name })"
    >
      <n-icon :component="detectedGroup.icon" :size="15" />
      <span class="dl-split-label">{{
        t('app.download.forPlatform', { platform: detectedGroup.name })
      }}</span>
    </a>
    <span class="dl-split-sep" aria-hidden="true" />
    <n-popover trigger="click" placement="bottom-end">
      <template #trigger>
        <button class="dl-split-caret" type="button" :aria-label="t('app.download.otherPlatforms')">
          <n-icon :component="ChevronDownOutline" :size="12" />
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
        :title="t('app.download.title')"
        :aria-label="t('app.download.title')"
      >
        <n-icon :component="DownloadOutline" :size="15" />
      </button>
    </template>
    <DownloadMenu :groups="groups" />
  </n-popover>
</template>

<style scoped>
/* v2 (#3107): a quiet control in the auth screen's bottom bar and in Settings —
   the --t-ctl level, the 8px radius, no outline. */
.dl-split {
  display: inline-flex;
  align-items: stretch;
  height: 28px;
  border-radius: 8px;
  background: var(--t-ctl);
  overflow: hidden;
}
.dl-split-main {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 10px;
  color: var(--t-text2);
  text-decoration: none;
  font-size: 12.5px;
  font-weight: 500;
  white-space: nowrap;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.dl-split-main:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dl-split-caret {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  border: none;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.dl-split-caret:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* The separator is spacing now, not a line. */
.dl-split-sep {
  width: 2px;
  flex: none;
}
.dl-round {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text2);
  cursor: pointer;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}
.dl-round:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dl-split-main:focus-visible,
.dl-split-caret:focus-visible,
.dl-round:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
</style>
