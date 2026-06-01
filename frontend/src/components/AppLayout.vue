<script setup>
import { onMounted } from 'vue'
import { NLayout, NLayoutSider, NLayoutHeader, NLayoutContent } from 'naive-ui'
import Sidebar from './Sidebar.vue'
import Topbar from './Topbar.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'

const ws = useWorkspacesStore()
const authStore = useAuthStore()

onMounted(async () => {
  await authStore.verify()
  await ws.loadWorkspaces()
})
</script>

<template>
  <n-layout has-sider style="height: 100vh">
    <n-layout-sider bordered :width="260" content-style="padding: 0; height: 100%">
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
</template>
