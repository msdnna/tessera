<script setup>
// /project/<slug> — a project's own address (#3047). A project with boards goes
// straight to its first one; a project without any shows the mockup's «Пустой
// проект» (CatalogE): start from a template — «Канбан спринта», «Баг-трекер»,
// «Роадмап», the board born with its columns, view and tags — or from a blank
// board (⇧N), whose name is typed right here.
//
// The templates are the server's (handlers/board_templates.go); this page only
// names one. A template board is named after the template — renaming is a
// double click in the tree away, and asking for a name first would put a form
// in front of the one-click start the mockup draws.
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { FolderOpenOutline } from '@vicons/ionicons5'
import { projects as projApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import Kbd from '@/components/Kbd.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTourStore } from '@/stores/tour'
import { useShortcuts } from '@/composables/useShortcuts'
import { humanizeError } from '@/utils/errors'
import { CAPS } from '@/utils/keyCaps'
import { BOARD_TEMPLATES, firstBoard } from '@/utils/boardTemplates'
import { columnName } from '@/utils/defaultNames'
import '@/components/kit/kit.css'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const ws = useWorkspacesStore()
const tour = useTourStore()

const project = computed(() => ws.projects.find((p) => p.slug === route.params.projectSlug) || null)
const boards = computed(() => (project.value ? ws.boardsByProject[project.value.id] : null))
const ready = ref(false)

const boardPath = (b) => `/project/${project.value.slug}/board/${b.slug}`

watch(
  () => project.value?.id,
  async (id) => {
    ready.value = false
    if (!id) return
    try {
      await ws.loadBoards(id)
    } catch {
      // An unreachable list reads as empty; the tree shows the same.
    }
    if (project.value?.id !== id) return
    const b = firstBoard(boards.value)
    if (b) router.replace(boardPath(b))
    else ready.value = true
  },
  { immediate: true },
)

// ── create ──
const busy = ref('')
const error = ref('')
async function createBoard(body, key) {
  if (!project.value || busy.value) return
  busy.value = key
  error.value = ''
  try {
    const res = await projApi.createBoard(project.value.id, body)
    tour.noteCreated({ boardId: res.data?.id })
    await ws.loadBoards(project.value.id)
    if (res.data?.slug) router.push(boardPath(res.data))
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = ''
  }
}
function fromTemplate(tpl) {
  createBoard({ name: t(`project.boardTemplate.${tpl.key}.name`), template: tpl.key }, tpl.key)
}
const preview = (tpl) => tpl.columns.map((k) => columnName({ name_key: k, name: k })).join(' → ')

// The blank board: a name field in place, ↵ creates, Esc gives up.
const naming = ref(false)
const blankName = ref('')
const nameEl = ref(null)
async function startBlank() {
  naming.value = true
  blankName.value = ''
  await nextTick()
  nameEl.value?.focus()
}
function onNameKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    naming.value = false
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const name = blankName.value.trim()
    if (name) createBoard({ name }, 'blank')
  }
}
useShortcuts({
  'project.blankBoard': () => ready.value && !naming.value && startBlank(),
})
</script>

<template>
  <div class="pv" data-testid="project-view">
    <div v-if="!project" class="pv-card">
      <EmptyState :icon="FolderOpenOutline" :text="t('project.emptyProject.notFound')" />
    </div>
    <div v-else-if="ready" class="pv-card" data-testid="project-empty">
      <EmptyState
        :icon="FolderOpenOutline"
        tone="#0eb0a9"
        :title="t('project.emptyProject.title', { name: project.name })"
        :text="t('project.emptyProject.text')"
      >
        <div class="pv-tpls">
          <button
            v-for="tpl in BOARD_TEMPLATES"
            :key="tpl.key"
            type="button"
            class="pv-tpl"
            :disabled="!!busy"
            :title="preview(tpl)"
            :data-testid="`board-template-${tpl.key}`"
            @click="fromTemplate(tpl)"
          >
            <TesseraSpinner v-if="busy === tpl.key" :size="14" />
            <TesseraIcon v-else :name="tpl.glyph" :size="15" class="pv-tpl-ic" />
            {{ t(`project.boardTemplate.${tpl.key}.name`) }}
          </button>
        </div>
        <label v-if="naming" class="pv-name">
          <input
            ref="nameEl"
            v-model="blankName"
            class="pv-name-in"
            :placeholder="t('project.emptyProject.blankPlaceholder')"
            :disabled="busy === 'blank'"
            data-testid="board-blank-name"
            @keydown="onNameKey"
            @blur="!blankName.trim() && (naming = false)"
          />
          <TesseraSpinner v-if="busy === 'blank'" :size="14" />
          <Kbd v-else>{{ CAPS.enter }}</Kbd>
        </label>
        <button v-else type="button" class="pv-blank" data-testid="board-blank" @click="startBlank">
          {{ t('project.emptyProject.blank') }}
          <ShortcutKeys id="project.blankBoard" />
        </button>
        <p v-if="error" class="pv-error" role="alert">{{ error }}</p>
      </EmptyState>
    </div>
  </div>
</template>

<style scoped>
.pv {
  height: 100%;
  display: flex;
  padding: 0 16px 16px;
  box-sizing: border-box;
}
/* The mockup's frame: a soft --v-col wash over the board, radius 16. */
.pv-card {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 16px;
  background: color-mix(in srgb, var(--t-col) 55%, var(--t-bg));
}
.pv-tpls {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
  margin-top: 8px;
}
/* A template: 34px, radius 10, a card on the wash, the view glyph in accent. */
.pv-tpl {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 11px 0 9px;
  border: 0;
  border-radius: 10px;
  background: var(--t-col);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.pv-tpl:hover {
  background: var(--t-card-hover);
}
.pv-tpl:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.pv-tpl:disabled {
  opacity: 0.6;
  cursor: default;
}
.pv-tpl-ic {
  color: var(--t-primary);
}
.pv-blank {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
  padding: 2px 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.pv-blank:hover {
  color: var(--t-text1);
}
.pv-blank:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.pv-name {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: 280px;
  height: 34px;
  margin-top: 4px;
  padding: 0 8px 0 12px;
  box-sizing: border-box;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  box-shadow: 0 0 0 2px var(--t-primary);
}
.pv-name-in {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
}
.pv-name-in::placeholder {
  color: var(--t-placeholder);
}
.pv-error {
  margin: 6px 0 0;
  font-size: 12.5px;
  color: var(--t-error);
}
</style>
