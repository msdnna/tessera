<script setup>
// The document tree (v2, #3106, mockup DocsPage): a --t-col panel beside the
// documents, like the project tree in the sidebar. Rows are the workspace's
// documents nested by parent_id — the same flat list the section already loads,
// so the tree costs no request of its own. A row opens its document; the
// chevron folds its children. Expansion is remembered per workspace
// (localStorage — UX state, not data), and the path to the open document is
// always unfolded.
//
// Keys on a focused row: ↑↓ move, → unfolds (or steps into the first child),
// ← folds (or steps out to the parent), ↵ opens.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  ChevronForwardOutline,
  CloseOutline,
  DocumentTextOutline,
  FolderOutline,
  GridOutline,
  SearchOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { sGet, sSet } from '@/utils/serverStorage'

const props = defineProps({
  docs: { type: Array, default: () => [] },
  selectedId: { type: String, default: '' },
  wsId: { type: String, default: '' },
  wsName: { type: String, default: '' },
  templatesCount: { type: Number, default: 0 },
  // Floating over the text (the tree did not fit beside it): a close button.
  floating: { type: Boolean, default: false },
  // The documents' trash (#3006) is what the section shows right now.
  trashActive: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'create', 'templates', 'close', 'trash'])
const { t } = useI18n()

const byParent = computed(() => {
  const m = new Map()
  for (const d of props.docs) {
    const k = d.parent_id || ''
    if (!m.has(k)) m.set(k, [])
    m.get(k).push(d)
  }
  return m
})
const byId = computed(() => new Map(props.docs.map((d) => [d.id, d])))
const kids = (id) => byParent.value.get(id) || []

// ── expansion ──
const storeKey = computed(() => `tessera_doc_tree_open:${props.wsId}`)
const open = ref(new Set())
function loadOpen() {
  try {
    open.value = new Set(JSON.parse(sGet(storeKey.value) || '[]'))
  } catch {
    open.value = new Set()
  }
}
function saveOpen() {
  sSet(storeKey.value, JSON.stringify([...open.value]))
}
watch(() => props.wsId, loadOpen, { immediate: true })
function toggle(id, on = !open.value.has(id)) {
  const next = new Set(open.value)
  if (on) next.add(id)
  else next.delete(id)
  open.value = next
  saveOpen()
}
// The path to the open document is always unfolded — opening a document from
// a link must show where it lives.
watch(
  () => [props.selectedId, props.docs.length],
  () => {
    let cur = byId.value.get(props.selectedId)
    const next = new Set(open.value)
    let changed = false
    while (cur?.parent_id) {
      if (!next.has(cur.parent_id)) {
        next.add(cur.parent_id)
        changed = true
      }
      cur = byId.value.get(cur.parent_id)
    }
    if (changed) {
      open.value = next
      saveOpen()
    }
  },
  { immediate: true },
)

// ── filter ──
const searching = ref(false)
const query = ref('')
const searchEl = ref(null)
async function startSearch() {
  searching.value = true
  await nextTick()
  searchEl.value?.focus()
}
function stopSearch() {
  searching.value = false
  query.value = ''
}

// The visible rows, flattened with their depth. While filtering: every match
// with its ancestors, unfolded.
const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  const out = []
  if (q) {
    const keep = new Set()
    for (const d of props.docs) {
      if (!(d.title || '').toLowerCase().includes(q)) continue
      let cur = d
      while (cur) {
        keep.add(cur.id)
        cur = cur.parent_id ? byId.value.get(cur.parent_id) : null
      }
    }
    const walk = (pid, depth) => {
      for (const d of kids(pid)) {
        if (!keep.has(d.id)) continue
        out.push({ doc: d, depth, count: kids(d.id).length, open: true })
        walk(d.id, depth + 1)
      }
    }
    walk('', 0)
    return out
  }
  const walk = (pid, depth) => {
    for (const d of kids(pid)) {
      const count = kids(d.id).length
      const isOpen = count > 0 && open.value.has(d.id)
      out.push({ doc: d, depth, count, open: isOpen })
      if (isOpen) walk(d.id, depth + 1)
    }
  }
  walk('', 0)
  return out
})

// ── keys ──
const listEl = ref(null)
function focusRow(id) {
  listEl.value?.querySelector(`[data-doc-row="${id}"]`)?.focus()
}
function onRowKey(e, row, i) {
  const list = rows.value
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const next = list[i + (e.key === 'ArrowDown' ? 1 : -1)]
    if (next) focusRow(next.doc.id)
  } else if (e.key === 'ArrowRight') {
    e.preventDefault()
    if (!row.count) return
    if (!row.open) toggle(row.doc.id, true)
    else focusRow(kids(row.doc.id)[0]?.id)
  } else if (e.key === 'ArrowLeft') {
    e.preventDefault()
    if (row.open) toggle(row.doc.id, false)
    else if (row.doc.parent_id) focusRow(row.doc.parent_id)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    emit('open', row.doc)
  }
}
function onSearchKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    stopSearch()
  } else if (e.key === 'ArrowDown' && rows.value.length) {
    e.preventDefault()
    focusRow(rows.value[0].doc.id)
  } else if (e.key === 'Enter' && rows.value.length) {
    e.preventDefault()
    emit('open', rows.value[0].doc)
  }
}
</script>

<template>
  <aside class="dtr" :class="{ floating }" data-testid="doc-tree">
    <header class="dtr-head">
      <template v-if="!searching">
        <span class="dtr-title">{{ t('documents.view.crumbRoot') }}</span>
        <button
          type="button"
          class="dtr-ic"
          :title="t('documents.tree.search')"
          :aria-label="t('documents.tree.search')"
          data-testid="doc-tree-search"
          @click="startSearch"
        >
          <n-icon :component="SearchOutline" :size="15" />
        </button>
      </template>
      <label v-else class="dtr-search">
        <n-icon :component="SearchOutline" :size="14" class="dtr-search-ic" />
        <input
          ref="searchEl"
          v-model="query"
          class="dtr-search-in"
          :placeholder="t('documents.tree.searchPlaceholder')"
          @keydown="onSearchKey"
          @blur="!query && stopSearch()"
        />
      </label>
      <button
        type="button"
        class="dtr-ic"
        :title="t('documents.view.create')"
        :aria-label="t('documents.view.create')"
        data-testid="doc-tree-new"
        @click="emit('create')"
      >
        <n-icon :component="AddOutline" :size="17" />
      </button>
      <button
        v-if="floating"
        type="button"
        class="dtr-ic"
        :title="t('common.action.close')"
        :aria-label="t('common.action.close')"
        @click="emit('close')"
      >
        <n-icon :component="CloseOutline" :size="16" />
      </button>
    </header>

    <div ref="listEl" class="dtr-body" role="tree" :aria-label="t('documents.view.crumbRoot')">
      <div v-if="wsName" class="dtr-sec">{{ wsName }}</div>
      <p v-if="!rows.length" class="dtr-empty">
        {{ query ? t('documents.tree.nothing') : t('documents.view.empty') }}
      </p>
      <div
        v-for="(row, i) in rows"
        :key="row.doc.id"
        class="dtr-row"
        :class="{ active: row.doc.id === selectedId }"
        :style="{ paddingLeft: `${6 + row.depth * 14}px` }"
        role="treeitem"
        tabindex="0"
        :aria-expanded="row.count ? row.open : undefined"
        :aria-selected="row.doc.id === selectedId"
        :data-doc-row="row.doc.id"
        data-testid="doc-tree-row"
        @click="emit('open', row.doc)"
        @keydown="onRowKey($event, row, i)"
      >
        <button
          v-if="row.count"
          type="button"
          class="dtr-chev"
          :class="{ open: row.open }"
          tabindex="-1"
          :aria-label="row.open ? t('documents.tree.fold') : t('documents.tree.unfold')"
          @click.stop="toggle(row.doc.id)"
        >
          <n-icon :component="ChevronForwardOutline" :size="12" />
        </button>
        <span v-else class="dtr-chev-sp" />
        <span v-if="row.doc.icon" class="dtr-emoji">{{ row.doc.icon }}</span>
        <n-icon
          v-else
          :component="row.count ? FolderOutline : DocumentTextOutline"
          :size="15"
          class="dtr-glyph"
        />
        <span class="dtr-name">{{ row.doc.title || t('documents.view.untitled') }}</span>
        <span v-if="row.count && !row.open" class="dtr-n">{{ row.count }}</span>
      </div>
    </div>

    <footer class="dtr-foot">
      <button
        type="button"
        class="dtr-row"
        data-testid="doc-tree-templates"
        @click="emit('templates')"
      >
        <n-icon :component="GridOutline" :size="15" class="dtr-glyph" />
        <span class="dtr-name">{{ t('documents.tree.templates') }}</span>
        <span v-if="templatesCount" class="dtr-n">{{ templatesCount }}</span>
      </button>
      <button
        type="button"
        class="dtr-row"
        :class="{ active: trashActive }"
        data-testid="doc-tree-trash"
        @click="emit('trash')"
      >
        <n-icon :component="TrashOutline" :size="15" class="dtr-glyph" />
        <span class="dtr-name">{{ t('documents.tree.trash') }}</span>
      </button>
    </footer>
  </aside>
</template>

<style scoped>
/* 240px, --t-col, radius 18 (mockup DocsPage): rows 30px, 14px per level. */
.dtr {
  display: flex;
  flex-direction: column;
  width: 240px;
  min-height: 0;
  padding: 8px;
  box-sizing: border-box;
  border-radius: var(--t-r-col);
  background: var(--t-col);
  color: var(--t-text1);
}
.dtr.floating {
  box-shadow: var(--t-sh-pop);
  background: var(--t-pop);
}
.dtr-head {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 36px;
  padding: 0 2px 0 8px;
  flex: none;
}
.dtr-title {
  flex: 1;
  font-size: 14px;
  font-weight: 700;
}
.dtr-ic {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.dtr-ic:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dtr-ic:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.dtr-search {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 8px;
  border-radius: 8px;
  background: var(--t-ctl);
  min-width: 0;
}
.dtr-search:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.dtr-search-ic {
  color: var(--t-text3);
}
.dtr-search-in {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
}
.dtr-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 0;
}
.dtr-sec {
  padding: 12px 8px 6px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dtr-empty {
  margin: 8px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.dtr-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  height: 30px;
  padding: 0 8px 0 6px;
  box-sizing: border-box;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.dtr-row:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dtr-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.dtr-row.active {
  background: var(--t-active);
  color: var(--t-text1);
  font-weight: 650;
}
.dtr-row.active .dtr-glyph {
  color: var(--t-primary);
}
.dtr-chev,
.dtr-chev-sp {
  flex: none;
  width: 16px;
  height: 16px;
}
.dtr-chev {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  transition: transform 0.12s ease;
}
.dtr-chev.open {
  transform: rotate(90deg);
}
.dtr-chev:hover {
  color: var(--t-text1);
}
.dtr-glyph {
  flex: none;
  color: var(--t-text3);
}
.dtr-emoji {
  flex: none;
  width: 15px;
  font-size: 13px;
  line-height: 1;
  text-align: center;
}
.dtr-name {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dtr-n {
  flex: none;
  font-size: 11.5px;
  color: var(--t-text3);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.dtr-foot {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-top: 8px;
}
.dtr-foot .dtr-row {
  padding-left: 8px;
}
</style>
