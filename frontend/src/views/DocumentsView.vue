<script setup>
import { ref, computed, watch, onMounted, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NText, NTree, NIcon, NPopconfirm, NSpin, useMessage } from 'naive-ui'
import { DocumentsOutline, AddOutline, ArrowBackOutline } from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useResponsive } from '@/composables/useResponsive'
import { buildDocTree, docTreeOptions } from '@/utils/documents'

const message = useMessage()
const wsStore = useWorkspacesStore()
const route = useRoute()
const router = useRouter()
const { isMobile } = useResponsive()

const list = ref([])
const selected = ref(null)
const title = ref('')
const loading = ref(false)
const expanded = ref([])

const tree = computed(() => docTreeOptions(buildDocTree(list.value)))
const selectedKeys = computed(() => (selected.value ? [selected.value.id] : []))
const childCount = computed(() =>
  selected.value ? list.value.filter((d) => d.parent_id === selected.value.id).length : 0,
)

function renderPrefix({ option }) {
  return option.icon
    ? h('span', { class: 'doc-emoji' }, option.icon)
    : h(NIcon, { component: DocumentsOutline, size: 16 })
}

async function loadList() {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.list(wsStore.currentId)
    list.value = res.data || []
    // Keep every container open: the tree is small in D1 and a collapsed
    // ancestor would hide the document the user just opened by link.
    expanded.value = list.value.filter((d) => d.parent_id === null).map((d) => d.id)
  } catch (e) {
    message.error(e.message)
  }
}

async function select(doc) {
  if (!doc?.id) return
  loading.value = true
  try {
    const res = await docsApi.get(doc.id)
    selected.value = res.data
    title.value = res.data.title || ''
    const param = res.data.slug || res.data.id
    if (param && route.params.slug !== param) router.replace(`/documents/${param}`)
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

function onTreeSelect(keys) {
  const id = keys[0]
  if (!id) return
  select(list.value.find((d) => d.id === id))
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
      return true
    } catch {
      // 404 in this workspace — try the next one.
    }
  }
  return false
}

async function create(parentId = null) {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: 'Без названия',
      parent_id: parentId,
    })
    await loadList()
    if (parentId && !expanded.value.includes(parentId))
      expanded.value = [...expanded.value, parentId]
    await select(res.data)
  } catch (e) {
    message.error(e.message)
  }
}

async function rename() {
  const t = title.value.trim()
  if (!t || !selected.value?.id) return
  try {
    const res = await docsApi.update(selected.value.id, { title: t })
    selected.value = res.data
    await loadList()
    message.success('Сохранено')
  } catch (e) {
    message.error(e.message)
  }
}

async function remove() {
  if (!selected.value?.id) return
  try {
    // The API answers 409 for a container with children; the confirmation above
    // already told the user how many go with it, so the flag is explicit here.
    await docsApi.remove(selected.value.id, childCount.value > 0)
    selected.value = null
    router.replace('/documents')
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

function backToList() {
  selected.value = null
}

onMounted(async () => {
  await loadList()
  if (route.params.slug) {
    const ok = await resolveSlug(route.params.slug)
    if (!ok) message.error('Документ не найден')
  }
})

watch(
  () => wsStore.currentId,
  () => {
    selected.value = null
    loadList()
  },
)
</script>

<template>
  <div class="docs" :class="{ mobile: isMobile }">
    <div v-show="!isMobile || !selected" class="tree-pane">
      <n-button type="primary" block size="small" @click="create(null)">
        <template #icon><n-icon :component="AddOutline" /></template>
        Новый документ
      </n-button>
      <n-tree
        v-if="list.length"
        block-line
        :data="tree"
        :selected-keys="selectedKeys"
        :expanded-keys="expanded"
        :render-prefix="renderPrefix"
        @update:selected-keys="onTreeSelect"
        @update:expanded-keys="(k) => (expanded = k)"
      />
      <empty-state v-else :icon="DocumentsOutline" text="Документов пока нет" size="small" />
    </div>

    <div v-show="!isMobile || selected" class="doc-pane">
      <n-button
        v-if="isMobile && selected"
        quaternary
        size="small"
        class="back"
        @click="backToList"
      >
        <template #icon><n-icon :component="ArrowBackOutline" /></template>
        К списку
      </n-button>
      <n-spin v-if="loading" size="small" />
      <template v-else-if="selected">
        <n-input v-model:value="title" placeholder="Заголовок" class="title" @blur="rename" />
        <div class="meta">
          <n-text depth="3">
            Изменён {{ new Date(selected.updated_at).toLocaleString('ru-RU') }}
          </n-text>
        </div>
        <div class="editor-stub">
          <n-text depth="3">Редактор документа появится в D2</n-text>
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
          <n-button @click="create(selected.id)">
            <template #icon><n-icon :component="AddOutline" /></template>
            Вложенный документ
          </n-button>
        </div>
      </template>
      <div v-else class="placeholder">
        <n-text depth="3">Выберите документ или создайте новый</n-text>
      </div>
    </div>
  </div>
</template>

<style scoped>
.docs {
  display: flex;
  gap: 16px;
  height: 100%;
}
.tree-pane {
  width: 280px;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
}
.doc-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}
.title :deep(input) {
  font-size: 18px;
  font-weight: 600;
}
.meta {
  font-size: 12px;
}
/* Placeholder for the D2 editor: a flat neutral surface, no accent gradient —
   there is nothing here to emphasise yet. */
.editor-stub {
  flex: 1;
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px dashed var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
}
.actions {
  display: flex;
  justify-content: space-between;
}
.placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
.back {
  align-self: flex-start;
}
.doc-emoji {
  font-size: 14px;
  line-height: 1;
}
/* Mobile: one pane at a time — the tree, or the document. */
@media (max-width: 768px) {
  .docs.mobile .tree-pane,
  .docs.mobile .doc-pane {
    width: 100%;
    flex: 1;
  }
}
</style>
