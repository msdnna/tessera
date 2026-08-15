<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NIcon, NInput, NPopconfirm, NSpin, NText, useMessage } from 'naive-ui'
import { AddOutline, ArrowBackOutline, DocumentsOutline } from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import DocEditor from '@/components/documents/DocEditor.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useDocAutosave } from '@/composables/useDocAutosave'
import { toDocJSON } from '@/utils/docSchema'

const message = useMessage()
const wsStore = useWorkspacesStore()
const route = useRoute()
const router = useRouter()

const list = ref([])
const selected = ref(null)
const content = ref(null)
const title = ref('')
const loading = ref(false)
// Breadcrumb trail of containers the user drilled into. The grid replaced the
// tree (review of #2726), so nesting needs a way to be walked; a tile always
// opens the document itself, and the trail is how you get back out.
const trail = ref([])

const parentId = computed(() =>
  trail.value.length ? trail.value[trail.value.length - 1].id : null,
)
const tiles = computed(() => list.value.filter((d) => (d.parent_id || null) === parentId.value))
const childCount = computed(() =>
  selected.value ? list.value.filter((d) => d.parent_id === selected.value.id).length : 0,
)
const children = computed(() =>
  selected.value ? list.value.filter((d) => d.parent_id === selected.value.id) : [],
)

// The version the editor is based on. Sent with every save so a document edited
// somewhere else answers 409 instead of being overwritten.
const version = ref(null)

const {
  saving,
  dirty,
  conflict,
  error: saveError,
  schedule: scheduleSave,
  flush: flushSave,
  cancel: cancelSave,
  resolveConflict,
} = useDocAutosave(async (json) => {
  const res = await docsApi.updateContent(selected.value.id, json, version.value)
  version.value = res.data?.updated_at || version.value
  applyPreview(selected.value.id, res.data)
  return res.data
})

function applyPreview(id, data) {
  const row = list.value.find((d) => d.id === id)
  if (!row || !data) return
  if (typeof data.preview === 'string') row.preview = data.preview
  if (data.updated_at) row.updated_at = data.updated_at
}

function fmtDate(v) {
  return v ? new Date(v).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }) : ''
}

async function loadList() {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.list(wsStore.currentId)
    list.value = res.data || []
  } catch (e) {
    message.error(e.message)
  }
}

async function open(doc) {
  if (!doc?.id) return
  await flushSave()
  loading.value = true
  try {
    const res = await docsApi.get(doc.id)
    selected.value = res.data
    title.value = res.data.title || ''
    content.value = toDocJSON(res.data.content)
    version.value = res.data.updated_at
    resolveConflict(res.data.updated_at)
    const param = res.data.slug || res.data.id
    if (param && route.params.slug !== param) router.replace(`/documents/${param}`)
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function backToGrid() {
  await flushSave()
  selected.value = null
  content.value = null
  if (route.params.slug) router.replace('/documents')
}

// Drill into a container from the editor: the trail lets the grid show its
// children, and the document itself stays reachable through the crumb.
async function drillInto(doc) {
  await backToGrid()
  trail.value = [...trail.value, { id: doc.id, title: doc.title }]
}

function crumbTo(index) {
  trail.value = index < 0 ? [] : trail.value.slice(0, index + 1)
}

async function reload() {
  if (!selected.value?.id) return
  const doc = { id: selected.value.id }
  resolveConflict(null)
  await open(doc)
  await loadList()
}

// Resolves the :slug deep link. Slugs are unique per workspace, not globally,
// so the current workspace is tried first and the user's others after — and the
// resolver's workspace_id then switches the app's scope. Without that switch the
// document opens while the rest of the UI stays pointed elsewhere (#2721).
async function resolveSlug(slug) {
  const ids = [wsStore.currentId, ...wsStore.list.map((w) => w.id)].filter(Boolean)
  for (const wsId of new Set(ids)) {
    try {
      const res = await docsApi.bySlug(wsId, slug)
      if (res.data?.workspace_id && res.data.workspace_id !== wsStore.currentId) {
        await wsStore.selectWorkspace(res.data.workspace_id)
        await loadList()
      }
      selected.value = res.data
      title.value = res.data.title || ''
      content.value = toDocJSON(res.data.content)
      version.value = res.data.updated_at
      return true
    } catch {
      // 404 in this workspace — try the next one.
    }
  }
  return false
}

async function create() {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: 'Без названия',
      parent_id: parentId.value,
    })
    await loadList()
    await open(res.data)
  } catch (e) {
    message.error(e.message)
  }
}

async function createNested() {
  if (!selected.value?.id) return
  const parent = selected.value
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: 'Без названия',
      parent_id: parent.id,
    })
    await loadList()
    await open(res.data)
    trail.value = [...trail.value, { id: parent.id, title: parent.title }]
  } catch (e) {
    message.error(e.message)
  }
}

async function rename() {
  const t = title.value.trim()
  if (!t || !selected.value?.id || t === selected.value.title) return
  try {
    const res = await docsApi.update(selected.value.id, { title: t })
    // Title changes bump updated_at server-side, so the editor's version has to
    // follow — otherwise the next autosave hits a 409 against our own rename.
    selected.value = res.data
    version.value = res.data.updated_at
    resolveConflict(res.data.updated_at)
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

async function remove() {
  if (!selected.value?.id) return
  try {
    await docsApi.remove(selected.value.id, childCount.value > 0)
    cancelSave()
    selected.value = null
    content.value = null
    router.replace('/documents')
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

async function uploadImage(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await docsApi.uploadAsset(selected.value.id, fd)
  return res.data?.url || ''
}

function onEditorChange(json) {
  content.value = json
  scheduleSave(json)
}

// The tab closing mid-debounce is the one case a promise cannot save: the
// handler warns instead, and flush() still gets its chance in the common case
// where the browser keeps the page alive long enough.
function onBeforeUnload(e) {
  if (!dirty.value) return
  flushSave()
  e.preventDefault()
  e.returnValue = ''
}

onMounted(async () => {
  window.addEventListener('beforeunload', onBeforeUnload)
  await loadList()
  if (route.params.slug) {
    const ok = await resolveSlug(route.params.slug)
    if (!ok) message.error('Документ не найден')
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  flushSave()
})

onBeforeRouteLeave(async () => {
  await flushSave()
})

watch(
  () => wsStore.currentId,
  () => {
    cancelSave()
    selected.value = null
    content.value = null
    trail.value = []
    loadList()
  },
)
</script>

<template>
  <div class="docs">
    <!-- Grid: documents as tiles with a preview, no editing surface. -->
    <template v-if="!selected">
      <div class="head">
        <div class="crumbs">
          <n-button text size="small" @click="crumbTo(-1)">Документы</n-button>
          <template v-for="(c, i) in trail" :key="c.id">
            <span class="sep">/</span>
            <n-button text size="small" @click="crumbTo(i)">{{ c.title }}</n-button>
          </template>
        </div>
        <n-button type="primary" size="small" @click="create">
          <template #icon><n-icon :component="AddOutline" /></template>
          Новый документ
        </n-button>
      </div>

      <div v-if="tiles.length" class="grid">
        <button v-for="d in tiles" :key="d.id" type="button" class="tile" @click="open(d)">
          <div class="tile-head">
            <span v-if="d.icon" class="doc-emoji">{{ d.icon }}</span>
            <n-icon v-else :component="DocumentsOutline" :size="16" />
            <span class="tile-title">{{ d.title || 'Без названия' }}</span>
          </div>
          <p class="tile-preview">
            {{ d.preview || 'Пустой документ' }}
          </p>
          <div class="tile-foot">
            <span>{{ fmtDate(d.updated_at) }}</span>
            <span v-if="list.filter((x) => x.parent_id === d.id).length">
              вложенных: {{ list.filter((x) => x.parent_id === d.id).length }}
            </span>
          </div>
        </button>
      </div>
      <empty-state v-else :icon="DocumentsOutline" text="Документов пока нет" size="small" />
    </template>

    <!-- Editor: title + working area, as asked in the review of #2726. -->
    <template v-else>
      <div class="head">
        <n-button quaternary size="small" @click="backToGrid">
          <template #icon><n-icon :component="ArrowBackOutline" /></template>
          К списку
        </n-button>
        <span class="status">
          <n-text v-if="saving" depth="3">Сохранение…</n-text>
          <n-text v-else-if="dirty" depth="3">Есть несохранённые правки</n-text>
          <n-text v-else depth="3">Все изменения сохранены</n-text>
        </span>
      </div>

      <n-alert v-if="conflict" type="warning" class="conflict">
        Документ изменён в другом месте — ваши последние правки не сохранены.
        <n-button text size="small" @click="reload">Загрузить актуальную версию</n-button>
      </n-alert>
      <n-alert v-else-if="saveError" type="error" class="conflict">
        {{ saveError }}
      </n-alert>

      <n-spin v-if="loading" size="small" />
      <template v-else>
        <n-input v-model:value="title" placeholder="Заголовок" class="title" @blur="rename" />
        <doc-editor
          :model-value="content"
          :upload-image="uploadImage"
          class="editor"
          @change="onEditorChange"
          @blur="flushSave()"
        />
        <div v-if="children.length" class="nested">
          <n-text depth="3">Вложенные документы</n-text>
          <div class="grid small">
            <button v-for="d in children" :key="d.id" type="button" class="tile" @click="open(d)">
              <div class="tile-head">
                <span v-if="d.icon" class="doc-emoji">{{ d.icon }}</span>
                <n-icon v-else :component="DocumentsOutline" :size="16" />
                <span class="tile-title">{{ d.title || 'Без названия' }}</span>
              </div>
              <p class="tile-preview">{{ d.preview || 'Пустой документ' }}</p>
            </button>
          </div>
        </div>
        <div class="actions">
          <n-popconfirm
            :positive-button-props="{ type: 'error' }"
            positive-text="Удалить"
            @positive-click="remove"
          >
            <template #trigger>
              <n-button type="error" ghost>Удалить</n-button>
            </template>
            <template v-if="childCount">
              Удалить документ вместе с вложенными ({{ childCount }})?
            </template>
            <template v-else>Удалить документ?</template>
          </n-popconfirm>
          <span class="grow" />
          <n-button v-if="childCount" quaternary @click="drillInto(selected)">
            Показать вложенные ({{ childCount }})
          </n-button>
          <n-button @click="createNested">
            <template #icon><n-icon :component="AddOutline" /></template>
            Вложенный документ
          </n-button>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.docs {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  min-height: 0;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
}
.sep {
  color: var(--t-text3);
}
.status {
  font-size: 12px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
  overflow-y: auto;
  align-content: start;
}
.grid.small {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  margin-top: 6px;
}
/* Tiles are neutral surfaces — no accent gradient: the grid is a list, and
   every card carrying the accent would leave nothing for it to emphasise. */
.tile {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 128px;
  padding: 12px;
  text-align: left;
  border: 1px solid var(--t-border);
  border-radius: 10px;
  background: var(--t-surface);
  color: var(--t-text1);
  cursor: pointer;
  transition:
    border-color 0.12s ease,
    background 0.12s ease;
}
.tile:hover {
  background: var(--t-hover);
  border-color: var(--t-primary);
}
.tile-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.tile-title {
  font-weight: 600;
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tile-preview {
  flex: 1;
  margin: 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--t-text3);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 4;
  line-clamp: 4;
  -webkit-box-orient: vertical;
}
.tile-foot {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--t-text3);
}
.title :deep(input) {
  font-size: 18px;
  font-weight: 600;
}
.editor {
  flex: 1;
  min-height: 0;
}
.conflict {
  flex: none;
}
.nested {
  flex: none;
}
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.grow {
  flex: 1;
}
.doc-emoji {
  font-size: 14px;
  line-height: 1;
}
</style>
