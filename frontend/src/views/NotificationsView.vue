<script setup>
// The page of all notifications (#3049). On a phone (#2976 — package G
// «Уведомления») the bell opens it instead of a popover, so the system Back
// leaves it like any other screen; on a wider window it is where the popover's
// «Все уведомления →» leads. Either way the rows are read from the server a
// page at a time, with the filter applied there (NotifFeed `all`).
import NotifFeed from '@/components/NotifFeed.vue'
import { useRouter } from 'vue-router'
import { useResponsive } from '@/composables/useResponsive'

const router = useRouter()
const { isMobile } = useResponsive()

function back() {
  if (window.history.state?.back) router.back()
  else router.push('/more')
}
</script>

<template>
  <div class="nv" :class="{ wide: !isMobile }" data-testid="notifications-page">
    <NotifFeed :page="isMobile" all @back="back" />
  </div>
</template>

<style scoped>
.nv {
  height: 100%;
}
.nv.wide {
  overflow: hidden;
  padding: 0 24px;
}
</style>
