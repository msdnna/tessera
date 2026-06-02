<script setup>
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NIcon, NButton, NInput, NPopover, NPopconfirm, NText, useMessage } from 'naive-ui'
import {
  GridOutline,
  EllipsisHorizontalOutline,
  ChevronForwardOutline,
  AddOutline,
} from '@vicons/ionicons5'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'

const props = defineProps({
  project: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()

const expanded = ref(false)
const settingsOpen = ref(false)
const addingBoard = ref(false)
const newBoardName = ref('')

// editable copies for the settings popover
const eName = ref('')
const eIcon = ref('')
const eColor = ref('')
const swatches = ['', '#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']

const boards = computed(() => store.boardsByProject[props.project.id] || [])
const initials = computed(() => (props.project.name || '?').trim().slice(0, 2).toUpperCase())

async function toggle() {
  expanded.value = !expanded.value
  if (expanded.value && !store.boardsByProject[props.project.id]) {
    await store.loadBoards(props.project.id)
  }
}

function openSettings() {
  eName.value = props.project.name
  eIcon.value = props.project.icon || ''
  eColor.value = props.project.color || ''
  settingsOpen.value = true
}
async function saveSettings() {
  try {
    await projApi.update(props.project.id, {
      name: eName.value.trim() || props.project.name,
      color: eColor.value,
      icon: eIcon.value,
      group_id: props.project.group_id || null,
    })
    settingsOpen.value = false
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}
async function remove() {
  try {
    await projApi.remove(props.project.id)
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}

async function addBoard() {
  const n = newBoardName.value.trim()
  if (!n) {
    addingBoard.value = false
    return
  }
  try {
    await projApi.createBoard(props.project.id, { name: n })
    newBoardName.value = ''
    addingBoard.value = false
    await store.loadBoards(props.project.id)
    expanded.value = true
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="project-block">
    <div class="row project-row" :style="{ paddingLeft: depth * 14 + 8 + 'px' }">
      <n-icon
        class="chev"
        :class="{ open: expanded }"
        :component="ChevronForwardOutline"
        @click="toggle"
      />
      <span
        class="picon"
        :style="{ background: project.color || 'var(--t-primary)' }"
        @click="toggle"
      >
        {{ project.icon || initials }}
      </span>
      <span class="name" @click="toggle">{{ project.name }}</span>
      <n-popover v-model:show="settingsOpen" trigger="manual" placement="right-start">
        <template #trigger>
          <n-button class="menu-btn" text size="tiny" @click="openSettings">
            <n-icon :component="EllipsisHorizontalOutline" />
          </n-button>
        </template>
        <div class="settings">
          <n-input v-model:value="eName" size="small" placeholder="Название" />
          <div class="srow">
            <n-input
              v-model:value="eIcon"
              size="small"
              maxlength="2"
              placeholder="🙂"
              style="width: 64px"
            />
            <div class="swatches">
              <button
                v-for="s in swatches"
                :key="s || 'none'"
                class="sw"
                :class="{ active: s === eColor }"
                :style="{ background: s || 'var(--t-border)' }"
                @click="eColor = s"
              />
            </div>
          </div>
          <div class="sfooter">
            <n-popconfirm @positive-click="remove">
              <template #trigger
                ><n-button text size="tiny" type="error">Удалить</n-button></template
              >
              Удалить проект со всеми досками?
            </n-popconfirm>
            <n-button type="primary" size="tiny" @click="saveSettings">Сохранить</n-button>
          </div>
        </div>
      </n-popover>
    </div>

    <div v-if="expanded" class="boards" :style="{ paddingLeft: depth * 14 + 28 + 'px' }">
      <div
        v-for="b in boards"
        :key="b.id"
        class="row board-row"
        :class="{ active: route.params.id === b.id }"
        @click="router.push(`/board/${b.id}`)"
      >
        <n-icon :component="GridOutline" :size="14" />
        <span class="name">{{ b.name }}</span>
      </div>
      <div v-if="addingBoard" class="row">
        <n-input
          v-model:value="newBoardName"
          size="tiny"
          autofocus
          placeholder="Название доски"
          @keyup.enter="addBoard"
          @blur="addBoard"
        />
      </div>
      <n-button v-else text size="tiny" class="add" @click="addingBoard = true">
        <n-icon :component="AddOutline" /> доска
      </n-button>
      <n-text v-if="!boards.length && !addingBoard" depth="3" class="empty">нет досок</n-text>
    </div>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.row:hover {
  background: var(--t-hover);
}
.menu-btn {
  opacity: 0;
}
.project-row:hover .menu-btn {
  opacity: 1;
}
.chev {
  color: var(--t-text3);
  transition: transform 0.15s;
  font-size: 12px;
}
.chev.open {
  transform: rotate(90deg);
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 5px;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  flex: none;
}
.name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
.board-row {
  font-size: 13px;
  color: var(--t-text2);
}
.board-row.active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
}
.add {
  margin: 2px 0;
}
.empty {
  font-size: 12px;
  padding: 2px 8px;
}
.settings {
  width: 220px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.srow {
  display: flex;
  gap: 8px;
  align-items: center;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
}
.sw {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}
.sw.active {
  border-color: var(--t-text1);
}
.sfooter {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
