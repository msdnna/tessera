<script setup>
// The dropdown body shared by both trigger shapes of DownloadAppButton (the
// labelled split button and the round icon button). Lists every published build
// grouped by platform; single-variant platforms collapse to one clickable row,
// multi-variant Linux shows a label + one row per format with the recommended
// one (AppImage) badged. URLs come straight from the manifests (see useDownloads).
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { DownloadOutline } from '@vicons/ionicons5'

defineProps({
  groups: { type: Array, required: true },
})

const { t, te } = useI18n()

// Format names are resolved here rather than carried on the variant, so they
// follow a language switch. An unknown format from a newer manifest falls back
// to its own id instead of printing a missing key.
function formatLabel(format) {
  const key = `app.download.format.${format}`
  return te(key) ? t(key) : format
}
</script>

<template>
  <div class="dl-pop">
    <div class="dl-title">{{ t('app.download.title') }}</div>
    <template v-for="g in groups" :key="g.key">
      <!-- Single build (Windows / Android): the whole platform row is the link. -->
      <a
        v-if="g.single"
        class="dl-platform dl-platform--link"
        :href="g.variants[0].url"
        download
        rel="noopener noreferrer"
      >
        <n-icon :component="g.icon" :size="18" class="dl-platform-icon" />
        <span class="dl-platform-name">{{ g.name }}</span>
        <span v-if="g.version" class="dl-ver">{{
          t('app.download.versionTag', { version: g.version })
        }}</span>
      </a>
      <!-- Multiple builds (Linux): platform label + one row per format. -->
      <div v-else class="dl-group">
        <div class="dl-platform">
          <n-icon :component="g.icon" :size="18" class="dl-platform-icon" />
          <span class="dl-platform-name">{{ g.name }}</span>
          <span v-if="g.version" class="dl-ver">{{
            t('app.download.versionTag', { version: g.version })
          }}</span>
        </div>
        <a
          v-for="v in g.variants"
          :key="v.format"
          class="dl-item"
          :href="v.url"
          download
          rel="noopener noreferrer"
        >
          <n-icon :component="DownloadOutline" :size="15" class="dl-item-icon" />
          <span class="dl-item-label">{{ formatLabel(v.format) }}</span>
          <span v-if="v.recommended" class="dl-badge">{{ t('app.download.recommended') }}</span>
        </a>
      </div>
    </template>
  </div>
</template>

<style scoped>
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
