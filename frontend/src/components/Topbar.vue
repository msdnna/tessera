<script setup>
import { NButton, NIcon } from 'naive-ui'
import { MenuOutline } from '@vicons/ionicons5'
import SearchBar from './SearchBar.vue'

defineProps({ mobile: { type: Boolean, default: false } })
defineEmits(['menu'])
</script>

<template>
  <div class="topbar" :class="{ mobile }">
    <n-button v-if="mobile" class="menu-btn" quaternary circle @click="$emit('menu')">
      <n-icon :component="MenuOutline" />
    </n-button>
    <div class="search-wrap">
      <SearchBar />
    </div>
  </div>
</template>

<style scoped>
.topbar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  position: relative;
}
.topbar.mobile {
  padding-left: 52px;
}
.menu-btn {
  position: absolute;
  left: 10px;
}
.search-wrap {
  width: 100%;
  max-width: 520px;
}
</style>
