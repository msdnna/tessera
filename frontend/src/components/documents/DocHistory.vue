<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NInput, NPopconfirm, NText } from 'naive-ui'
import { BookmarkOutline, TimeOutline } from '@vicons/ionicons5'
import { DIFF_ADDED, DIFF_CHANGED, DIFF_MOVED, DIFF_REMOVED } from '@/utils/docDiff'
import { useFormat } from '@/composables/useFormat'

// The version journal (#2731): entries on the left of the reading area, and the
// block-level comparison of the selected entry against the newest one below.
defineProps({
  versions: { type: Array, default: () => [] },
  selectedId: { type: String, default: '' },
  // The version the selection is compared against — the newest one unless the
  // reader picked another.
  baseline: { type: Object, default: null },
  // Result of diffDocs: one ordered list of blocks with a status each.
  rows: { type: Array, default: () => [] },
  summary: { type: Object, default: () => ({}) },
  ready: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['select', 'snapshot', 'restore', 'close'])
const { formatDate, formatTime } = useFormat()
const { t } = useI18n()

const label = ref('')
const naming = ref(false)

function submitSnapshot() {
  emit('snapshot', label.value)
  label.value = ''
  naming.value = false
}

// A journal entry covers an editing session, so it has a span rather than a
// moment. Same-minute ends are not shown — "13:05–13:05" is noise.
function span(v) {
  const from = new Date(v.created_at)
  const to = new Date(v.updated_at)
  // Named `at` rather than `t`: `t` is the translation function in this file
  // now, and shadowing it inside a helper is how a label quietly stops moving
  // with the language.
  const at = (d) => formatTime(d)
  const day = formatDate(from, { day: 'numeric', month: 'short' })
  if (at(from) === at(to)) return `${day}, ${at(from)}`
  return `${day}, ${at(from)}–${at(to)}`
}

function author(v) {
  return v.author_name || v.author_email || t('documents.history.unknownAuthor')
}

// Read per call, not baked into a table at setup: a map built once keeps the
// language of the first render for the rest of the session (#2799).
const DIFF_KEY = {
  [DIFF_ADDED]: 'added',
  [DIFF_REMOVED]: 'removed',
  [DIFF_CHANGED]: 'changed',
  [DIFF_MOVED]: 'moved',
}

function statusLabel(status) {
  const key = DIFF_KEY[status]
  return key ? t(`documents.history.status.${key}`) : status
}
</script>

<template>
  <aside class="doc-history">
    <div class="panel-head">
      <n-icon :component="TimeOutline" :size="16" />
      <span class="panel-title">{{ $t('documents.history.title') }}</span>
      <n-text v-if="loading" depth="3">…</n-text>
      <span class="grow" />
      <n-button quaternary size="tiny" @click="emit('close')">
        {{ $t('common.action.close') }}
      </n-button>
    </div>

    <div class="panel-foot">
      <n-button v-if="!naming" size="tiny" block data-testid="doc-snapshot" @click="naming = true">
        <template #icon><n-icon :component="BookmarkOutline" /></template>
        {{ $t('documents.history.snapshot') }}
      </n-button>
      <template v-else>
        <n-input
          v-model:value="label"
          size="tiny"
          :placeholder="$t('documents.history.labelPlaceholder')"
          data-testid="doc-snapshot-label"
          @keyup.enter="submitSnapshot"
        />
        <div class="row">
          <n-button
            size="tiny"
            type="primary"
            data-testid="doc-snapshot-save"
            @click="submitSnapshot"
          >
            {{ $t('common.action.save') }}
          </n-button>
          <n-button size="tiny" quaternary @click="naming = false">
            {{ $t('common.action.cancel') }}
          </n-button>
        </div>
      </template>
    </div>

    <n-text v-if="error" type="error" class="empty">{{ error }}</n-text>

    <div class="panel-body">
      <p v-if="!versions.length && !loading" class="empty">
        {{ $t('documents.history.empty') }}
      </p>
      <!-- Newest first, as the server returns them: the question a journal
           answers most often is "что изменилось с тех пор". -->
      <button
        v-for="v in versions"
        :key="v.id"
        type="button"
        class="entry"
        :class="{ active: v.id === selectedId, milestone: v.manual }"
        @click="emit('select', v.id)"
      >
        <div class="entry-head">
          <n-icon v-if="v.manual" :component="BookmarkOutline" :size="13" />
          <span class="rev">{{ $t('documents.history.revision', { n: v.revision }) }}</span>
          <span class="time">{{ span(v) }}</span>
        </div>
        <span v-if="v.label" class="label">{{ v.label }}</span>
        <span class="author">{{ author(v) }}</span>
        <p class="preview">{{ v.preview || $t('documents.view.emptyPreview') }}</p>
      </button>
    </div>

    <!-- The comparison. It sits under the list rather than in a modal: picking
         another entry while reading a diff is the normal way to walk a history,
         and a dialog would make that a close-and-reopen each time. -->
    <div v-if="selectedId" class="diff">
      <div class="diff-head">
        <n-text depth="3" class="section-title">
          {{ $t('documents.history.compare', { revision: baseline?.revision ?? '—' }) }}
        </n-text>
        <n-popconfirm
          :positive-button-props="{ 'data-testid': 'doc-restore-confirm' }"
          @positive-click="emit('restore', selectedId)"
        >
          <template #trigger>
            <n-button size="tiny" type="primary" ghost data-testid="doc-restore">
              {{ $t('documents.history.restore') }}
            </n-button>
          </template>
          {{ $t('documents.history.restoreConfirm') }}
        </n-popconfirm>
      </div>

      <n-text v-if="!ready" depth="3" class="empty">
        {{ $t('documents.history.loadingVersion') }}
      </n-text>
      <n-text v-else-if="summary.identical" depth="3" class="empty">
        {{ $t('documents.history.identical') }}
      </n-text>
      <template v-else>
        <n-text depth="3" class="counts">
          <span v-if="summary.added">
            {{ $t('documents.history.added', summary.added, { count: summary.added }) }}
          </span>
          <span v-if="summary.removed">
            {{ $t('documents.history.removed', summary.removed, { count: summary.removed }) }}
          </span>
          <span v-if="summary.changed">
            {{ $t('documents.history.changed', { count: summary.changed }) }}
          </span>
          <span v-if="summary.moved">
            {{ $t('documents.history.moved', { count: summary.moved }) }}
          </span>
        </n-text>
        <div class="diff-body">
          <div v-for="(row, i) in rows" :key="i" class="block" :class="row.status">
            <span v-if="row.status !== 'same'" class="badge">{{ statusLabel(row.status) }}</span>
            <!-- The previous wording of an edited block is kept next to the new
                 one: "изменено" without the old text asks the reader to
                 remember what they came here to look up. -->
            <p v-if="row.prevText" class="was">{{ row.prevText }}</p>
            <p class="text">{{ row.text || '—' }}</p>
          </div>
        </div>
      </template>
    </div>
  </aside>
</template>

<style scoped>
/* Since task 2738 this is not a column in the working row but the content of the
   sidebar, so it fills whatever it is given: the width, the border and the
   overlay behaviour all belong to `.side` in DocumentsView, and repeating them
   here would mean two places to change and a 300px panel inside a 320px box. */
.doc-history {
  display: flex;
  flex-direction: column;
  width: 100%;
  flex: 1;
  min-height: 0;
  gap: 8px;
}
.panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--t-text2);
}
.panel-title {
  font-size: 13px;
  font-weight: 600;
}
.grow {
  flex: 1;
}
.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.panel-foot {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: none;
}
.row {
  display: flex;
  gap: 6px;
}
/* Entries are neutral; only the one being read takes the accent, as in the
   discussion panel. */
.entry {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  cursor: pointer;
  text-align: left;
  font: inherit;
  color: inherit;
}
.entry.active {
  border-color: transparent;
  background:
    linear-gradient(var(--t-surface), var(--t-surface)) padding-box,
    var(--t-accent-grad) border-box;
  border: 1px solid transparent;
}
/* A milestone someone asked to keep reads differently from an autosaved
   session — it is the entry people scroll the journal to find. */
.entry.milestone .rev {
  color: var(--t-primary);
}
.entry-head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
}
.rev {
  font-weight: 600;
  color: var(--t-text2);
}
.time,
.author {
  color: var(--t-text3);
  font-size: 11px;
}
.label {
  font-size: 12px;
  color: var(--t-text1);
}
.preview {
  margin: 2px 0 0;
  font-size: 11px;
  line-height: 1.4;
  color: var(--t-text3);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
}
.diff {
  flex: none;
  max-height: 45%;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--t-border);
}
.diff-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.section-title {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.counts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 11px;
}
.diff-body {
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
/* Status is carried by a left border and a word, not by colour alone: the two
   colours that read as "added" and "removed" are also the two most common forms
   of colour blindness. */
.block {
  padding: 4px 6px;
  border-left: 3px solid transparent;
  font-size: 12px;
  line-height: 1.45;
}
.block.same {
  color: var(--t-text3);
}
.block.added {
  border-left-color: var(--t-success, #18a058);
}
.block.removed {
  border-left-color: var(--t-error, #d03050);
}
.block.changed {
  border-left-color: var(--t-warning, #f0a020);
}
.block.moved {
  border-left-color: var(--t-primary);
}
.badge {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--t-text3);
}
.text {
  margin: 2px 0 0;
  color: var(--t-text1);
  word-break: break-word;
}
.block.removed .text {
  text-decoration: line-through;
  color: var(--t-text3);
}
.was {
  margin: 2px 0 0;
  color: var(--t-text3);
  text-decoration: line-through;
  word-break: break-word;
}
.empty {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--t-text3);
}
</style>
