<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NButton, NIcon } from 'naive-ui'
import { SparklesOutline } from '@vicons/ionicons5'
import { renderMarkdown } from '@/utils/markdown'
import { useWhatsNewStore } from '@/stores/whatsNew'

// "What's New" after an update (#2749): a modal listing the curated highlights of
// every release the user has just updated into. Self-contained — it reads the
// store's `pending` list and clears it (marking each version seen) on dismiss.
const { t } = useI18n()
const store = useWhatsNewStore()

const show = computed({
  get: () => store.pending.length > 0,
  set: (v) => {
    if (!v) store.dismissModal()
  },
})

function itemsHtml(itemKeys) {
  // Render the bullets as one Markdown list (sanitized by renderMarkdown). The
  // entries carry catalogue keys, not text (#2800), so the notes follow the
  // interface language.
  return renderMarkdown(itemKeys.map((key) => `- ${t(key)}`).join('\n'))
}

function close() {
  store.dismissModal()
}
</script>

<template>
  <n-modal v-model:show="show">
    <n-card class="wn-card" :bordered="false" role="dialog" :aria-label="t('app.whatsNew.title')">
      <div class="wn-head">
        <div class="wn-badge"><n-icon :component="SparklesOutline" :size="20" /></div>
        <div>
          <div class="wn-title">{{ t('app.whatsNew.title') }}</div>
          <div class="wn-sub">{{ t('app.whatsNew.sub') }}</div>
        </div>
      </div>

      <div class="wn-body">
        <section v-for="rel in store.pending" :key="rel.version" class="wn-rel">
          <header class="wn-rel-head">
            <span class="wn-rel-title">{{ t(rel.titleKey) }}</span>
            <span class="wn-rel-ver">{{ rel.version }}</span>
          </header>
          <!-- eslint-disable-next-line vue/no-v-html -- sanitized by renderMarkdown -->
          <div class="wn-md" v-html="itemsHtml(rel.itemKeys)" />
        </section>
      </div>

      <div class="wn-foot">
        <n-button type="primary" @click="close">{{ t('app.whatsNew.gotIt') }}</n-button>
      </div>
    </n-card>
  </n-modal>
</template>

<style scoped>
.wn-card {
  width: 520px;
  max-width: calc(100vw - 32px);
  border-radius: 14px;
  background: var(--t-surface);
  /* Glowing accent card, matching the picked spotlight popover style. */
  border: 1px solid color-mix(in srgb, var(--t-primary) 45%, var(--t-border));
  box-shadow:
    0 18px 50px rgba(124, 92, 255, 0.26),
    0 0 0 1px color-mix(in srgb, var(--t-primary) 26%, transparent);
}
.wn-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}
.wn-badge {
  flex: none;
  width: 40px;
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--t-accent-grad, var(--t-primary));
  color: var(--t-on-primary, #fff);
}
.wn-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--t-text1);
}
.wn-sub {
  font-size: 12px;
  color: var(--t-text3);
}
.wn-body {
  max-height: 52vh;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.wn-rel-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 4px;
}
.wn-rel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--t-text1);
}
.wn-rel-ver {
  font-size: 11px;
  font-weight: 600;
  color: var(--t-text3);
  padding: 1px 7px;
  border-radius: 999px;
  border: 1px solid var(--t-border);
}
.wn-md {
  font-size: 13px;
  color: var(--t-text2);
  line-height: 1.5;
}
.wn-md :deep(ul) {
  margin: 0;
  padding-left: 18px;
}
.wn-md :deep(li) {
  margin: 3px 0;
}
.wn-foot {
  margin-top: 18px;
  display: flex;
  justify-content: flex-end;
}
</style>
