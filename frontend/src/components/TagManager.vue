<script setup>
import { ref, reactive, computed, watchEffect, nextTick } from 'vue'
import { NInput, NButton, NText, NIcon, NPopconfirm, NSwitch, useMessage } from 'naive-ui'
import { TrashOutline } from '@vicons/ionicons5'
import { projects as projectsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { buildTagGroups } from '@/utils/tagGroups'
import { useThemeStore } from '@/stores/theme'
import TagPill from './TagPill.vue'

const theme = useThemeStore()

const props = defineProps({
  projectId: { type: String, default: null },
  tags: { type: Array, default: () => [] },
  prefixNames: { type: Object, default: () => ({}) },
})

// Tags grouped by prefix (friendly name); a single prefix-less bucket renders
// flat without a header.
const groups = computed(() => buildTagGroups(props.tags, props.prefixNames))
const showHeaders = computed(() => groups.value.length > 1)
const emit = defineEmits(['changed'])

// Friendly prefix-name editor (mig 0026 store): the prefixes that actually exist
// among this project's tags, each with an editable label. Lets non-GitLab
// projects rename prefixes too (was previously editable only via the GitLab modal).
const prefixGroups = computed(() => groups.value.filter((g) => g.prefix))
const labelEdits = reactive({}) // canonical prefix (g.key) → editable label
watchEffect(() => {
  for (const g of prefixGroups.value) {
    if (!(g.key in labelEdits)) labelEdits[g.key] = props.prefixNames[g.key] || ''
  }
})
// Save the whole merged set (preserves prefixes not currently among the tags,
// blank label clears the mapping) — mirrors GitLabModal's merge-save.
async function savePrefixes() {
  if (!props.projectId) return
  const merged = { ...props.prefixNames }
  for (const g of prefixGroups.value) {
    const label = (labelEdits[g.key] || '').trim()
    if (label) merged[g.key] = label
    else delete merged[g.key]
  }
  try {
    await projectsApi.setTagPrefixes(
      props.projectId,
      Object.entries(merged).map(([prefix, label]) => ({ prefix, label })),
    )
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

const message = useMessage()
const editingId = ref(null)
const nameEdit = ref('')
const nameInput = ref(null)
const newName = ref('')
const swatches = [
  '#7c5cff',
  '#2f80ed',
  '#0eb0a9',
  '#18a058',
  '#f0a020',
  '#e0533d',
  '#eb2f96',
  '#9aa0aa',
]

function startEdit(t) {
  editingId.value = t.id
  nameEdit.value = t.name
  nextTick(() => nameInput.value?.focus?.())
}
// blur/enter: save if changed, else just close (no accidental reset)
async function saveName(t) {
  const n = nameEdit.value.trim()
  editingId.value = null
  if (!n || n === t.name) return
  await patch(t, { name: n })
}
async function setColor(t, c) {
  await patch(t, { color: c })
}
async function patch(t, fields) {
  try {
    await projectsApi.updateTag(t.id, { name: t.name, color: t.color || '', ...fields })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(t) {
  try {
    await projectsApi.deleteTag(t.id)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function add() {
  const n = newName.value.trim()
  if (!n) return
  try {
    await projectsApi.createTag(props.projectId, { name: n, color: swatches[0] })
    newName.value = ''
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="tagmgr">
    <n-text depth="3" class="head">Теги проекта</n-text>
    <div class="list">
      <template v-for="g in groups" :key="g.key">
        <n-text v-if="showHeaders" depth="3" class="grp-head">{{ g.label }}</n-text>
        <div v-for="t in g.tags" :key="t.id" class="tag-block">
          <div class="tag-row">
            <n-input
              v-if="editingId === t.id"
              :ref="(el) => el && (nameInput = el)"
              v-model:value="nameEdit"
              size="tiny"
              placeholder="Имя тега"
              @keyup.enter="saveName(t)"
              @blur="saveName(t)"
            />
            <TagPill
              v-else
              class="chip"
              :title="`${t.name} · двойной клик — переименовать`"
              :tag="t"
              :prefix-names="prefixNames"
              variant="ghost"
              :scope-mode="showHeaders ? 'hide' : 'auto'"
              @dblclick="startEdit(t)"
            />
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              positive-text="Удалить"
              @positive-click="remove(t)"
            >
              <template #trigger>
                <n-button text size="tiny" type="error">
                  <n-icon :component="TrashOutline" />
                </n-button>
              </template>
              Удалить тег? Он снимется со всех задач.
            </n-popconfirm>
          </div>
          <div v-if="editingId === t.id" class="swatches">
            <button
              v-for="s in swatches"
              :key="s"
              class="sw"
              :class="{ active: s === t.color }"
              :style="{ backgroundImage: hueGrad(s) }"
              @mousedown.prevent
              @click="setColor(t, s)"
            />
          </div>
        </div>
      </template>
      <n-text v-if="!tags.length" depth="3" class="empty">Тегов пока нет.</n-text>
    </div>
    <div class="add">
      <n-input v-model:value="newName" size="tiny" placeholder="Новый тег" @keyup.enter="add" />
      <n-button type="primary" size="tiny" @click="add">Добавить</n-button>
    </div>

    <template v-if="prefixGroups.length">
      <div class="pfx-mode">
        <n-text depth="3" class="head pfx-head">Короткие префиксы</n-text>
        <n-switch
          size="small"
          :value="theme.tagPrefixMode === 'raw'"
          title="Показывать сырой префикс (напр. «T») вместо понятного имени"
          @update:value="(v) => theme.setTagPrefixMode(v ? 'raw' : 'name')"
        />
      </div>
      <n-text depth="3" class="head pfx-head">Имена префиксов</n-text>
      <div class="pfx-list">
        <div v-for="g in prefixGroups" :key="g.key" class="pfx-row">
          <span class="pfx-key">{{ g.prefix.trim() }}</span>
          <n-input
            v-model:value="labelEdits[g.key]"
            size="tiny"
            placeholder="напр. Статус"
            @keyup.enter="savePrefixes"
            @blur="savePrefixes"
          />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.tagmgr {
  width: 250px;
}
.head {
  display: block;
  font-size: 12px;
  margin-bottom: 8px;
}
.grp-head {
  display: block;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.75;
  margin: 6px 0 2px;
}
.grp-head:first-child {
  margin-top: 0;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 280px;
  overflow-y: auto;
  /* overflow-y:auto forces overflow-x to clip; pad so the focused rename input's
     top/left border ring isn't cut off. */
  padding: 3px;
  margin-bottom: 10px;
}
.tag-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.chip {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 10px;
  cursor: pointer;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0 4px;
  padding-left: 2px;
}
.sw {
  appearance: none;
  -webkit-appearance: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid transparent;
  background-origin: border-box;
  cursor: pointer;
}
.sw.active {
  border-color: var(--t-text1);
}
.add {
  display: flex;
  gap: 6px;
}
.empty {
  font-size: 12px;
}
.pfx-head {
  margin-top: 14px;
}
.pfx-mode {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.pfx-mode .pfx-head {
  margin-top: 14px;
}
.pfx-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  /* room for the focused input's border ring (overflow clip on the popover) */
  padding: 3px;
}
.pfx-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.pfx-key {
  flex: 0 0 auto;
  min-width: 34px;
  font-size: 12px;
  font-weight: 600;
  opacity: 0.85;
}
</style>
