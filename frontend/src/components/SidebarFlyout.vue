<script setup>
import { computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NIcon, NText } from 'naive-ui'
import { FolderOutline, GridOutline } from '@vicons/ionicons5'
import { useWorkspacesStore } from '@/stores/workspaces'
import { iconComponent } from '@/utils/projectIcons'

// SidebarFlyout — recursive content for the collapsed-rail hover menu. Renders a
// group (folder + its nested groups/projects) or a project (icon + its boards).
const props = defineProps({
  node: { type: Object, required: true },
  kind: { type: String, required: true }, // 'group' | 'project'
})
const emit = defineEmits(['navigate'])

const store = useWorkspacesStore()
const router = useRouter()
const route = useRoute()

const subgroups = computed(() => (props.kind === 'group' ? store.childGroups(props.node.id) : []))
const childProjects = computed(() =>
  props.kind === 'group' ? store.projectsInGroup(props.node.id) : [],
)
const boards = computed(() =>
  props.kind === 'project' ? store.boardsByProject[props.node.id] || [] : [],
)
const iconComp = computed(() => (props.kind === 'project' ? iconComponent(props.node.icon) : null))
const initials = computed(() => (props.node.name || '?').trim().slice(0, 2).toUpperCase())

onMounted(() => {
  if (props.kind === 'project' && !store.boardsByProject[props.node.id]) {
    store.loadBoards(props.node.id)
  }
})

function openBoard(b) {
  router.push(`/board/${b.id}`)
  emit('navigate')
}
</script>

<template>
  <div class="fly">
    <!-- Project: header + its boards -->
    <template v-if="kind === 'project'">
      <div class="fly-head">
        <span class="picon" :style="{ background: node.color || 'var(--t-primary)' }">
          <n-icon v-if="iconComp" :component="iconComp" :size="12" />
          <template v-else>{{ initials }}</template>
        </span>
        <span class="fly-name">{{ node.name }}</span>
      </div>
      <button
        v-for="b in boards"
        :key="b.id"
        class="fly-board"
        :class="{ active: route.params.id === b.id }"
        @click="openBoard(b)"
      >
        <n-icon :component="GridOutline" :size="14" />
        <span class="fly-board-name">{{ b.name }}</span>
      </button>
      <n-text v-if="!boards.length" depth="3" class="fly-empty">нет досок</n-text>
    </template>

    <!-- Group: folder header + recursive children -->
    <template v-else>
      <div class="fly-head">
        <n-icon :component="FolderOutline" :size="15" class="fly-folder" />
        <span class="fly-name">{{ node.name }}</span>
      </div>
      <div class="fly-children">
        <SidebarFlyout
          v-for="g in subgroups"
          :key="g.id"
          :node="g"
          kind="group"
          @navigate="emit('navigate')"
        />
        <SidebarFlyout
          v-for="p in childProjects"
          :key="p.id"
          :node="p"
          kind="project"
          @navigate="emit('navigate')"
        />
        <n-text v-if="!subgroups.length && !childProjects.length" depth="3" class="fly-empty">
          пусто
        </n-text>
      </div>
    </template>
  </div>
</template>

<style scoped>
.fly {
  min-width: 180px;
}
.fly-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  font-weight: 600;
  color: var(--t-text1);
  font-size: 13px;
}
.fly-folder {
  color: var(--t-text2);
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 5px;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  flex: none;
}
.fly-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.fly-board {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;
  padding: 5px 8px 5px 22px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--t-text2);
}
.fly-board:hover {
  background: var(--t-hover);
}
.fly-board.active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
}
.fly-board-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.fly-children {
  padding-left: 8px;
  border-left: 1px solid var(--t-border);
  margin-left: 8px;
}
.fly-empty {
  display: block;
  font-size: 12px;
  padding: 2px 8px 2px 22px;
}
</style>
