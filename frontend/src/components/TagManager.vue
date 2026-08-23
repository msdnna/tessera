<script setup>
import { ref, reactive, computed, watchEffect, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NInput, NButton, NText, NIcon, NPopconfirm, NSwitch, useMessage } from 'naive-ui'
import { TrashOutline } from '@vicons/ionicons5'
import { projects as projectsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { buildTagGroups } from '@/utils/tagGroups'
import { useThemeStore } from '@/stores/theme'
import TagPill from './TagPill.vue'

const { t } = useI18n()
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

function startEdit(tag) {
  editingId.value = tag.id
  nameEdit.value = tag.name
  nextTick(() => nameInput.value?.focus?.())
}
// blur/enter: save if changed, else just close (no accidental reset)
async function saveName(tag) {
  const n = nameEdit.value.trim()
  editingId.value = null
  if (!n || n === tag.name) return
  await patch(tag, { name: n })
}
async function setColor(tag, c) {
  await patch(tag, { color: c })
}
async function patch(tag, fields) {
  try {
    await projectsApi.updateTag(tag.id, { name: tag.name, color: tag.color || '', ...fields })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(tag) {
  try {
    await projectsApi.deleteTag(tag.id)
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
    <n-text depth="3" class="head">{{ t('project.tags.head') }}</n-text>
    <div class="list">
      <template v-for="g in groups" :key="g.key">
        <n-text v-if="showHeaders" depth="3" class="grp-head">{{ g.label }}</n-text>
        <!-- `tag`, not `t`: the loop variable would shadow the translate fn. -->
        <div v-for="tag in g.tags" :key="tag.id" class="tag-block">
          <div class="tag-row">
            <n-input
              v-if="editingId === tag.id"
              :ref="(el) => el && (nameInput = el)"
              v-model:value="nameEdit"
              size="tiny"
              :placeholder="t('project.tags.namePlaceholder')"
              @keyup.enter="saveName(tag)"
              @blur="saveName(tag)"
            />
            <TagPill
              v-else
              class="chip"
              :title="t('project.tags.renameHint', { name: tag.name })"
              :tag="tag"
              :prefix-names="prefixNames"
              variant="ghost"
              :scope-mode="showHeaders ? 'hide' : 'auto'"
              @dblclick="startEdit(tag)"
            />
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              :positive-text="t('common.action.delete')"
              @positive-click="remove(tag)"
            >
              <template #trigger>
                <n-button text size="tiny" type="error">
                  <n-icon :component="TrashOutline" />
                </n-button>
              </template>
              {{ t('project.tags.deleteConfirm') }}
            </n-popconfirm>
          </div>
          <div v-if="editingId === tag.id" class="swatches">
            <button
              v-for="s in swatches"
              :key="s"
              class="sw"
              :class="{ active: s === tag.color }"
              :style="{ backgroundImage: hueGrad(s) }"
              @mousedown.prevent
              @click="setColor(tag, s)"
            />
          </div>
        </div>
      </template>
      <n-text v-if="!tags.length" depth="3" class="empty">{{ t('project.tags.empty') }}</n-text>
    </div>
    <div class="add">
      <n-input
        v-model:value="newName"
        size="tiny"
        :placeholder="t('project.tags.newPlaceholder')"
        @keyup.enter="add"
      />
      <n-button type="primary" size="tiny" @click="add">{{ t('project.tags.add') }}</n-button>
    </div>

    <template v-if="prefixGroups.length">
      <div class="pfx-mode">
        <n-text depth="3" class="head pfx-head">{{ t('project.tags.prefixMode') }}</n-text>
        <n-switch
          size="small"
          :value="theme.tagPrefixMode === 'raw'"
          :title="t('project.tags.prefixModeHint')"
          @update:value="(v) => theme.setTagPrefixMode(v ? 'raw' : 'name')"
        />
      </div>
      <n-text depth="3" class="head pfx-head">{{ t('project.tags.prefixNames') }}</n-text>
      <div class="pfx-list">
        <div v-for="g in prefixGroups" :key="g.key" class="pfx-row">
          <span class="pfx-key">{{ g.prefix.trim() }}</span>
          <n-input
            v-model:value="labelEdits[g.key]"
            size="tiny"
            :placeholder="t('project.tags.prefixPlaceholder')"
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
