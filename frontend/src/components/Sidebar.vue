<script setup>
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NSelect,
  NButton,
  NInput,
  NModal,
  NCard,
  NScrollbar,
  NText,
  NIcon,
  useMessage,
} from 'naive-ui'
import { DocumentTextOutline, AlarmOutline, AddOutline } from '@vicons/ionicons5'
import { useWorkspacesStore } from '@/stores/workspaces'
import { workspaces as wsApi, projects as projApi } from '@/api'

const store = useWorkspacesStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()

function isActiveBoard(b) {
  return route.params.id === b.id
}

const wsOptions = computed(() => store.list.map((w) => ({ label: w.name, value: w.id })))
const expanded = ref(new Set())

const ungrouped = computed(() => store.projects.filter((p) => !p.group_id))
function projectsInGroup(gid) {
  return store.projects.filter((p) => p.group_id === gid)
}

async function onWorkspaceChange(id) {
  await store.selectWorkspace(id)
  expanded.value = new Set()
}

async function toggleProject(p) {
  const next = new Set(expanded.value)
  if (next.has(p.id)) {
    next.delete(p.id)
  } else {
    next.add(p.id)
    if (!store.boardsByProject[p.id]) await store.loadBoards(p.id)
  }
  expanded.value = next
}

function openBoard(b) {
  router.push(`/board/${b.id}`)
}

// ── minimal create modal ──
const modal = ref({ show: false, title: '', value: '', submit: null })
function promptCreate(title, submit) {
  modal.value = { show: true, title, value: '', submit }
}
async function confirmCreate() {
  const name = modal.value.value.trim()
  if (!name) return
  try {
    await modal.value.submit(name)
    modal.value.show = false
  } catch (e) {
    message.error(e.message)
  }
}

function newWorkspace() {
  promptCreate('Новое пространство', async (name) => {
    const res = await wsApi.create({ name })
    await store.loadWorkspaces()
    await store.selectWorkspace(res.data.id)
  })
}
function newGroup() {
  promptCreate('Новая группа', async (name) => {
    await wsApi.createGroup(store.currentId, { name })
    await store.selectWorkspace(store.currentId)
  })
}
function newProject(groupId = null) {
  promptCreate('Новый проект', async (name) => {
    await wsApi.createProject(store.currentId, { name, group_id: groupId })
    await store.selectWorkspace(store.currentId)
  })
}
function newBoard(project) {
  promptCreate('Новая доска', async (name) => {
    await projApi.createBoard(project.id, { name })
    if (!store.boardsByProject[project.id]) await store.loadBoards(project.id)
    else await store.loadBoards(project.id)
    expanded.value = new Set(expanded.value).add(project.id)
  })
}
</script>

<template>
  <div class="sidebar">
    <div class="brand">
      <span class="brand-mark">mt</span>
      <span class="brand-name">Tessera</span>
    </div>
    <div class="ws-switch">
      <n-select
        :value="store.currentId"
        :options="wsOptions"
        size="small"
        @update:value="onWorkspaceChange"
      />
      <n-button quaternary circle size="small" title="Новое пространство" @click="newWorkspace">
        <n-icon :component="AddOutline" />
      </n-button>
    </div>

    <nav class="nav">
      <router-link to="/notes" class="nav-link">
        <n-icon :component="DocumentTextOutline" :size="18" />
        <span>Заметки</span>
      </router-link>
      <router-link to="/reminders" class="nav-link">
        <n-icon :component="AlarmOutline" :size="18" />
        <span>Напоминания</span>
      </router-link>
    </nav>

    <n-scrollbar class="tree">
      <!-- grouped projects -->
      <div v-for="g in store.groups" :key="g.id" class="group">
        <div class="group-head">
          <n-text depth="2" strong>{{ g.name }}</n-text>
          <n-button text size="tiny" title="Проект в группе" @click="newProject(g.id)">
            <n-icon :component="AddOutline" />
          </n-button>
        </div>
        <div v-for="p in projectsInGroup(g.id)" :key="p.id" class="project-block">
          <div class="project-row" @click="toggleProject(p)">
            <span class="dot" :style="{ background: p.color || '#9ca3af' }" />
            <span class="name">{{ p.name }}</span>
          </div>
          <div v-if="expanded.has(p.id)" class="boards">
            <div
              v-for="b in store.boardsByProject[p.id] || []"
              :key="b.id"
              class="board-row"
              :class="{ active: isActiveBoard(b) }"
              @click="openBoard(b)"
            >
              {{ b.name }}
            </div>
            <n-button text size="tiny" class="add-board" @click="newBoard(p)">＋ доска</n-button>
          </div>
        </div>
      </div>

      <!-- ungrouped projects -->
      <div class="group">
        <div class="group-head">
          <n-text depth="3">Без группы</n-text>
        </div>
        <div v-for="p in ungrouped" :key="p.id" class="project-block">
          <div class="project-row" @click="toggleProject(p)">
            <span class="dot" :style="{ background: p.color || '#9ca3af' }" />
            <span class="name">{{ p.name }}</span>
          </div>
          <div v-if="expanded.has(p.id)" class="boards">
            <div
              v-for="b in store.boardsByProject[p.id] || []"
              :key="b.id"
              class="board-row"
              :class="{ active: isActiveBoard(b) }"
              @click="openBoard(b)"
            >
              {{ b.name }}
            </div>
            <n-button text size="tiny" class="add-board" @click="newBoard(p)">＋ доска</n-button>
          </div>
        </div>
      </div>
    </n-scrollbar>

    <div class="actions">
      <n-button size="small" block @click="newProject(null)">＋ Проект</n-button>
      <n-button size="small" block quaternary @click="newGroup">＋ Группа</n-button>
    </div>

    <n-modal v-model:show="modal.show">
      <n-card :title="modal.title" style="max-width: 360px" role="dialog">
        <n-input
          v-model:value="modal.value"
          placeholder="Название"
          autofocus
          @keyup.enter="confirmCreate"
        />
        <template #footer>
          <n-button type="primary" @click="confirmCreate">Создать</n-button>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 14px 4px;
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--t-primary);
  color: var(--t-on-primary);
  font-weight: 700;
  font-size: 13px;
  letter-spacing: -0.5px;
}
.brand-name {
  font-weight: 700;
  font-size: 16px;
  color: var(--t-text1);
}
.ws-switch {
  display: flex;
  gap: 6px;
  padding: 12px;
  align-items: center;
}
.nav {
  display: flex;
  flex-direction: column;
  padding: 4px 8px 8px;
}
.nav-link {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 14px;
  color: var(--t-text2);
  text-decoration: none;
}
.nav-link:hover {
  background: var(--t-hover);
}
.nav-link.router-link-active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
}
.tree {
  flex: 1;
  padding: 0 8px;
}
.group {
  margin-bottom: 12px;
}
.group-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 8px;
}
.project-row,
.board-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.project-row:hover,
.board-row:hover {
  background: rgba(128, 128, 128, 0.12);
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
.boards {
  padding-left: 16px;
}
.board-row {
  font-size: 13px;
  opacity: 0.9;
}
.board-row.active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
  opacity: 1;
}
.add-board {
  margin: 2px 0 6px 8px;
}
.actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
}
</style>
