<script setup>
import { NEmpty } from 'naive-ui'
</script>

<template>
  <div class="home">
    <n-empty description="Выберите доску в боковой панели или создайте проект">
      <template #extra>
        <span style="opacity: 0.6">Канбан-доски — Фаза 4</span>
      </template>
    </n-empty>
  </div>
</template>

<style scoped>
.home {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
