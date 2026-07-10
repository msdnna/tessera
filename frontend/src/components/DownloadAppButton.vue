<script setup>
// A circular tool button (login screen, web only) that reveals a dropdown of app
// downloads. The trigger shows the visitor's detected-platform logo plus a small
// down-chevron (so it reads as a dropdown before it's opened) inside the same
// round shape as the theme/server tool buttons. Its own styles live here because
// AuthLayout's `.auth-tool-btn` is scoped and wouldn't reach this child.
//
// The popover lists every available build with valid, version-correct URLs pulled
// from the published manifests at runtime (see composables/useDownloads.js). The
// detected platform is listed first. Single-variant platforms (Windows / Android)
// collapse to one clickable row; multi-variant Linux shows a label + its formats,
// with the recommended one (AppImage) badged.
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
        class="dl-trigger"
        type="button"
        title="Скачать приложение"
        aria-label="Скачать приложение"
      >
        <n-icon :component="triggerIcon" :size="18" />
        <n-icon :component="ChevronDownOutline" :size="11" class="dl-trigger-caret" />
      </button>
    </template>
    <div class="dl-pop">
      <div class="dl-title">Скачать приложение</div>
      <template v-for="g in groups" :key="g.key">
        <!-- Single build (Windows / Android): the whole platform row is the link. -->
        <a
          v-if="g.single"
          class="dl-platform dl-platform--link"
          :href="g.variants[0].url"
          download
          rel="noopener"
        >
          <n-icon :component="g.icon" :size="18" class="dl-platform-icon" />
          <span class="dl-platform-name">{{ g.name }}</span>
          <span v-if="g.version" class="dl-ver">v{{ g.version }}</span>
        </a>
        <!-- Multiple builds (Linux): platform label + one row per format. -->
        <div v-else class="dl-group">
          <div class="dl-platform">
            <n-icon :component="g.icon" :size="18" class="dl-platform-icon" />
            <span class="dl-platform-name">{{ g.name }}</span>
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
      </template>
    </div>
  </n-popover>
</template>

<style scoped>
/* Round tool button matching AuthLayout's `.auth-tool-btn` (scoped there, so
   restated here), with a compact down-chevron beside the platform glyph so it
   reads as a dropdown without breaking the circular shape. */
.dl-trigger {
  position: relative;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1px;
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
.dl-trigger:hover {
  color: var(--t-primary);
  border-color: var(--t-primary);
}
.dl-trigger-caret {
  color: var(--t-text3, var(--t-text2));
  transition: color 0.15s ease;
}
.dl-trigger:hover .dl-trigger-caret {
  color: var(--t-primary);
}

.dl-pop {
  width: 250px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.dl-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.dl-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

/* Platform row — a label above its format list, or (single build) a link itself. */
.dl-platform {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 8px;
  color: var(--t-text1);
}
.dl-platform-icon {
  color: var(--t-text2);
  flex: none;
}
.dl-platform-name {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
}
.dl-platform--link {
  text-decoration: none;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.dl-platform--link:hover {
  background: var(--t-hover, rgba(124, 108, 255, 0.1));
  color: var(--t-primary);
}
.dl-platform--link:hover .dl-platform-icon {
  color: var(--t-primary);
}
.dl-ver {
  flex: none;
  font-size: 12px;
  font-weight: 400;
  color: var(--t-text3, var(--t-text2));
  font-variant-numeric: tabular-nums;
}

.dl-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px 6px 12px;
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
