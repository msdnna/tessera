<script setup>
// «Теги» of the «Проект» window (#2990, mockup ProjectWin / CatalogC): the
// project's tags as a table grouped by prefix. A group's friendly name is edited
// right in its header; a tag is renamed with a click on its name, ↵ or F2 — no
// double click; the colour is a swatch with a palette popover. Rows are
// selected with Space (⇧↑↓ — a range) and the selection is merged (M),
// recoloured (C) or deleted (⌫ — at once, with an undo toast). N — new tag,
// / — filter. Tags under a GitLab label rule are locked: their name and colour
// come from GitLab.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CAPS, cap } from '@/utils/keyCaps'
import { NIcon, NSwitch, NTooltip, useMessage } from 'naive-ui'
import {
  AddOutline,
  FunnelOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  LockClosedOutline,
  GitBranchOutline,
  EllipsisHorizontalOutline,
  GitMergeOutline,
  ColorPaletteOutline,
  TrashOutline,
  CreateOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { projects as projApi, gitlab as glApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useUndoStore } from '@/stores/undo'
import { useThemeStore } from '@/stores/theme'
import { humanizeError } from '@/utils/errors'
import { PICKER_COLORS } from '@/utils/pickerColors'
import { hueGrad } from '@/utils/gradient'
import {
  ruledPrefixes,
  isRuled,
  tagTableGroups,
  rowOrder,
  toggleSelected,
  selectRange,
  stepRow,
  newTagName,
  defaultMergeTarget,
  tagSource,
} from '@/utils/tagTable'
import TagPill from '../TagPill.vue'
import Kbd from '../Kbd.vue'
import InlineEdit from '../kit/InlineEdit.vue'
import ActionMenu from '../kit/ActionMenu.vue'
import PopoverForm from '../kit/PopoverForm.vue'
import ColorPop from './ColorPop.vue'

const props = defineProps({
  project: { type: Object, required: true },
  initialFilter: { type: String, default: '' },
})
const emit = defineEmits(['count', 'changed'])
const { t } = useI18n()
const message = useMessage()
const ws = useWorkspacesStore()
const undo = useUndoStore()
const theme = useThemeStore()

const tags = ref([])
const prefixNames = ref({})
const locked = ref(new Set())
const glPath = ref('')
const loading = ref(true)
const filter = ref(props.initialFilter || '')

async function load() {
  const pid = props.project.id
  try {
    const [tg, pfx, gl] = await Promise.all([
      projApi.tags(pid),
      projApi.tagPrefixes(pid).catch(() => ({ data: [] })),
      ws.currentId
        ? glApi.listIntegrations(ws.currentId).catch(() => ({ data: { integrations: [] } }))
        : Promise.resolve({ data: { integrations: [] } }),
    ])
    if (pid !== props.project.id) return
    tags.value = tg.data || []
    prefixNames.value = Object.fromEntries((pfx.data || []).map((p) => [p.prefix, p.label]))
    const integ = (gl.data?.integrations || []).filter((b) => b.project_id === pid)
    locked.value = ruledPrefixes(integ, pid)
    glPath.value = integ[0]?.project_path || ''
    emit('count', tags.value.length)
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(
  () => props.project.id,
  () => {
    selected.value = new Set()
    load()
  },
)
function changed() {
  emit('changed')
  return load()
}

const groups = computed(() =>
  tagTableGroups(tags.value, {
    prefixNames: prefixNames.value,
    filter: filter.value,
    locked: locked.value,
  }),
)
const groupCount = computed(() => groups.value.filter((g) => g.key).length)
const collapsed = ref(new Set())
function toggleGroup(key) {
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsed.value = next
}
const order = computed(() => rowOrder(groups.value, collapsed.value))
const byId = computed(() => new Map(tags.value.map((tg) => [tg.id, tg])))
const ruled = (tag) => isRuled(tag, locked.value)

// ── row cursor and selection ──
const cursor = ref(null)
const anchor = ref(null)
const selected = ref(new Set())
const selectedTags = computed(() =>
  [...selected.value].map((id) => byId.value.get(id)).filter(Boolean),
)
const rowEls = {}
function focusRow(id) {
  cursor.value = id
  nextTick(() => rowEls[id]?.focus({ preventScroll: false }))
}
function toggleRow(tag) {
  if (ruled(tag)) return
  selected.value = toggleSelected(selected.value, tag.id)
  anchor.value = tag.id
}
function onRowClick(tag, e) {
  cursor.value = tag.id
  if (e.shiftKey && anchor.value) {
    selected.value = selectRange(
      selected.value,
      order.value.filter((id) => !ruled(byId.value.get(id))),
      anchor.value,
      tag.id,
    )
  }
}

// ── new tag / filter ──
const newName = ref('')
const newBusy = ref(false)
const newErr = ref('')
const newEl = ref(null)
const filterEl = ref(null)
async function createTag() {
  const name = newTagName(newName.value)
  if (!name || newBusy.value) {
    if (!name && newName.value.trim()) newErr.value = t('project.tagTable.newInvalid')
    return
  }
  newBusy.value = true
  newErr.value = ''
  try {
    const color = PICKER_COLORS[Math.floor(Math.random() * PICKER_COLORS.length)].hex
    await projApi.createTag(props.project.id, { name, color })
    newName.value = ''
    await changed()
  } catch (e) {
    newErr.value = e.status === 409 ? t('project.tagTable.taken', { name }) : humanizeError(e)
  } finally {
    newBusy.value = false
  }
}

// ── rename ──
const renamingId = ref(null)
const renameValue = ref('')
const renameErr = ref('')
const renameBusy = ref(false)
function startRename(tag) {
  if (ruled(tag)) return
  renamingId.value = tag.id
  renameValue.value = tag.name
  renameErr.value = ''
}
async function commitRename(tag, value, { via }) {
  const n = (value || '').trim()
  if (!n || n === tag.name) {
    if (!n && via === 'enter') return
    renamingId.value = null
    focusRow(tag.id)
    return
  }
  renameBusy.value = true
  try {
    await projApi.updateTag(tag.id, { name: n, color: tag.color || '' })
    renamingId.value = null
    await changed()
    focusRow(tag.id)
  } catch (e) {
    renameErr.value = e.status === 409 ? t('project.tagTable.taken', { name: n }) : humanizeError(e)
  } finally {
    renameBusy.value = false
  }
}
function cancelRename(tag) {
  renamingId.value = null
  focusRow(tag.id)
}

// ── group header: the prefix's friendly name ──
const editingGroup = ref(null)
const groupValue = ref('')
const groupBusy = ref(false)
function startGroupEdit(g) {
  if (!g.key || g.ruled) return
  editingGroup.value = g.key
  groupValue.value = prefixNames.value[g.key] || ''
}
async function commitGroup(g, value) {
  const label = (value || '').trim()
  if (label === (prefixNames.value[g.key] || '')) {
    editingGroup.value = null
    return
  }
  groupBusy.value = true
  try {
    await projApi.setTagPrefix(props.project.id, g.prefix || g.key, label)
    editingGroup.value = null
    await changed()
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    groupBusy.value = false
  }
}

// ── colour ──
const colorFor = ref(null) // tag id, or 'bulk'
const colorAnchor = ref(null)
const colorShow = computed({
  get: () => !!colorFor.value,
  set: (v) => {
    if (!v) colorFor.value = null
  },
})
function openColor(tag, el) {
  if (ruled(tag)) return
  colorAnchor.value = el
  colorFor.value = tag.id
}
async function pickColor(hex) {
  const target = colorFor.value
  const list = target === 'bulk' ? selectedTags.value : [byId.value.get(target)].filter(Boolean)
  try {
    for (const tag of list) {
      if (tag.color !== hex) await projApi.updateTag(tag.id, { name: tag.name, color: hex })
    }
    await changed()
  } catch (e) {
    message.error(humanizeError(e))
  }
}

// ── merge ──
const mergeShow = ref(false)
const mergeTarget = ref(null)
const mergeBusy = ref(false)
const mergeBtn = ref(null)
function openMerge() {
  if (selectedTags.value.length < 2) return
  mergeTarget.value = defaultMergeTarget(selectedTags.value)?.id || null
  mergeShow.value = true
}
async function doMerge() {
  const target = mergeTarget.value
  const sources = selectedTags.value.map((tg) => tg.id).filter((id) => id !== target)
  if (!target || !sources.length) return
  mergeBusy.value = true
  try {
    await projApi.mergeTags(props.project.id, target, sources)
    mergeShow.value = false
    selected.value = new Set()
    await changed()
    message.success(t('project.tagTable.merged', { n: sources.length }))
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    mergeBusy.value = false
  }
}

// ── delete: at once, with an undo ──
async function removeTags(list) {
  const ids = list.filter((tg) => !ruled(tg)).map((tg) => tg.id)
  if (!ids.length) return
  const pid = props.project.id
  try {
    const { data } = await projApi.deleteTags(pid, ids)
    selected.value = new Set()
    await changed()
    const gone = data?.tags || []
    undo.push({
      text:
        gone.length === 1
          ? { key: 'project.tagTable.deletedOne', params: { name: gone[0].name } }
          : { key: 'project.tagTable.deletedMany', params: { n: gone.length } },
      doneText:
        gone.length === 1
          ? { key: 'project.tagTable.restoredOne', params: { name: gone[0].name } }
          : { key: 'project.tagTable.restoredMany', params: { n: gone.length } },
      undo: async () => {
        await projApi.restoreTags(pid, gone)
        if (props.project.id === pid) await changed()
      },
    })
  } catch (e) {
    message.error(humanizeError(e))
  }
}

// ── the row's own menu ──
const menuFor = ref(null)
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => !!menuFor.value,
  set: (v) => {
    if (!v) menuFor.value = null
  },
})
const menuItems = computed(() => [
  { key: 'rename', label: t('project.tagTable.rename'), icon: CreateOutline, caps: ['F2'] },
  { key: 'color', label: t('project.tagTable.recolor'), icon: ColorPaletteOutline, caps: ['C'] },
  { gap: true },
  {
    key: 'delete',
    label: t('project.tagTable.delete'),
    icon: TrashOutline,
    danger: true,
    caps: ['⌫'],
  },
])
function openMenu(tag, el) {
  menuAnchor.value = el
  menuFor.value = tag.id
}
function onMenu(key) {
  const tag = byId.value.get(menuFor.value)
  if (!tag) return
  if (key === 'rename') startRename(tag)
  else if (key === 'color') openColor(tag, rowEls[tag.id]?.querySelector('.tt-sw'))
  else if (key === 'delete') removeTags([tag])
}

// ── keys on the table ──
function onTableKey(e) {
  if (e.isComposing || renamingId.value || editingGroup.value) return
  if (e.target?.tagName === 'INPUT') return
  const cur = cursor.value ? byId.value.get(cursor.value) : null
  const plain = !e.metaKey && !e.ctrlKey && !e.altKey
  const sel = order.value.filter((id) => !ruled(byId.value.get(id)))
  switch (e.key) {
    case 'ArrowDown':
    case 'ArrowUp': {
      e.preventDefault()
      const next = stepRow(order.value, cursor.value, e.key === 'ArrowDown' ? 1 : -1)
      if (!next) return
      if (e.shiftKey) {
        if (!anchor.value) anchor.value = cursor.value || next
        selected.value = selectRange(new Set(), sel, anchor.value, next)
      }
      focusRow(next)
      return
    }
    case ' ':
      if (cur) {
        e.preventDefault()
        toggleRow(cur)
      }
      return
    case 'Enter':
    case 'F2':
      if (cur) {
        e.preventDefault()
        startRename(cur)
      }
      return
    case 'Backspace':
    case 'Delete':
      if (!plain) return
      e.preventDefault()
      removeTags(selectedTags.value.length ? selectedTags.value : cur ? [cur] : [])
      return
  }
  if (!plain) return
  const k = e.code
  if (k === 'KeyN') {
    e.preventDefault()
    newEl.value?.focus()
  } else if (k === 'Slash' || e.key === '/') {
    e.preventDefault()
    filterEl.value?.focus()
  } else if (k === 'KeyM' && selectedTags.value.length >= 2) {
    e.preventDefault()
    openMerge()
  } else if (k === 'KeyC') {
    e.preventDefault()
    if (selectedTags.value.length) {
      colorAnchor.value = colorBtn.value
      colorFor.value = 'bulk'
    } else if (cur) openColor(cur, rowEls[cur.id]?.querySelector('.tt-sw'))
  }
}
const colorBtn = ref(null)

function sourceLabel(tag) {
  const s = tagSource(tag, locked.value)
  if (s === 'rule') return t('project.tagTable.sourceRule')
  if (s === 'gitlab') return t('project.tagTable.sourceGitlab')
  return t('project.tagTable.sourceUser')
}

function focusFirst() {
  nextTick(() => {
    if (filter.value) filterEl.value?.focus()
    else if (order.value[0]) focusRow(order.value[0])
    else newEl.value?.focus()
  })
}
// Esc: a selection is cleared before the window closes.
function escape() {
  if (selected.value.size) {
    selected.value = new Set()
    return true
  }
  return false
}
defineExpose({ focusFirst, escape })
</script>

<template>
  <div class="tagtab" data-testid="project-tags">
    <div class="tt-title">
      <h2>{{ t('project.window.tab.tags') }}</h2>
      <span class="tt-note">
        {{ t('project.tagTable.summary', { tags: tags.length, groups: groupCount }) }}
      </span>
      <span class="tt-grow" />
      <!-- A device setting, not the project's: raw prefixes («T») instead of
           their friendly names on this screen and everywhere else. -->
      <label class="tt-raw-sw" :title="t('project.tags.prefixModeHint')">
        <span>{{ t('project.tags.prefixMode') }}</span>
        <n-switch
          size="small"
          :value="theme.tagPrefixMode === 'raw'"
          @update:value="(v) => theme.setTagPrefixMode(v ? 'raw' : 'name')"
        />
      </label>
    </div>

    <div class="tt-tools">
      <label class="tt-new" :class="{ error: !!newErr }">
        <n-icon :component="AddOutline" :size="15" class="tt-tool-ic" />
        <input
          ref="newEl"
          v-model="newName"
          :placeholder="t('project.tagTable.newPlaceholder')"
          :disabled="newBusy"
          spellcheck="false"
          data-testid="tag-new-input"
          @input="newErr = ''"
          @keydown.enter.prevent="createTag"
          @keydown.esc.stop="((newName = ''), $event.target.blur())"
        />
        <Kbd soft>{{ cap('n') }}</Kbd>
      </label>
      <label class="tt-filter">
        <n-icon :component="FunnelOutline" :size="14" class="tt-tool-ic" />
        <input
          ref="filterEl"
          v-model="filter"
          :placeholder="t('project.tagTable.filter')"
          spellcheck="false"
          data-testid="tag-filter-input"
          @keydown.esc.stop="((filter = ''), $event.target.blur())"
        />
        <Kbd soft>/</Kbd>
      </label>
    </div>
    <div v-if="newErr" class="tt-err">{{ newErr }}</div>

    <div class="tt-head">
      <span />
      <span>{{ t('project.tagTable.colTag') }}</span>
      <span>{{ t('project.tagTable.colColor') }}</span>
      <span>{{ t('project.tagTable.colTasks') }}</span>
      <span>{{ t('project.tagTable.colSource') }}</span>
      <span />
    </div>

    <div class="tt-body" tabindex="-1" @keydown="onTableKey">
      <div v-if="!loading && !groups.length" class="tt-empty">
        {{ filter ? t('project.tagTable.noMatch') : t('project.tagTable.empty') }}
      </div>
      <template v-for="g in groups" :key="g.key">
        <div class="tt-group" :class="{ ruled: g.ruled }" data-testid="tag-group">
          <button
            type="button"
            class="tt-chev"
            :aria-expanded="!collapsed.has(g.key)"
            @click="toggleGroup(g.key)"
          >
            <n-icon
              :component="collapsed.has(g.key) ? ChevronForwardOutline : ChevronDownOutline"
              :size="13"
            />
          </button>
          <template v-if="g.key">
            <div v-if="editingGroup === g.key" class="tt-gedit">
              <InlineEdit
                v-model="groupValue"
                variant="header"
                :show-hint="false"
                :busy="groupBusy"
                :placeholder="t('project.tagTable.prefixPlaceholder')"
                @commit="(v) => commitGroup(g, v)"
                @cancel="editingGroup = null"
              />
              <span class="tt-gedit-note">{{ t('project.tagTable.prefixNote') }}</span>
            </div>
            <button
              v-else
              type="button"
              class="tt-gname"
              :class="{ empty: !g.named }"
              :disabled="g.ruled"
              data-testid="tag-group-name"
              @click="startGroupEdit(g)"
              @keydown.f2.prevent="startGroupEdit(g)"
            >
              {{ g.named ? prefixNames[g.key] : t('project.tagTable.prefixAdd') }}
              <n-icon v-if="!g.ruled" :component="CreateOutline" :size="13" class="tt-gpen" />
            </button>
            <code class="tt-raw">{{ g.prefix.trim() }}</code>
          </template>
          <span v-else class="tt-gname static">{{ g.label }}</span>
          <span class="tt-gcount">{{ g.tags.length }}</span>
          <span class="tt-grow" />
          <span v-if="g.ruled" class="tt-gl">
            <n-icon :component="LockClosedOutline" :size="13" />
            {{ t('project.tagTable.fromGitlab') }}
          </span>
        </div>

        <template v-if="!collapsed.has(g.key)">
          <div
            v-for="tag in g.tags"
            :key="tag.id"
            :ref="(el) => (rowEls[tag.id] = el)"
            class="tt-row"
            :class="{
              sel: selected.has(tag.id),
              ruled: ruled(tag),
              ctx: menuFor === tag.id,
            }"
            tabindex="-1"
            data-testid="tag-row"
            :data-tag="tag.name"
            @click="onRowClick(tag, $event)"
            @focus="cursor = tag.id"
          >
            <button
              type="button"
              class="tt-check"
              :class="{ on: selected.has(tag.id) }"
              :disabled="ruled(tag)"
              :aria-pressed="selected.has(tag.id)"
              tabindex="-1"
              data-testid="tag-check"
              @click.stop="toggleRow(tag)"
            >
              <n-icon v-if="selected.has(tag.id)" :component="CheckmarkOutline" :size="12" />
            </button>
            <div class="tt-name">
              <InlineEdit
                v-if="renamingId === tag.id"
                v-model="renameValue"
                :busy="renameBusy"
                :error="renameErr"
                @commit="(v, how) => commitRename(tag, v, how)"
                @cancel="cancelRename(tag)"
              />
              <n-tooltip v-else-if="ruled(tag)" placement="bottom-start">
                <template #trigger>
                  <span class="tt-pill-wrap">
                    <TagPill :tag="tag" :prefix-names="prefixNames" class="tt-pill" />
                  </span>
                </template>
                <div class="tt-tip-t">{{ t('project.tagTable.ruledTitle') }}</div>
                <div class="tt-tip-s">
                  {{ t('project.tagTable.ruledSub', { prefix: g.prefix.trim(), path: glPath }) }}
                </div>
              </n-tooltip>
              <button
                v-else
                type="button"
                class="tt-pill-btn"
                tabindex="-1"
                :title="t('project.tagTable.renameHint')"
                @click.stop="startRename(tag)"
              >
                <TagPill :tag="tag" :prefix-names="prefixNames" class="tt-pill" />
              </button>
            </div>
            <button
              type="button"
              class="tt-sw"
              :disabled="ruled(tag)"
              tabindex="-1"
              :title="t('project.tagTable.recolor')"
              @click.stop="openColor(tag, $event.currentTarget)"
            >
              <span class="tt-dot" :style="{ background: hueGrad(tag.color || '#888') }" />
              <n-icon v-if="!ruled(tag)" :component="ChevronDownOutline" :size="11" />
            </button>
            <span class="tt-count">{{ tag.task_count ?? '' }}</span>
            <span class="tt-src" :class="tagSource(tag, locked)">
              <n-icon
                v-if="tagSource(tag, locked) === 'rule'"
                :component="LockClosedOutline"
                :size="13"
              />
              <n-icon
                v-else-if="tagSource(tag, locked) === 'gitlab'"
                :component="GitBranchOutline"
                :size="13"
              />
              {{ sourceLabel(tag) }}
            </span>
            <button
              v-if="!ruled(tag)"
              type="button"
              class="tt-more"
              tabindex="-1"
              data-testid="tag-row-menu"
              @click.stop="openMenu(tag, $event.currentTarget)"
            >
              <n-icon :component="EllipsisHorizontalOutline" :size="15" />
            </button>
            <span v-else />
          </div>
        </template>
      </template>
    </div>

    <!-- The selection's own bar (mockup: «3 выбрано · Слить в… M · Перекрасить C ·
         Удалить ⌫ · Esc снять»). -->
    <transition name="tt-bar">
      <div v-if="selected.size" class="tt-bar" data-testid="tag-selection-bar">
        <span class="tt-bar-n">{{ t('project.tagTable.selected', { n: selected.size }) }}</span>
        <button
          ref="mergeBtn"
          type="button"
          class="tt-bar-btn accent"
          :disabled="selected.size < 2"
          data-testid="tag-merge"
          @click="openMerge"
        >
          <n-icon :component="GitMergeOutline" :size="14" />
          {{ t('project.tagTable.merge') }}
          <Kbd>{{ cap('m') }}</Kbd>
        </button>
        <button
          ref="colorBtn"
          type="button"
          class="tt-bar-btn"
          @click="((colorAnchor = colorBtn), (colorFor = 'bulk'))"
        >
          <n-icon :component="ColorPaletteOutline" :size="14" />
          {{ t('project.tagTable.recolor') }}
          <Kbd>{{ cap('c') }}</Kbd>
        </button>
        <button
          type="button"
          class="tt-bar-btn danger"
          data-testid="tag-delete-selected"
          @click="removeTags(selectedTags)"
        >
          <n-icon :component="TrashOutline" :size="14" />
          {{ t('project.tagTable.delete') }}
          <Kbd>{{ CAPS.backspace }}</Kbd>
        </button>
        <button type="button" class="tt-bar-btn ghost" @click="selected = new Set()">
          <Kbd>{{ CAPS.escape }}</Kbd>
          {{ t('project.tagTable.clear') }}
        </button>
      </div>
    </transition>

    <ColorPop
      v-model:show="colorShow"
      :anchor="() => colorAnchor"
      :value="colorFor && colorFor !== 'bulk' ? byId.get(colorFor)?.color || '' : ''"
      :title="
        colorFor === 'bulk' ? t('project.tagTable.recolorMany', { n: selectedTags.length }) : ''
      "
      @pick="pickColor"
    />
    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :width="240"
      :header="byId.get(menuFor)?.name || ''"
      :items="menuItems"
      testid="tag-menu"
      @select="onMenu"
    />
    <PopoverForm
      v-model:show="mergeShow"
      :anchor="() => mergeBtn"
      placement="bottom-start"
      :offset="{ x: 0, y: 8 }"
      :width="320"
      :title="t('project.tagTable.mergeTitle', { n: selectedTags.length })"
      :subtitle="t('project.tagTable.mergeSub')"
      :primary="
        t('project.tagTable.mergeInto', {
          name: byId.get(mergeTarget)?.name || '',
        })
      "
      :busy="mergeBusy"
      :disabled="!mergeTarget"
      hint="list"
      testid="tag-merge-form"
      @submit="doMerge"
    >
      <div class="tt-merge">
        <button
          v-for="tg in selectedTags"
          :key="tg.id"
          type="button"
          class="tt-merge-row"
          :class="{ on: mergeTarget === tg.id }"
          @click="mergeTarget = tg.id"
        >
          <span class="tt-radio" />
          <TagPill :tag="tg" :prefix-names="prefixNames" class="tt-pill" />
          <span class="tt-grow" />
          <span class="tt-count">{{ tg.task_count ?? '' }}</span>
        </button>
      </div>
    </PopoverForm>
  </div>
</template>

<style scoped>
.tagtab {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  padding: 6px 4px 0 0;
}
.tt-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 16px;
}
.tt-raw-sw {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: var(--t-text3);
  cursor: pointer;
}
.tt-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.tt-note {
  font-size: 13px;
  color: var(--t-text3);
}
.tt-tools {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 236px;
  gap: 10px;
}
.tt-new,
.tt-filter {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 8px 0 12px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  cursor: text;
}
.tt-new:focus-within,
.tt-filter:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.tt-new.error {
  box-shadow: 0 0 0 2px var(--t-error);
}
.tt-new input,
.tt-filter input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.tt-new input::placeholder,
.tt-filter input::placeholder {
  color: var(--t-placeholder);
}
.tt-tool-ic {
  color: var(--t-text3);
  flex: none;
}
.tt-err {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--t-error);
}
.tt-head,
.tt-row {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) 74px 80px 170px 32px;
  align-items: center;
  gap: 4px;
}
.tt-head {
  margin-top: 18px;
  padding: 0 4px 8px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.tt-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  outline: 0;
  padding: 0 4px 70px;
}
.tt-empty {
  padding: 32px 0;
  text-align: center;
  font-size: 13px;
  color: var(--t-text3);
}
.tt-group {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 4px 2px 0;
}
.tt-chev {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tt-chev:hover {
  background: var(--t-hover);
}
.tt-gname {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 14px;
  font-weight: 650;
  cursor: pointer;
}
.tt-gname.static {
  cursor: default;
}
.tt-gname:hover:not(:disabled):not(.static) {
  background: var(--t-hover);
}
.tt-gname:disabled {
  cursor: default;
}
.tt-gname.empty {
  font-weight: 500;
  font-style: italic;
  color: var(--t-text3);
}
.tt-gname:focus-visible {
  outline: 0;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.tt-gpen {
  color: var(--t-text3);
  opacity: 0;
}
.tt-gname:hover .tt-gpen,
.tt-gname:focus-visible .tt-gpen {
  opacity: 1;
}
.tt-gedit {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.tt-gedit :deep(.ie) {
  width: 200px;
}
.tt-gedit-note {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.tt-raw {
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 11.5px;
}
.tt-gcount {
  font-size: 12.5px;
  color: var(--t-text3);
}
.tt-grow {
  flex: 1;
}
.tt-gl {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  padding-right: 8px;
}
.tt-row {
  min-height: 40px;
  padding: 0 4px;
  border-radius: 9px;
  outline: 0;
  font-size: 13px;
}
.tt-row:hover,
.tt-row.ctx {
  background: var(--t-hover);
}
.tt-row:focus-visible {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.tt-row.sel {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
}
.tt-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  margin-left: 6px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: var(--t-pill);
  color: #fff;
  cursor: pointer;
}
.tt-check.on {
  background: var(--t-accent-grad);
}
.tt-check:disabled {
  opacity: 0.45;
  cursor: default;
}
.tt-name {
  min-width: 0;
  display: flex;
  align-items: center;
}
.tt-pill-btn {
  padding: 0;
  border: 0;
  background: transparent;
  cursor: text;
  max-width: 100%;
}
.tt-pill-wrap {
  display: inline-flex;
}
.tt-pill {
  font-size: 12px;
  line-height: 1.2;
  padding: 2px 8px;
  border-radius: 999px;
  max-width: 100%;
}
.tt-sw {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  width: fit-content;
  height: 24px;
  padding: 0 4px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tt-sw:hover:not(:disabled) {
  background: var(--t-hover);
}
.tt-sw:disabled {
  cursor: default;
}
.tt-dot {
  width: 16px;
  height: 16px;
  border-radius: 50%;
}
.tt-count {
  font-variant-numeric: tabular-nums;
  color: var(--t-text2);
}
.tt-src {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--t-text2);
  white-space: nowrap;
}
.tt-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  opacity: 0;
}
.tt-row:hover .tt-more,
.tt-row.ctx .tt-more,
.tt-row:focus-visible .tt-more {
  opacity: 1;
}
.tt-more:hover {
  background: var(--t-active);
  color: var(--t-text1);
}
.tt-row.ruled .tt-pill-wrap {
  opacity: 0.92;
}
.tt-tip-t {
  font-weight: 650;
}
.tt-tip-s {
  margin-top: 2px;
  font-size: 12px;
  opacity: 0.8;
}
/* The floating selection bar. */
.tt-bar {
  position: absolute;
  left: 50%;
  bottom: 10px;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px 6px 14px;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  white-space: nowrap;
  z-index: 2;
}
.tt-bar-n {
  font-size: 13.5px;
  font-weight: 650;
  margin-right: 8px;
}
.tt-bar-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.tt-bar-btn:hover:not(:disabled) {
  background: var(--t-hover);
}
.tt-bar-btn:disabled {
  opacity: 0.5;
  cursor: default;
}
.tt-bar-btn.accent {
  background: color-mix(in srgb, var(--t-primary) 18%, transparent);
  font-weight: 600;
}
.tt-bar-btn.accent :deep(svg) {
  color: var(--t-primary);
}
.tt-bar-btn.danger :deep(svg) {
  color: var(--t-error);
}
.tt-bar-btn.ghost {
  color: var(--t-text3);
  font-size: 12px;
}
.tt-bar-enter-active,
.tt-bar-leave-active {
  transition:
    opacity 0.15s,
    transform 0.15s;
}
.tt-bar-enter-from,
.tt-bar-leave-to {
  opacity: 0;
  transform: translate(-50%, 8px);
}
.tt-merge {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tt-merge-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  cursor: pointer;
}
.tt-merge-row:hover {
  background: var(--t-hover);
}
.tt-merge-row.on {
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
}
.tt-radio {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  box-shadow: inset 0 0 0 2px var(--t-text3);
}
.tt-merge-row.on .tt-radio {
  box-shadow: inset 0 0 0 4px var(--t-primary);
}
@media (max-width: 768px) {
  .tt-tools {
    grid-template-columns: minmax(0, 1fr);
  }
  .tt-head {
    display: none;
  }
  .tt-row {
    grid-template-columns: 26px minmax(0, 1fr) 40px 32px 28px;
  }
  .tt-src {
    display: none;
  }
}
</style>
