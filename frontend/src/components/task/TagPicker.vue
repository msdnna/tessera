<script setup>
// The tag picker (v2, #3113 — mockups TagPicker, PackageC «Пикер тегов»): a 300px
// popover with a search field on top, the task's tags, the found ones, «Создать
// тег «…» ⌘↵» when nothing is called that, «Управлять тегами…» (the «Проект»
// window on «Теги») and a key footer. Opened from the task modal's «Теги» row and
// from the tag pill of a board card; on a phone it is a bottom sheet.
//
// Keys, with the focus in the search field: ↑↓ move, ↵ toggles the row (or
// creates / opens the manager on those rows), Space toggles once the arrows have
// picked a row (until then it types — tag names have spaces), ⌘↵ creates the typed
// tag, Esc closes the picker and only the picker.
//
// The picker says what to do; the host does it (`toggle`, `create`, `manage`) —
// the modal and the card keep their own copies of the task's tags. Tags under a
// GitLab rule prefix (the project window's lock) are never offered, and the ones
// already on the task are shown locked: their name and presence come from GitLab.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  CheckmarkOutline,
  LockClosedOutline,
  PricetagOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import AnchorLayer from '../kit/AnchorLayer.vue'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import TesseraSpinner from '../TesseraSpinner.vue'
import TagPill from '../TagPill.vue'
import { CAPS } from '@/utils/keyCaps'
import { canonPrefix, matchTagByName, tagNamespace, tagParts } from '@/utils/tagGroups'
import { uiCollator } from '@/i18n'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'bottom-start' },
  // Every tag of the task's project, and the ids on the task.
  tags: { type: Array, default: () => [] },
  selected: { type: Array, default: () => [] },
  prefixNames: { type: Object, default: () => ({}) },
  // Canonical prefixes owned by a GitLab label rule (Set or array).
  lockedPrefixes: { type: [Object, Array], default: null },
  // A create is in flight — the create row spins and ignores repeats.
  creating: { type: Boolean, default: false },
  // Host answer to «create this name»: resolves true once the tag is on the task
  // (the query is cleared then).
  create: { type: Function, default: null },
  canManage: { type: Boolean, default: true },
  // The catalog key shown in the search field (task.tags in the modal, card.tags
  // on a card).
  keyId: { type: String, default: '' },
  testid: { type: String, default: 'tag-picker' },
})
const emit = defineEmits(['update:show', 'toggle', 'manage'])
const { t } = useI18n()

const query = ref('')
const input = ref(null)
const active = ref(0)
// Space toggles once a row has been picked with the arrows; before that the
// reader is typing a name.
const navigated = ref(false)

const lockSet = computed(() => {
  const l = props.lockedPrefixes
  if (!l) return new Set()
  return l instanceof Set ? l : new Set(l)
})
const isLocked = (tag) => {
  const key = canonPrefix(tagNamespace(tag.name))
  return !!key && lockSet.value.has(key)
}

const byName = (a, b) => uiCollator().compare(a.name || '', b.name || '')
const selectedSet = computed(() => new Set(props.selected))
// The task's own tags, in name order — never filtered by the query (mockup).
const own = computed(() => props.tags.filter((g) => selectedSet.value.has(g.id)).sort(byName))
// Everything else that may be attached by hand.
const offered = computed(() =>
  props.tags.filter((g) => !selectedSet.value.has(g.id) && !isLocked(g)),
)
const q = computed(() => query.value.trim().toLowerCase())
function haystack(tag) {
  const p = tagParts(tag.name, props.prefixNames)
  return `${tag.name} ${p.hasScope ? `${p.scope} ${p.label}` : ''}`.toLowerCase()
}
// What the query finds, best match first: the exact name, then names starting
// with it, then the rest.
const found = computed(() => {
  if (!q.value) return [...offered.value].sort(byName)
  const rank = (tag) => {
    const n = (tag.name || '').toLowerCase()
    if (n === q.value) return 0
    if (
      n.startsWith(q.value) ||
      tagParts(tag.name, props.prefixNames).label.toLowerCase().startsWith(q.value)
    )
      return 1
    return 2
  }
  return offered.value
    .filter((g) => haystack(g).includes(q.value))
    .sort((a, b) => rank(a) - rank(b) || byName(a, b))
})
// «Создать тег «…»» — when nothing on the project (the task's tags included) is
// already called that. Tags under a GitLab prefix are out of reach on purpose, so
// a name matching one of them is still a fresh tag.
const canCreate = computed(
  () =>
    !!props.create &&
    !!q.value &&
    !matchTagByName(
      props.tags.filter((g) => !isLocked(g)),
      query.value,
    ),
)

// The keyboard's rows, in screen order.
const rows = computed(() => {
  const out = []
  for (const g of own.value) if (!isLocked(g)) out.push({ kind: 'tag', tag: g, on: true })
  for (const g of found.value) out.push({ kind: 'tag', tag: g, on: false })
  if (canCreate.value) out.push({ kind: 'create' })
  if (props.canManage) out.push({ kind: 'manage' })
  return out
})
const rowIndex = (pred) => rows.value.findIndex(pred)
const isActive = (pred) => rowIndex(pred) === active.value

// Typing points the cursor at the best match, or at «Создать». A name that only
// the task's own tags answer to points at nothing: ↵ then goes to the host's
// create, which knows the tag is already on (and says so) — ↵ on a typed name
// must never take a tag OFF.
watch(query, () => {
  navigated.value = false
  if (!q.value) {
    active.value = 0
    return
  }
  const firstFound = rows.value.findIndex((r) => r.kind === 'tag' && !r.on)
  active.value = firstFound >= 0 ? firstFound : rows.value.findIndex((r) => r.kind === 'create')
})
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    query.value = ''
    active.value = 0
    navigated.value = false
    await nextTick()
    await nextTick()
    input.value?.focus({ preventScroll: true })
  },
)

function close() {
  emit('update:show', false)
}
async function doCreate() {
  if (!props.create || props.creating || !query.value.trim()) return
  const ok = await props.create(query.value.trim())
  if (ok) query.value = ''
}
function activate(row, viaKey = false) {
  if (!row) return
  if (row.kind === 'tag') {
    emit('toggle', row.tag.id)
    if (viaKey && q.value) query.value = ''
  } else if (row.kind === 'create') doCreate()
  else if (row.kind === 'manage') {
    emit('manage', query.value.trim())
    close()
  }
}

function move(step) {
  const n = rows.value.length
  if (!n) return
  active.value = active.value < 0 && step < 0 ? n - 1 : (active.value + step + n) % n
  navigated.value = true
  nextTick(() => listEl.value?.querySelector('.tp-row.on')?.scrollIntoView?.({ block: 'nearest' }))
}
const listEl = ref(null)
function onKeydown(e) {
  if (e.isComposing) return
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    close()
    return
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    move(e.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    // A name the project already has is that tag, not a twin: the host's
    // create resolves it (attaches it, or says it is on already).
    if (q.value) doCreate()
    return
  }
  if (e.key === 'Enter') {
    e.preventDefault()
    const row = rows.value[active.value]
    if (row) activate(row, true)
    else if (q.value) doCreate()
    return
  }
  if (e.key === ' ' && navigated.value) {
    const row = rows.value[active.value]
    if (row?.kind === 'tag') {
      e.preventDefault()
      activate(row)
    }
  }
}

const count = (tag) => (tag.task_count != null ? tag.task_count : '')
defineExpose({ query })
</script>

<template>
  <AnchorLayer
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :title="t('task.tags.title')"
    :testid="testid"
    @update:show="(v) => !v && close()"
  >
    <div class="tp" role="dialog" :aria-label="t('task.tags.title')" @keydown="onKeydown">
      <label class="tp-search">
        <n-icon :component="PricetagOutline" :size="14" class="tp-sicon" />
        <input
          ref="input"
          v-model="query"
          class="tp-input"
          :placeholder="t('task.tags.search')"
          :aria-label="t('task.tags.search')"
          data-testid="tag-picker-search"
          spellcheck="false"
          autocomplete="off"
        />
        <ShortcutKeys v-if="keyId" :id="keyId" class="tp-key" />
      </label>

      <div ref="listEl" class="tp-list">
        <template v-if="own.length">
          <div class="tp-head">
            <span class="tp-head-t">{{ t('task.tags.onTask') }}</span>
            <span class="tp-head-n">{{ own.length }}</span>
          </div>
          <div
            v-for="g in own"
            :key="g.id"
            class="tp-row"
            :class="{
              on: !isLocked(g) && isActive((r) => r.tag?.id === g.id),
              locked: isLocked(g),
            }"
            role="menuitemcheckbox"
            aria-checked="true"
            :title="isLocked(g) ? t('task.tags.locked') : ''"
            :data-testid="`tag-picker-row-${g.id}`"
            @mouseenter="!isLocked(g) && (active = rowIndex((r) => r.tag?.id === g.id))"
            @mousedown.prevent
            @click="!isLocked(g) && emit('toggle', g.id)"
          >
            <span class="tp-box on">
              <n-icon :component="CheckmarkOutline" :size="12" />
            </span>
            <TagPill class="tp-pill" :tag="g" :prefix-names="prefixNames" />
            <span class="tp-grow" />
            <n-icon v-if="isLocked(g)" :component="LockClosedOutline" :size="13" class="tp-lock" />
            <span v-else class="tp-n">{{ count(g) }}</span>
          </div>
        </template>

        <div v-if="found.length" class="tp-head">
          <span class="tp-head-t">{{ q ? t('task.tags.found') : t('task.tags.all') }}</span>
          <span class="tp-head-n">{{ found.length }}</span>
        </div>
        <div
          v-for="g in found"
          :key="g.id"
          class="tp-row"
          :class="{ on: isActive((r) => r.tag?.id === g.id) }"
          role="menuitemcheckbox"
          aria-checked="false"
          :data-testid="`tag-picker-row-${g.id}`"
          @mouseenter="active = rowIndex((r) => r.tag?.id === g.id)"
          @mousedown.prevent
          @click="emit('toggle', g.id)"
        >
          <span class="tp-box" />
          <TagPill class="tp-pill" :tag="g" :prefix-names="prefixNames" />
          <span class="tp-grow" />
          <span class="tp-n">{{ count(g) }}</span>
        </div>
        <div v-if="q && !found.length && !canCreate" class="tp-empty">
          {{ t('task.tags.nothing') }}
        </div>

        <div
          v-if="canCreate"
          class="tp-row tp-act"
          :class="{ on: isActive((r) => r.kind === 'create'), busy: creating }"
          role="menuitem"
          data-testid="tag-picker-create"
          @mouseenter="active = rowIndex((r) => r.kind === 'create')"
          @mousedown.prevent
          @click="doCreate"
        >
          <TesseraSpinner v-if="creating" :size="14" />
          <n-icon v-else :component="AddOutline" :size="16" class="tp-aicon" />
          <span class="tp-act-t">{{ t('task.tags.create', { name: query.trim() }) }}</span>
          <Kbd>{{ CAPS.modEnter }}</Kbd>
        </div>
      </div>

      <template v-if="canManage">
        <div class="tp-gap" />
        <div
          class="tp-row tp-act"
          :class="{ on: isActive((r) => r.kind === 'manage') }"
          role="menuitem"
          data-testid="task-tags-manage"
          @mouseenter="active = rowIndex((r) => r.kind === 'manage')"
          @mousedown.prevent
          @click="activate({ kind: 'manage' })"
        >
          <n-icon :component="SettingsOutline" :size="16" class="tp-aicon" />
          <span class="tp-act-t">{{ t('task.tags.manage') }}</span>
          <span class="tp-n">{{ t('task.tags.manageWhere') }}</span>
        </div>
      </template>

      <div class="tp-foot">
        <span class="tp-fk"
          ><Kbd>{{ CAPS.arrows }}</Kbd
          >{{ t('task.tags.keyPick') }}</span
        >
        <span class="tp-fk"
          ><Kbd>{{ CAPS.space }}</Kbd
          >{{ t('task.tags.keyToggle') }}</span
        >
        <span class="tp-grow" />
        <Kbd>{{ CAPS.escape }}</Kbd>
      </div>
    </div>
  </AnchorLayer>
</template>

<style scoped>
.tp {
  width: 300px;
  max-width: calc(100vw - 16px);
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px;
  border-radius: var(--t-r-card);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.tp-search {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  margin: 2px 2px 4px;
  padding: 0 6px 0 10px;
  border-radius: 9px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
  cursor: text;
}
.tp-sicon {
  flex: none;
  color: var(--t-text3);
}
.tp-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.tp-input::placeholder {
  color: var(--t-placeholder);
}
.tp-key {
  flex: none;
}
.tp-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: min(320px, 50vh);
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}
.tp-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.tp-head-t {
  flex: 1;
}
.tp-head-n {
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.tp-row {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 8px 0 10px;
  box-sizing: border-box;
  border-radius: 7px;
  font-size: 13.5px;
  cursor: pointer;
  user-select: none;
}
.tp-row.on {
  background: var(--t-hover);
}
.tp-row.locked {
  cursor: default;
}
.tp-row.locked .tp-box {
  opacity: 0.55;
}
.tp-row.busy {
  cursor: progress;
}
.tp-box {
  flex: none;
  width: 16px;
  height: 16px;
  border-radius: 5px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-pill);
  color: #fff;
}
.tp-box.on {
  background: var(--t-accent-grad);
}
.tp-pill {
  min-width: 0;
  max-width: 190px;
}
.tp-grow {
  flex: 1;
}
.tp-n {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tp-lock {
  flex: none;
  color: var(--t-text3);
}
.tp-aicon {
  flex: none;
  color: var(--t-text2);
}
.tp-act-t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tp-empty {
  padding: 8px 10px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.tp-gap {
  height: 6px;
}
.tp-foot {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 34px;
  margin: 4px -6px -6px;
  padding: 0 12px;
  border-radius: 0 0 var(--t-r-card) var(--t-r-card);
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.tp-fk {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

/* A phone: the sheet is the card; rows a touch taller, no key hints. */
.kit-layer:not(.al) .tp {
  width: auto;
  max-width: none;
  padding: 0;
  background: transparent;
  box-shadow: none;
}
.kit-layer:not(.al) .tp-row {
  height: 44px;
  font-size: 15px;
}
.kit-layer:not(.al) .tp-foot,
.kit-layer:not(.al) .tp-key {
  display: none;
}
.kit-layer:not(.al) .tp-input {
  font-size: 16px;
}
</style>
