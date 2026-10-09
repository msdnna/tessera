<script setup>
// The notification feed as a page (v2, #2976 — package G «Уведомления»): on a
// phone the bell opens this instead of a popover, so the system Back leaves it
// like any other screen. On a wider window there is no page of all
// notifications yet (#3049) — the address sends the reader home and opens the
// bell's popover there.
import { onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import NotifFeed from '@/components/NotifFeed.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { useResponsive } from '@/composables/useResponsive'

const router = useRouter()
const notes = useNotificationsStore()
const { isMobile } = useResponsive()

function toPopover() {
  router.replace('/')
  notes.requestFeed()
}
onMounted(() => {
  if (!isMobile.value) toPopover()
})
watch(isMobile, (m) => !m && toPopover())

function back() {
  if (window.history.state?.back) router.back()
  else router.push('/more')
}
</script>

<template>
  <div class="nv" data-testid="notifications-page">
    <NotifFeed page @back="back" />
  </div>
</template>

<style scoped>
.nv {
  height: 100%;
}
</style>
