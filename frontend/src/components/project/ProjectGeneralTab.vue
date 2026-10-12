<script setup>
// «Общее» of the «Проект» window (#2989): the fields the sidebar's popover forms
// edit one at a time (#2969), gathered in one place — name, address, icon and
// colour, group — and the danger zone. Changes save at once: a text field on ↵
// or when it loses focus, a pick when it is picked.
import { computed, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, NSelect } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  ArchiveOutline,
  ArrowUndoOutline,
  TrashOutline,
  ReturnDownBackOutline,
} from '@vicons/ionicons5'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { humanizeError } from '@/utils/errors'
import { makeSlug } from '@/utils/slug'
import { withProjectSlug } from '@/utils/projectRoute'
import { slugTaken } from '@/utils/slugTaken'
import { countsPhrase } from '@/utils/deleteCounts'
import { useProjectArchive } from '@/composables/useProjectArchive'
import { hueGrad } from '@/utils/gradient'
import ColorIconPicker from '../kit/ColorIconPicker.vue'
import SlugSuggest from '../kit/SlugSuggest.vue'
import ProjectIcon from '../ProjectIcon.vue'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import '../kit/kit.css'

const props = defineProps({
  project: { type: Object, required: true },
})
const emit = defineEmits(['changed', 'deleted'])
const { t } = useI18n()
const message = useMessage()
const store = useWorkspacesStore()
const route = useRoute()
const router = useRouter()

const nameEl = ref(null)
const slugEl = ref(null)
const name = ref('')
const slug = ref('')
const slugError = ref('')
// A taken address (#3013): the free variant, and the address that failed — a
// blur right after the 409 must not send it again.
const slugSuggestion = ref('')
const slugFailed = ref('')
const busy = ref('')
watch(
  () => props.project,
  (p) => {
    name.value = p?.name || ''
    slug.value = p?.slug || ''
    clearSlugError()
  },
  { immediate: true },
)
function clearSlugError() {
  slugError.value = ''
  slugSuggestion.value = ''
  slugFailed.value = ''
}

// What deleting the project takes with it (#3014), named under the button.
const counts = ref(null)
watch(
  () => props.project?.id,
  async (id) => {
    counts.value = null
    if (!id) return
    try {
      const res = await projApi.counts(id)
      if (props.project?.id === id) counts.value = res.data
    } catch {
      // The generic line stays — the numbers are a courtesy, not a gate.
    }
  },
  { immediate: true },
)
const deleteHint = computed(() => {
  const what = countsPhrase(t, counts.value)
  return what
    ? t('project.window.general.deleteHintCounts', { what })
    : t('project.window.general.deleteHint')
})

// Full replace, like the sidebar's settings (handlers/projects.go UpdateProject).
async function save(patch) {
  try {
    await projApi.update(props.project.id, {
      name: props.project.name,
      color: props.project.color || '',
      icon: props.project.icon || '',
      icon_mode: props.project.icon_mode || 'badge',
      tree_mode: props.project.tree_mode || 'boards',
      group_id: props.project.group_id || null,
      ...patch,
    })
    await store.refresh()
    emit('changed')
    return true
  } catch (e) {
    message.error(humanizeError(e))
    return false
  }
}

async function commitName() {
  const n = name.value.trim()
  if (!n) {
    name.value = props.project.name
    return
  }
  if (n === props.project.name || busy.value) return
  busy.value = 'name'
  await save({ name: n })
  busy.value = ''
}

const slugPreview = computed(() => makeSlug(slug.value))
async function commitSlug() {
  const next = slugPreview.value
  if (!next || next === props.project.slug || next === slugFailed.value || busy.value) {
    if (!next) slug.value = props.project.slug
    return
  }
  busy.value = 'slug'
  clearSlugError()
  try {
    const res = await projApi.setSlug(props.project.id, next)
    // An open board of this project stays reachable under the new address.
    if (route.params.projectSlug === props.project.slug) {
      router.replace(withProjectSlug(route, res.data.slug))
    }
    await store.refresh()
    emit('changed')
  } catch (e) {
    const taken = slugTaken(e, t)
    if (taken) {
      slugError.value = taken.error
      slugSuggestion.value = taken.suggestion
      slugFailed.value = next
    } else {
      slugError.value = humanizeError(e)
    }
  } finally {
    busy.value = ''
  }
  // Back into the field (it was disabled while saving): ⌥↵ works from there.
  if (slugError.value) nextTick(() => slugEl.value?.focus())
}
// The chip or ⌥↵ in the field: the free variant goes in, ↵ or leaving the
// field saves it.
function useSuggestion() {
  if (!slugSuggestion.value) return
  slug.value = slugSuggestion.value
  clearSlugError()
  nextTick(() => slugEl.value?.focus())
}

const initials = computed(() => (props.project.name || '?').trim().slice(0, 2).toUpperCase())
const badgeStyle = computed(() =>
  props.project.icon_mode === 'icon' || props.project.color === 'transparent'
    ? { background: 'var(--t-ctl)' }
    : { background: hueGrad(props.project.color || 'var(--t-primary)') },
)
const glyph = computed(() =>
  props.project.icon_mode === 'icon' ? props.project.color || 'var(--t-primary)' : '',
)

// Groups as a flat, indented list (a group may nest in another).
const groupOptions = computed(() => {
  const groups = store.groups || []
  const kids = new Map()
  for (const g of groups) {
    const k = g.parent_id || ''
    if (!kids.has(k)) kids.set(k, [])
    kids.get(k).push(g)
  }
  const out = []
  const walk = (pid, depth) => {
    for (const g of kids.get(pid) || []) {
      out.push({ label: `${'  '.repeat(depth)}${g.name}`, value: g.id })
      walk(g.id, depth + 1)
    }
  }
  walk('', 0)
  return out
})
function pickGroup(id) {
  if (id && id !== props.project.group_id) save({ group_id: id })
}

// The archive (#3005) is reversible, so it happens at once with an undo toast;
// an archived project shows «Восстановить» in its place.
const { archiveProject, restoreProject } = useProjectArchive()
const isArchived = computed(() => !!props.project.archived_at)
async function onArchive() {
  if (!store.canManage || busy.value) return
  busy.value = 'archive'
  try {
    if (isArchived.value) await restoreProject(props.project)
    else await archiveProject(props.project)
  } finally {
    busy.value = ''
  }
}

// Deleting is irreversible: the button arms first («Точно удалить?»), the
// second press deletes. It disarms by itself after a few seconds.
const armed = ref(false)
let disarm = 0
function onDelete() {
  if (!store.canManage || busy.value) return
  if (!armed.value) {
    armed.value = true
    clearTimeout(disarm)
    disarm = setTimeout(() => (armed.value = false), 4000)
    return
  }
  clearTimeout(disarm)
  remove()
}
async function remove() {
  busy.value = 'delete'
  try {
    await projApi.remove(props.project.id)
    if (route.params.projectSlug === props.project.slug) router.push('/')
    emit('deleted')
    await store.refresh()
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = ''
    armed.value = false
  }
}

function focusFirst() {
  nextTick(() => nameEl.value?.focus())
}
function escape() {
  if (armed.value) {
    armed.value = false
    return true
  }
  return false
}
defineExpose({ focusFirst, escape })
</script>

<template>
  <div class="pg" data-testid="project-general">
    <div class="pg-title">
      <h2>{{ t('project.window.tab.general') }}</h2>
      <span class="pg-note">{{ t('project.window.general.autosave') }}</span>
    </div>

    <div class="pg-row">
      <div class="pg-lbl">
        <div class="pg-lbl-t">{{ t('project.window.general.name') }}</div>
      </div>
      <div class="pg-field pg-with-key">
        <input
          ref="nameEl"
          v-model="name"
          class="kf-input"
          :disabled="busy === 'name'"
          data-testid="project-name-input"
          @keydown.enter.prevent="($event.target.blur(), commitName())"
          @keydown.esc.stop="((name = project.name), $event.target.blur())"
          @blur="commitName"
        />
        <Kbd class="pg-enter"><n-icon :component="ReturnDownBackOutline" :size="11" /></Kbd>
      </div>
    </div>

    <div class="pg-row">
      <div class="pg-lbl">
        <div class="pg-lbl-t">{{ t('project.window.general.address') }}</div>
        <div class="pg-lbl-s">{{ t('project.window.general.addressHint') }}</div>
      </div>
      <div class="pg-field">
        <div class="pg-slug" :class="{ error: !!slugError, ro: !store.canManage }">
          <span class="pg-slug-pre">…/project/</span>
          <input
            ref="slugEl"
            v-model="slug"
            spellcheck="false"
            :disabled="!store.canManage || busy === 'slug'"
            :title="store.canManage ? '' : t('project.menu.managerOnly')"
            data-testid="project-slug-field"
            @input="clearSlugError"
            @keydown.enter.alt.prevent="useSuggestion"
            @keydown.enter.exact.prevent="$event.target.blur()"
            @keydown.esc.stop="((slug = project.slug), clearSlugError(), $event.target.blur())"
            @blur="commitSlug"
          />
        </div>
        <div v-if="slugError" class="pg-err" role="alert">{{ slugError }}</div>
        <SlugSuggest
          v-if="slugError && slugSuggestion"
          class="pg-suggest"
          :suggestion="slugSuggestion"
          @pick="useSuggestion"
        />
        <div v-if="!slugError" class="pg-help">{{ t('project.slug.aliasesHint') }}</div>
      </div>
    </div>

    <div class="pg-row">
      <div class="pg-lbl">
        <div class="pg-lbl-t">{{ t('project.window.general.look') }}</div>
        <div class="pg-lbl-s">{{ t('project.window.general.lookHint') }}</div>
      </div>
      <div class="pg-field pg-look">
        <span class="pg-big" :style="badgeStyle">
          <ProjectIcon :icon="project.icon" :initials="initials" :size="22" :color="glyph" />
        </span>
        <ColorIconPicker
          :icon="project.icon"
          :color="project.color"
          :name="project.name"
          allow-upload
          @update:icon="save({ icon: $event })"
          @update:color="save({ color: $event })"
        />
      </div>
    </div>

    <div class="pg-row">
      <div class="pg-lbl">
        <div class="pg-lbl-t">{{ t('project.window.general.group') }}</div>
      </div>
      <div class="pg-field pg-narrow">
        <n-select
          :value="project.group_id"
          :options="groupOptions"
          data-testid="project-group-select"
          @update:value="pickGroup"
        />
      </div>
    </div>

    <div class="pg-row">
      <div class="pg-lbl">
        <div class="pg-lbl-t">{{ t('project.window.general.danger') }}</div>
        <div class="pg-lbl-s">{{ t('project.window.general.dangerHint') }}</div>
      </div>
      <div class="pg-field">
        <div class="pg-danger">
          <button
            type="button"
            class="pg-btn"
            :disabled="!store.canManage || busy === 'archive'"
            :title="store.canManage ? '' : t('project.menu.managerOnly')"
            data-testid="project-archive"
            @click="onArchive"
          >
            <n-icon :component="isArchived ? ArrowUndoOutline : ArchiveOutline" :size="15" />
            {{ isArchived ? t('project.archive.restore') : t('project.menu.archive') }}
            <ShortcutKeys v-if="!isArchived" id="row.archive" />
          </button>
          <button
            type="button"
            class="pg-btn danger"
            :class="{ armed }"
            :disabled="!store.canManage || busy === 'delete'"
            :title="store.canManage ? '' : t('project.menu.managerOnly')"
            data-testid="project-delete"
            @click="onDelete"
          >
            <n-icon :component="TrashOutline" :size="15" />
            {{
              armed
                ? t('project.window.general.deleteArmed', { name: project.name })
                : t('project.menu.delete')
            }}
          </button>
        </div>
        <div class="pg-help" data-testid="project-delete-hint">{{ deleteHint }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pg {
  display: flex;
  flex-direction: column;
  gap: 26px;
  overflow-y: auto;
  padding: 6px 8px 24px 0;
}
.pg-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.pg-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.pg-note {
  font-size: 13px;
  color: var(--t-text3);
}
.pg-row {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}
.pg-lbl-t {
  font-size: 13.5px;
  font-weight: 650;
  padding-top: 9px;
}
.pg-lbl-s {
  margin-top: 4px;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text3);
}
.pg-field {
  min-width: 0;
}
.pg-narrow {
  max-width: 280px;
}
.pg-with-key {
  position: relative;
}
.pg-with-key .kf-input {
  padding-right: 40px;
}
.pg-enter {
  position: absolute;
  right: 8px;
  top: 8px;
}
.pg-slug {
  display: flex;
  align-items: center;
  height: 36px;
  padding: 0 10px;
  border-radius: 9px;
  background: var(--t-input-bg);
  font-size: 13.5px;
}
.pg-slug:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.pg-slug.error {
  box-shadow: 0 0 0 2px #e0533d;
}
.pg-slug-pre {
  color: var(--t-text3);
  flex: none;
}
.pg-slug input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  margin-left: 6px;
}
.pg-help,
.pg-suggest {
  margin-top: 6px;
}
.pg-err {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.pg-err {
  color: var(--t-error);
}
.pg-look {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px;
  border-radius: 12px;
  background: var(--t-ctl);
}
.pg-big {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 12px;
  color: #fff;
  font-size: 15px;
  font-weight: 700;
}
.pg-danger {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.pg-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  cursor: pointer;
}
.pg-btn:hover:not(:disabled) {
  background: var(--t-hover);
}
.pg-btn:disabled {
  cursor: default;
  opacity: 0.6;
}
.pg-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.pg-btn.danger :deep(svg) {
  color: var(--t-error);
}
.pg-btn.armed {
  background: color-mix(in srgb, var(--t-error) 18%, var(--t-ctl));
  color: var(--t-error);
}
@media (max-width: 768px) {
  .pg-row {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }
  .pg-lbl-t {
    padding-top: 0;
  }
}
</style>
