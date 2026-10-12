<script setup>
// The documents' «Корзина» (#3006, migration 0086, mockup DocsPage — the row at
// the foot of the tree): documents deleted in the last 30 days, newest first,
// each with «Вернуть» and a two-step «Удалить навсегда». A document deleted with
// its nested ones is one row — they come back together. Links to tasks are kept
// while a document lies here and come back with it.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  ArrowUndoOutline,
  ChevronBackOutline,
  DocumentTextOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import ArmButton from '../kit/ArmButton.vue'
import EmptyState from '../EmptyState.vue'
import { documents as docsApi } from '@/api'
import { useFormat } from '@/composables/useFormat'
import { useMessage } from '@/composables/useMessage'
import { humanizeError } from '@/utils/errors'
import { TRASH_DAYS, daysLeft } from '@/utils/taskTrash'

const props = defineProps({
  wsId: { type: String, default: '' },
  // Bumped by the view on a document.deleted / .restored / .purged event.
  reloadToken: { type: Number, default: 0 },
})
const emit = defineEmits(['restored', 'close'])
const { t } = useI18n()
const { formatRelative } = useFormat()
const message = useMessage()

const rows = ref([])
const loading = ref(false)
const busy = ref('')

async function load() {
  if (!props.wsId) return
  const ws = props.wsId
  loading.value = true
  try {
    const { data } = await docsApi.trash(ws)
    if (ws === props.wsId) rows.value = data || []
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    loading.value = false
  }
}
watch(() => [props.wsId, props.reloadToken], load, { immediate: true })

const sub = computed(() =>
  rows.value.length
    ? t('documents.trash.count', { n: rows.value.length }, rows.value.length)
    : t('documents.trash.keep', { days: TRASH_DAYS }),
)

function meta(r) {
  const parts = []
  if (r.deleted_by_name) parts.push(t('documents.trash.by', { name: r.deleted_by_name }))
  parts.push(formatRelative(r.deleted_at))
  const left = daysLeft(r.deleted_at)
  parts.push(t('documents.trash.left', { n: left }, left))
  return parts.join(' · ')
}

async function restore(r) {
  if (busy.value) return
  busy.value = r.id
  try {
    await docsApi.untrash(r.id)
    rows.value = rows.value.filter((x) => x.id !== r.id)
    message.success(
      t('documents.trash.restored', { title: r.title || t('documents.view.untitled') }),
    )
    emit('restored', r)
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = ''
  }
}

async function purge(r) {
  if (busy.value) return
  busy.value = r.id
  try {
    await docsApi.purge(r.id)
    rows.value = rows.value.filter((x) => x.id !== r.id)
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <section class="dtrash" data-testid="doc-trash">
    <button type="button" class="dtrash-back" data-testid="doc-trash-back" @click="emit('close')">
      <n-icon :component="ChevronBackOutline" :size="14" />
      {{ t('documents.view.crumbRoot') }}
    </button>
    <div class="dtrash-head">
      <n-icon :component="TrashOutline" :size="17" class="dtrash-ic" />
      <h2 class="dtrash-title">{{ t('documents.tree.trash') }}</h2>
      <span class="dtrash-sub">{{ sub }}</span>
    </div>
    <EmptyState
      v-if="!loading && !rows.length"
      :icon="TrashOutline"
      :title="t('documents.trash.emptyTitle')"
      :text="t('documents.trash.emptyText', { days: TRASH_DAYS })"
    />
    <ul v-else class="dtrash-list">
      <li v-for="r in rows" :key="r.id" class="dtrash-row" data-testid="doc-trash-row">
        <span v-if="r.icon" class="dtrash-emoji">{{ r.icon }}</span>
        <n-icon v-else :component="DocumentTextOutline" :size="16" class="dtrash-glyph" />
        <div class="dtrash-main">
          <div class="dtrash-name">{{ r.title || t('documents.view.untitled') }}</div>
          <div class="dtrash-meta">
            {{ meta(r) }}
            <template v-if="r.child_count">
              · {{ t('documents.view.nestedCount', { count: r.child_count }) }}
            </template>
          </div>
        </div>
        <button
          type="button"
          class="kit-btn dtrash-restore"
          :disabled="busy === r.id"
          data-testid="doc-trash-restore"
          @click="restore(r)"
        >
          <n-icon :component="ArrowUndoOutline" :size="14" />
          {{ t('documents.trash.restore') }}
        </button>
        <ArmButton
          :title="t('documents.trash.purge')"
          :confirm="t('documents.trash.purgeArmed')"
          testid="doc-trash-purge"
          @confirm="purge(r)"
        />
      </li>
    </ul>
  </section>
</template>

<style scoped>
.dtrash {
  max-width: 760px;
  padding: 8px 4px 24px;
}
.dtrash-back {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 8px;
  padding: 0 6px;
  height: 26px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.dtrash-back:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dtrash-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 14px;
}
.dtrash-ic {
  align-self: center;
  color: var(--t-text3);
}
.dtrash-title {
  margin: 0;
  font-size: 18px;
  font-weight: 650;
  color: var(--t-text1);
}
.dtrash-sub {
  font-size: 12.5px;
  color: var(--t-text3);
}
.dtrash-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.dtrash-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 10px;
}
.dtrash-row:hover {
  background: var(--t-hover);
}
.dtrash-glyph {
  flex: none;
  color: var(--t-text3);
}
.dtrash-emoji {
  flex: none;
  width: 16px;
  text-align: center;
}
.dtrash-main {
  flex: 1;
  min-width: 0;
}
.dtrash-name {
  font-size: 14px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dtrash-meta {
  margin-top: 2px;
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dtrash-restore {
  flex: none;
  height: 30px;
  padding: 0 12px;
  font-size: 13px;
  gap: 5px;
}
</style>
