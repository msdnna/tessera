<script setup>
// «Справка» in the administration (#3071): what readers answered to «Статья
// помогла?» — per article and language, most-voted first. Each row is a --t-col
// surface (no lines): the article, its language, the share of «да» as a green
// bar, the two counts. The votes are per-user acknowledgements; the server
// only ever hands out the totals.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSpin } from 'naive-ui'
import { ThumbsUpOutline, ThumbsDownOutline, HelpCircleOutline } from '@vicons/ionicons5'
import { admin } from '@/api'
import { useHelpStore } from '@/stores/help'
import EmptyState from '@/components/EmptyState.vue'

const { t } = useI18n()
const help = useHelpStore()
const rows = ref([])
const loading = ref(false)
const failed = ref('')

const view = computed(() =>
  rows.value.map((r) => {
    const total = r.yes + r.no
    return {
      ...r,
      title: help.bySlug(r.slug)?.title || r.slug,
      share: total ? Math.round((r.yes / total) * 100) : 0,
    }
  }),
)

async function load() {
  loading.value = true
  failed.value = ''
  try {
    const { data } = await admin.helpFeedback()
    rows.value = data || []
  } catch (e) {
    failed.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <n-spin :show="loading" class="hfa" data-testid="admin-help-feedback">
    <p v-if="failed" class="hfa-err">{{ failed }}</p>
    <EmptyState
      v-else-if="!loading && !view.length"
      :icon="HelpCircleOutline"
      :title="t('settings.admin.help.emptyTitle')"
      :text="t('settings.admin.help.empty')"
    />
    <div v-else class="hfa-rows">
      <div
        v-for="r in view"
        :key="`${r.slug}:${r.lang}`"
        class="hfa-row"
        data-testid="admin-help-row"
      >
        <span class="hfa-title">{{ r.title }}</span>
        <span class="hfa-lang">{{ r.lang.toUpperCase() }}</span>
        <span class="hfa-bar" :title="t('settings.admin.help.share', { n: r.share })">
          <span class="hfa-fill" :style="{ width: `${r.share}%` }" />
        </span>
        <span class="hfa-n yes"><n-icon :component="ThumbsUpOutline" :size="13" />{{ r.yes }}</span>
        <span class="hfa-n no"><n-icon :component="ThumbsDownOutline" :size="13" />{{ r.no }}</span>
      </div>
    </div>
  </n-spin>
</template>

<style scoped>
.hfa-rows {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.hfa-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto 140px 52px 52px;
  align-items: center;
  gap: 14px;
  min-height: 48px;
  padding: 0 14px;
  border-radius: 12px;
  background: var(--t-col);
}
.hfa-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
  font-weight: 600;
  color: var(--t-text1);
}
.hfa-lang {
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 11px;
  font-weight: 700;
}
.hfa-bar {
  height: 6px;
  border-radius: 3px;
  background: color-mix(in srgb, #e0533d 30%, var(--t-ctl));
  overflow: hidden;
}
.hfa-fill {
  display: block;
  height: 100%;
  border-radius: 3px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 86%, #000),
    #18a058 50%,
    color-mix(in srgb, #18a058 86%, #fff)
  );
}
.hfa-n {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  color: var(--t-text2);
}
.hfa-n.yes .n-icon {
  color: #18a058;
}
.hfa-n.no .n-icon {
  color: #e0533d;
}
.hfa-err {
  color: #e0533d;
  font-size: 13px;
}
@media (max-width: 760px) {
  .hfa-row {
    grid-template-columns: minmax(0, 1fr) auto 44px 44px;
  }
  .hfa-bar {
    display: none;
  }
}
</style>
