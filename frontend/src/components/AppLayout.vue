<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NDrawer,
  NDrawerContent,
} from 'naive-ui'
import Sidebar from './Sidebar.vue'
import Topbar from './Topbar.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useResponsive } from '@/composables/useResponsive'

const ws = useWorkspacesStore()
const authStore = useAuthStore()
const { isMobile } = useResponsive()
const route = useRoute()

const drawerOpen = ref(false)

onMounted(async () => {
  await authStore.verify()
  await ws.loadWorkspaces()
})

// Close the mobile drawer on navigation.
watch(
  () => route.fullPath,
  () => (drawerOpen.value = false),
)
</script>

<template>
  <!-- Desktop: fixed sider + content -->
  <n-layout v-if="!isMobile" has-sider style="height: 100vh">
    <n-layout-sider bordered :width="264" content-style="padding: 0; height: 100%">
      <Sidebar />
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered>
        <Topbar />
      </n-layout-header>
      <n-layout-content content-style="padding: 16px" style="height: calc(100vh - 53px)">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>

  <!-- Mobile: hamburger opens the sidebar in a drawer -->
  <n-layout v-else style="height: 100vh">
    <n-layout-header bordered>
      <Topbar :mobile="true" @menu="drawerOpen = true" />
    </n-layout-header>
    <n-layout-content content-style="padding: 12px" style="height: calc(100vh - 53px)">
      <router-view />
    </n-layout-content>
    <n-drawer v-model:show="drawerOpen" :width="280" placement="left">
      <n-drawer-content body-content-style="padding: 0">
        <Sidebar />
      </n-drawer-content>
    </n-drawer>
  </n-layout>
</template>
