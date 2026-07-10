<script setup>
// A single circular tool button (login screen, web only) that reveals a dropdown
// of app downloads. The trigger icon reflects the visitor's detected platform
// (Windows / Linux / Android), and the popover lists every available build with
// valid, version-correct URLs pulled from the published manifests at runtime
// (see composables/useDownloads.js). The detected platform is listed first, and
// its recommended variant (AppImage on Linux) is badged.
import { computed } from 'vue'
import { NIcon, NPopover } from 'naive-ui'
import {
  LogoWindows,
  LogoTux,
  LogoAndroid,
  DownloadOutline,
} from '@vicons/ionicons5'
import { useDownloads } from '@/composables/useDownloads'

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
        variants: data.variants.map((v) => ({
          ...v,
          recommended: multi && v.format === meta.recommend,
        })),
      }
    })
})

// Trigger icon: the detected platform's logo when we have a build for it,
// otherwise a neutral download glyph (mac / unknown, or manifests still loading).
const triggerIcon = computed(() => {
  const meta = META[detected.value]
  if (meta && DATA[detected.value]?.value) return meta.icon
  return DownloadOutline
})
</script>

<template>
  <n-popover v-if="groups.length" trigger="click" placement="bottom-end">
    <template #trigger>
      <button
        class="auth-tool-btn"
        type="button"
        title="Скачать приложение"
        aria-label="Скачать приложение"
      >
        <n-icon :component="triggerIcon" :size="20" />
      </button>
    </template>
    <div class="dl-pop">
      <div class="dl-title">Скачать приложение</div>
      <div v-for="g in groups" :key="g.key" class="dl-group">
        <div class="dl-group-head">
          <n-icon :component="g.icon" :size="16" />
          <span class="dl-group-name">{{ g.name }}</span>
          <span v-if="g.version" class="dl-ver">v{{ g.version }}</span>
        </div>
        <a
          v-for="v in g.variants"
          :key="v.format"
          class="dl-item"
          :href="v.url"
          download
          rel="noopener"
        >
          <n-icon :component="DownloadOutline" :size="15" class="dl-item-icon" />
          <span class="dl-item-label">{{ v.label }}</span>
          <span v-if="v.recommended" class="dl-badge">рекоменд.</span>
        </a>
      </div>
    </div>
  </n-popover>
</template>

<style scoped>
.dl-pop {
  width: 250px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.dl-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.dl-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.dl-group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--t-text2);
  font-size: 12px;
  font-weight: 600;
}
.dl-group-name {
  flex: 1;
}
.dl-ver {
  font-weight: 400;
  color: var(--t-text3, var(--t-text2));
  font-variant-numeric: tabular-nums;
}
.dl-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 8px;
  color: var(--t-text1);
  text-decoration: none;
  font-size: 13px;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.dl-item:hover {
  background: var(--t-hover, rgba(124, 108, 255, 0.1));
  color: var(--t-primary);
}
.dl-item-icon {
  color: var(--t-text3, var(--t-text2));
  flex: none;
}
.dl-item:hover .dl-item-icon {
  color: var(--t-primary);
}
.dl-item-label {
  flex: 1;
}
.dl-badge {
  flex: none;
  font-size: 11px;
  font-weight: 600;
  padding: 1px 7px;
  border-radius: 999px;
  color: #fff;
  background: var(--t-accent-grad-subtle, var(--t-primary));
}
</style>
