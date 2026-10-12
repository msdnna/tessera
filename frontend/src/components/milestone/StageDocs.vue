<script setup>
// «Документы» on the stage page (#3044): the documents pinned to the stage from
// their head's chip — specs, plans, retros of this sprint — each a door to the
// document. Drawn only when there is one: an empty card would be noise on a page
// that is about tasks, and pinning starts from the document, not from here.
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { DocumentTextOutline } from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import { useFormat } from '@/composables/useFormat'

const props = defineProps({
  milestoneId: { type: String, default: '' },
})
const { t } = useI18n()
const router = useRouter()
const { formatRelative } = useFormat()

const docs = ref([])
let seq = 0
watch(
  () => props.milestoneId,
  async (id) => {
    const mine = ++seq
    docs.value = []
    if (!id) return
    try {
      const { data } = await docsApi.byMilestone(id)
      if (mine === seq) docs.value = data || []
    } catch {
      // The card is extra; the page stands without it.
    }
  },
  { immediate: true },
)

function open(d) {
  router.push(`/documents/${d.slug || d.id}`)
}
</script>

<template>
  <section v-if="docs.length" class="sd-card" data-testid="stage-docs">
    <div class="sd-h">
      <span class="sd-t">{{ t('milestones.page.documents') }}</span>
      <span class="sd-n">{{ docs.length }}</span>
    </div>
    <button
      v-for="d in docs"
      :key="d.id"
      type="button"
      class="sd-row"
      data-testid="stage-doc"
      @click="open(d)"
    >
      <span v-if="d.icon" class="sd-emoji">{{ d.icon }}</span>
      <n-icon v-else :component="DocumentTextOutline" :size="15" class="sd-ic" />
      <span class="sd-title">{{ d.title }}</span>
      <span class="sd-when">{{ formatRelative(d.updated_at) }}</span>
    </button>
  </section>
</template>

<style scoped>
/* The page's card: --t-col, radius 16, like its neighbours. */
.sd-card {
  padding: 16px;
  border-radius: 16px;
  background: var(--t-col);
  min-width: 0;
}
.sd-h {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  min-height: 28px;
}
.sd-t {
  font-size: 14px;
  font-weight: 650;
}
.sd-n {
  font-size: 12.5px;
  color: var(--t-text3);
}
.sd-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 34px;
  padding: 0 8px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.sd-row:hover {
  background: var(--t-hover);
}
.sd-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.sd-ic {
  color: var(--t-primary);
}
.sd-emoji {
  font-size: 14px;
  line-height: 1;
}
.sd-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sd-when {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
</style>
