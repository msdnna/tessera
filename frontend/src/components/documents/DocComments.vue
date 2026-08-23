<script setup>
import { ref } from 'vue'
import { NButton, NIcon, NInput, NPopconfirm, NText } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  ChatbubbleEllipsesOutline,
  CreateOutline,
  RefreshOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { authorLabel } from '@/utils/docComments'
import { useFormat } from '@/composables/useFormat'

// The annotation panel: threads anchored to blocks, plus the ones about the
// document as a whole and the ones whose block has been deleted since.
const props = defineProps({
  // { anchored, document, detached } — see splitThreads.
  groups: { type: Object, default: () => ({ anchored: [], document: [], detached: [] }) },
  // Highlighted thread, driven by clicking a discussed block in the editor.
  activeBlockId: { type: String, default: '' },
  // Who "me" is, so only one's own comments offer edit/delete. Comes from the
  // document socket's welcome frame, like the presence roster.
  userId: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  // The text of the block the draft below is armed against, empty when the next
  // comment is about the document as a whole.
  pendingQuote: { type: String, default: '' },
  pendingBlock: { type: Boolean, default: false },
})
const emit = defineEmits(['add', 'reply', 'edit', 'resolve', 'remove', 'select', 'clear-anchor'])
const { formatDateTime } = useFormat()

// One draft per thread, keyed by root id; '' is the document-level box.
const drafts = ref({})
const editing = ref({})

function draft(key) {
  return drafts.value[key] || ''
}
function setDraft(key, v) {
  drafts.value = { ...drafts.value, [key]: v }
}

function submitRoot() {
  const body = draft('')
  if (!body.trim()) return
  emit('add', body)
  setDraft('', '')
}

function submitReply(thread) {
  const body = draft(thread.id)
  if (!body.trim()) return
  emit('reply', { id: thread.id, body })
  setDraft(thread.id, '')
}

function startEdit(comment) {
  editing.value = { ...editing.value, [comment.id]: comment.body }
}
function cancelEdit(id) {
  const next = { ...editing.value }
  delete next[id]
  editing.value = next
}
function submitEdit(id) {
  const body = editing.value[id]
  if (!body?.trim()) return
  emit('edit', { id, body })
  cancelEdit(id)
}

function mine(comment) {
  return !!props.userId && comment.author_id === props.userId
}

function fmtTime(v) {
  if (!v) return ''
  return formatDateTime(v, { day: 'numeric', month: 'short' })
}

/* ---- anchors for the annotation links (#2730) ---------------------------- */

const bodyEl = ref(null)

/**
 * Where each thread card sits, for the layer that draws the line to its block.
 *
 * Measured here rather than by the parent: the markup is this component's, and a
 * selector reaching in from outside would break on any change to it. Viewport
 * coordinates — the parent knows what they are relative to, this panel does not.
 *
 * A card scrolled out of the panel is reported `visible: false` instead of being
 * left out, so the caller can tell "no such thread" from "not on screen".
 *
 * @returns {Array<{id: string, blockId: string, resolved: boolean, x: number, y: number,
 *   visible: boolean}>}
 */
function cardAnchors() {
  const host = bodyEl.value
  if (!host) return []
  const clip = host.getBoundingClientRect()
  return [...host.querySelectorAll('[data-thread-id]')].map((el) => {
    const r = el.getBoundingClientRect()
    const y = (r.top + r.bottom) / 2
    return {
      id: el.getAttribute('data-thread-id') || '',
      blockId: el.getAttribute('data-block-id') || '',
      resolved: !!el.getAttribute('data-resolved'),
      x: r.left,
      y,
      visible: y >= clip.top && y <= clip.bottom,
    }
  })
}

defineExpose({ cardAnchors })
</script>

<template>
  <aside class="doc-comments">
    <div class="panel-head">
      <n-icon :component="ChatbubbleEllipsesOutline" :size="16" />
      <span class="panel-title">Обсуждение</span>
      <n-text v-if="loading" depth="3">…</n-text>
    </div>

    <div ref="bodyEl" class="panel-body">
      <!-- Anchored threads first: they are the ones the text is marked up for. -->
      <template v-for="section in ['anchored', 'document', 'detached']" :key="section">
        <div v-if="groups[section] && groups[section].length" class="section">
          <n-text v-if="section === 'document'" depth="3" class="section-title">
            К документу
          </n-text>
          <!-- Detached threads are kept deliberately: a paragraph being rewritten
               is the normal course of a review, and deleting the discussion that
               asked for the rewrite is not what anyone requested. -->
          <n-text v-else-if="section === 'detached'" depth="3" class="section-title">
            Блок удалён
          </n-text>

          <div
            v-for="t in groups[section]"
            :key="t.id"
            class="thread"
            :data-thread-id="t.id"
            :data-block-id="t.block_id || ''"
            :data-resolved="t.resolved_at ? '1' : ''"
            :class="{ active: t.block_id && t.block_id === activeBlockId, done: !!t.resolved_at }"
            @click="t.block_id && emit('select', t.block_id)"
          >
            <p v-if="t.quote" class="quote">{{ t.quote }}</p>

            <div class="comment">
              <div class="meta">
                <span class="author">{{ authorLabel(t) }}</span>
                <span class="time">{{ fmtTime(t.created_at) }}</span>
              </div>
              <n-input
                v-if="editing[t.id] !== undefined"
                :value="editing[t.id]"
                type="textarea"
                size="small"
                :autosize="{ minRows: 2 }"
                @update:value="editing = { ...editing, [t.id]: $event }"
              />
              <p v-else class="body">{{ t.body }}</p>
              <div v-if="editing[t.id] !== undefined" class="row">
                <n-button size="tiny" type="primary" @click="submitEdit(t.id)">Сохранить</n-button>
                <n-button size="tiny" quaternary @click="cancelEdit(t.id)">Отмена</n-button>
              </div>
            </div>

            <div v-for="r in t.replies" :key="r.id" class="comment reply">
              <div class="meta">
                <span class="author">{{ authorLabel(r) }}</span>
                <span class="time">{{ fmtTime(r.created_at) }}</span>
              </div>
              <n-input
                v-if="editing[r.id] !== undefined"
                :value="editing[r.id]"
                type="textarea"
                size="small"
                :autosize="{ minRows: 2 }"
                @update:value="editing = { ...editing, [r.id]: $event }"
              />
              <p v-else class="body">{{ r.body }}</p>
              <div class="row">
                <template v-if="editing[r.id] !== undefined">
                  <n-button size="tiny" type="primary" @click="submitEdit(r.id)">
                    Сохранить
                  </n-button>
                  <n-button size="tiny" quaternary @click="cancelEdit(r.id)">Отмена</n-button>
                </template>
                <template v-else-if="mine(r)">
                  <n-button size="tiny" quaternary title="Изменить" @click="startEdit(r)">
                    <template #icon><n-icon :component="CreateOutline" /></template>
                  </n-button>
                  <n-popconfirm
                    :positive-button-props="{ type: 'error' }"
                    positive-text="Удалить"
                    @positive-click="emit('remove', r.id)"
                  >
                    <template #trigger>
                      <n-button size="tiny" quaternary title="Удалить">
                        <template #icon><n-icon :component="TrashOutline" /></template>
                      </n-button>
                    </template>
                    Удалить комментарий?
                  </n-popconfirm>
                </template>
              </div>
            </div>

            <div class="row thread-actions">
              <!-- Resolving is open to every member, not just the author: closing a
                   handled remark is the point of a review, and waiting for its
                   author to come back is how threads pile up forever. -->
              <n-button
                v-if="!t.resolved_at"
                size="tiny"
                quaternary
                title="Пометить решённым"
                @click="emit('resolve', { id: t.id, resolved: true })"
              >
                <template #icon><n-icon :component="CheckmarkCircleOutline" /></template>
                Решено
              </n-button>
              <n-button
                v-else
                size="tiny"
                quaternary
                title="Вернуть в работу"
                @click="emit('resolve', { id: t.id, resolved: false })"
              >
                <template #icon><n-icon :component="RefreshOutline" /></template>
                Вернуть
              </n-button>
              <span class="grow" />
              <template v-if="mine(t) && editing[t.id] === undefined">
                <n-button size="tiny" quaternary title="Изменить" @click="startEdit(t)">
                  <template #icon><n-icon :component="CreateOutline" /></template>
                </n-button>
                <n-popconfirm
                  :positive-button-props="{ type: 'error' }"
                  positive-text="Удалить"
                  @positive-click="emit('remove', t.id)"
                >
                  <template #trigger>
                    <n-button size="tiny" quaternary title="Удалить тред">
                      <template #icon><n-icon :component="TrashOutline" /></template>
                    </n-button>
                  </template>
                  <template v-if="t.replies.length">
                    Удалить тред вместе с ответами ({{ t.replies.length }})?
                  </template>
                  <template v-else>Удалить комментарий?</template>
                </n-popconfirm>
              </template>
            </div>

            <div v-if="!t.resolved_at" class="row">
              <n-input
                :value="draft(t.id)"
                size="small"
                placeholder="Ответить…"
                @update:value="setDraft(t.id, $event)"
                @keyup.enter="submitReply(t)"
              />
            </div>
          </div>
        </div>
      </template>

      <p
        v-if="!groups.anchored?.length && !groups.document?.length && !groups.detached?.length"
        class="empty"
      >
        Комментариев пока нет. Выделите блок и нажмите на значок в поле слева, чтобы обсудить его.
      </p>
    </div>

    <div class="panel-foot">
      <!-- What the draft is armed against. Without it "Отправить" is a coin
           toss between annotating a block and commenting on the document. -->
      <div v-if="pendingBlock" class="anchor">
        <span class="anchor-text">{{ pendingQuote || 'выбранный блок' }}</span>
        <n-button size="tiny" quaternary title="Снять привязку" @click="emit('clear-anchor')">
          Открепить
        </n-button>
      </div>
      <n-input
        :value="draft('')"
        type="textarea"
        size="small"
        :autosize="{ minRows: 2, maxRows: 5 }"
        :placeholder="pendingBlock ? 'Комментарий к блоку…' : 'Комментарий к документу…'"
        @update:value="setDraft('', $event)"
      />
      <n-button size="small" type="primary" :disabled="!draft('').trim()" @click="submitRoot">
        Отправить
      </n-button>
    </div>
  </aside>
</template>

<style scoped>
.doc-comments {
  display: flex;
  flex-direction: column;
  width: 300px;
  flex: none;
  min-height: 0;
  border-left: 1px solid var(--t-border);
  padding-left: 12px;
  gap: 8px;
}
@media (max-width: 900px) {
  .doc-comments {
    width: auto;
    max-height: 40vh;
    border-left: none;
    border-top: 1px solid var(--t-border);
    padding-left: 0;
    padding-top: 12px;
  }
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
.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.section-title {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
/* Threads are neutral surfaces; only the selected one takes the accent, so the
   panel has exactly one thing drawing the eye at a time. */
.thread {
  padding: 8px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  cursor: pointer;
}
.thread.active {
  border-color: transparent;
  background:
    linear-gradient(var(--t-surface), var(--t-surface)) padding-box,
    var(--t-accent-grad) border-box;
  border: 1px solid transparent;
}
/* A settled thread steps back rather than disappearing: the decision is part of
   the document's history. */
.thread.done {
  opacity: 0.6;
}
.quote {
  margin: 0 0 6px;
  padding-left: 6px;
  border-left: 2px solid var(--t-border);
  color: var(--t-text3);
  font-size: 11px;
  line-height: 1.4;
}
.comment {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.comment.reply {
  margin-top: 8px;
  padding-left: 8px;
  border-left: 1px solid var(--t-border);
}
.meta {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
}
.author {
  font-weight: 600;
  color: var(--t-text2);
}
.time {
  color: var(--t-text3);
}
.body {
  margin: 0;
  font-size: 13px;
  line-height: 1.45;
  color: var(--t-text1);
  white-space: pre-wrap;
  word-break: break-word;
}
.row {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
}
.thread-actions {
  flex-wrap: wrap;
}
.grow {
  flex: 1;
}
.empty {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--t-text3);
}
.panel-foot {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: none;
}
.anchor {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-left: 6px;
  border-left: 2px solid var(--t-primary);
}
.anchor-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text3);
  font-size: 11px;
}
</style>
