<script setup>
// «Связать #N с задачей · ⇧L» (#3118, mockup PropPicker f=rel): the relation
// form moved out of the panel into a popover form of the PropPicker frame.
// On top, the links the task already has (✕ unlinks), then the kind as a
// segment (⌥1–4 — the digits in the field are the task's number), the search
// over the workspace's tasks grouped by project · board, and ↵ links the task
// under the cursor (or the typed number) and stays open for the next one.
//
// The host owns the relations list (its count is on the section); a new link
// is published back through `update:relations`, an unlink asked for through
// `remove` (the host has the undo toast).
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, useMessage } from 'naive-ui'
import { CloseOutline, SearchOutline } from '@vicons/ionicons5'
import PropPicker from '../../kit/PropPicker.vue'
import { tasks as tasksApi, workspaces as wsApi } from '@/api'
import { keyOf } from '@/shortcuts/keymap'
import { hasModKey, isMac } from '@/utils/keys'
import { footerFor, taskGlyph, WIDTHS } from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  taskId: { type: String, default: null },
  taskNumber: { type: [Number, String], default: null },
  relations: { type: Array, default: () => [] },
  wsId: { type: String, default: null },
  readonly: { type: String, default: '' },
  testid: { type: String, default: 'rel-form' },
})
const emit = defineEmits(['update:show', 'update:relations', 'changed', 'remove'])
const { t } = useI18n()
const message = useMessage()

const KINDS = ['relates', 'blocks', 'blocked_by', 'duplicates']
const kind = ref('relates')
const query = ref('')
const linking = ref(false)
const all = ref([])
let fetched = false
async function ensureTasks() {
  if (fetched || !props.wsId) return
  fetched = true
  try {
    // include_subtasks: subtasks link to each other too (blocking deps).
    const res = await wsApi.tasks(props.wsId, { include_subtasks: 1 })
    all.value = res.data || []
  } catch {
    fetched = false // a number typed by hand still links
  }
}
watch(
  () => props.show,
  (on) => {
    if (!on) return
    query.value = ''
    ensureTasks()
  },
)
watch(
  () => props.wsId,
  () => {
    fetched = false
    all.value = []
  },
)

// The segment's caps: ⌥1 on a Mac; elsewhere only the digit — «Alt» is in
// the footer («Alt 1–4 вид»), four of them would not fit the segment.
const kinds = computed(() =>
  KINDS.map((k, i) => ({
    value: k,
    label: t(`task.relations.kind.${k}`),
    cap: isMac ? `⌥${i + 1}` : String(i + 1),
  })),
) // Numbers already linked with the kind the form is armed with — the same pair
// and kind again would do nothing (ON CONFLICT DO NOTHING) but journal twice.
const linkedNumbers = computed(
  () =>
    new Set(
      props.relations.filter((r) => r.kind === kind.value).map((r) => Number(r.related_number)),
    ),
)
const typedNumber = computed(() => {
  const raw = query.value.trim().replace(/^#/, '').trim()
  const n = Number(raw)
  return raw !== '' && Number.isInteger(n) && n > 0 ? n : null
})

const items = computed(() => {
  const ql = query.value.trim().toLowerCase().replace(/^#/, '')
  const groups = new Map()
  let n = 0
  for (const x of all.value) {
    if (x.id === props.taskId || x.number == null) continue
    if (linkedNumbers.value.has(Number(x.number))) continue
    if (ql && !(String(x.number).startsWith(ql) || (x.title || '').toLowerCase().includes(ql)))
      continue
    const key = `${x.project_name || '—'} · ${x.board_name || '—'}`
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key).push(x)
    if (++n >= 50) break
  }
  const out = []
  for (const [title, list] of groups) {
    out.push({ kind: 'section', id: `sec:${title}`, title })
    for (const x of list)
      out.push({
        kind: 'row',
        id: `t:${x.id}`,
        label: x.title,
        mono: `#${x.number}`,
        status: taskGlyph(x),
        testid: 'rel-pick',
        run: () => link(Number(x.number)),
      })
  }
  return out
})

async function link(number) {
  if (props.readonly || linking.value || !number) return
  if (linkedNumbers.value.has(number)) {
    message.info(t('task.relations.already', { number }))
    query.value = ''
    return
  }
  // The task this form is about, taken before the first await: the modal swaps
  // tasks under a mounted form (#2932).
  const tid = props.taskId
  linking.value = true
  try {
    await tasksApi.addRelation(tid, number, kind.value)
    query.value = ''
    const r = await tasksApi.relations(tid)
    if (tid !== props.taskId) return
    emit('update:relations', r.data || [])
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    linking.value = false
  }
}

function onKey(e, api) {
  if (props.readonly) return false
  const k = keyOf(e)
  if (e.altKey && !hasModKey(e) && k && /^[1-4]$/.test(k)) {
    e.preventDefault()
    kind.value = KINDS[Number(k) - 1]
    return true
  }
  if (e.key === 'Enter' && !hasModKey(e) && !e.altKey) {
    e.preventDefault()
    const row = api.row
    // A number typed in full links that task even when the list (still
    // loading, or of another workspace's numbering) doesn't show it.
    if (typedNumber.value != null && (!row || row.mono !== `#${typedNumber.value}`))
      link(typedNumber.value)
    else if (row) api.pick(row)
    else if (query.value.trim()) message.warning(t('task.relations.noMatches'))
    return true
  }
  return false
}
defineExpose({ link })
const relKindLabel = (k) => (KINDS.includes(k) ? t(`task.relations.kind.${k}`) : k)
const footer = computed(() =>
  props.readonly ? footerFor('readonly', t) : footerFor('relation', t),
)
const title = computed(() =>
  props.taskNumber
    ? t('props.relation.title', { number: props.taskNumber })
    : t('props.relation.titleBare'),
)
</script>

<template>
  <PropPicker
    v-model:query="query"
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="WIDTHS.relation"
    :title="title"
    key-id="task.relation"
    search
    :placeholder="t('props.relation.search')"
    :items="items"
    :empty="query.trim() ? { icon: SearchOutline, title: t('props.relation.nothing') } : null"
    :footer="footer"
    :readonly="readonly"
    :on-key="onKey"
    :testid="testid"
    search-testid="rel-number"
    @update:show="emit('update:show', $event)"
  >
    <template #top>
      <div v-if="relations.length" class="rl-linked">
        <span v-for="r in relations" :key="r.id" class="rl-chip" data-testid="rel-linked">
          <span class="rl-dim">{{ relKindLabel(r.kind) }}</span>
          <span class="rl-dim">#{{ r.related_number }}</span>
          <span class="rl-t">{{ r.related_title }}</span>
          <button
            v-if="!readonly"
            type="button"
            class="rl-x"
            :aria-label="t('task.relations.unlink')"
            tabindex="-1"
            @mousedown.prevent
            @click="emit('remove', r)"
          >
            <n-icon :component="CloseOutline" :size="12" />
          </button>
        </span>
      </div>
      <div class="rl-seg" role="radiogroup">
        <button
          v-for="k in kinds"
          :key="k.value"
          type="button"
          role="radio"
          class="rl-seg-b"
          :class="{ on: kind === k.value }"
          :aria-checked="kind === k.value"
          tabindex="-1"
          :data-testid="`rel-kind-${k.value}`"
          @mousedown.prevent
          @click="kind = k.value"
        >
          {{ k.label }}<span class="rl-cap">{{ k.cap }}</span>
        </button>
      </div>
    </template>
  </PropPicker>
</template>

<style scoped>
.rl-linked {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 2px 8px 6px;
}
.rl-chip {
  max-width: 100%;
  height: 24px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 3px 0 9px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-pill);
  font-size: 12px;
  white-space: nowrap;
}
.rl-dim {
  flex: none;
  color: var(--t-text3);
}
.rl-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rl-x {
  flex: none;
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.rl-x:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.rl-seg {
  display: flex;
  gap: 2px;
  margin: 2px 2px 4px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-ctl);
}
.rl-seg-b {
  flex: 1;
  min-width: 0;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 4px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  overflow: hidden;
  cursor: pointer;
}
.rl-seg-b.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.rl-cap {
  font-size: 11px;
  font-weight: 500;
  color: var(--t-text3);
}
.kit-layer:not(.al) .rl-cap {
  display: none;
}
.kit-layer:not(.al) .rl-seg-b {
  height: 36px;
  font-size: 13.5px;
}
</style>
