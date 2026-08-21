<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { NDrawer, NDrawerContent, NButton, NIcon, NSpin } from 'naive-ui'
import { OpenOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import { useResponsive } from '@/composables/useResponsive'
import HelpArticle from './HelpArticle.vue'

// Contextual help (#2794): the article a «?» asked for, in a panel over the
// screen the reader is already on — no navigation, nothing lost. Mounted once in
// AppLayout; every HelpHint drives it through the help store.
const help = useHelpStore()
const router = useRouter()
const { isMobile } = useResponsive()

// Full width on a phone — 460px of drawer over a 390px viewport is just a
// crooked page.
const width = computed(() => (isMobile.value ? '100%' : 460))

function openFull() {
  const slug = help.drawerSlug
  help.closeDrawer()
  router.push(`/help/${slug}`)
}
</script>

<template>
  <n-drawer v-model:show="help.drawerShown" :width="width" placement="right">
    <n-drawer-content
      :title="help.drawerMeta?.title || 'Справка'"
      closable
      :native-scrollbar="false"
    >
      <n-spin v-if="help.drawerLoading && !help.drawerBody" size="small" />
      <p v-else-if="help.drawerError" class="hd-error">{{ help.drawerError }}</p>
      <HelpArticle
        v-else
        :source="help.drawerBody"
        inline
        @open-slug="help.openDrawer($event)"
        @navigate="help.closeDrawer()"
      />

      <template #footer>
        <n-button size="small" tertiary @click="openFull">
          <template #icon>
            <n-icon :component="OpenOutline" />
          </template>
          Открыть в справке
        </n-button>
      </template>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.hd-error {
  color: var(--t-text3);
  font-size: 13px;
}
/* The article component caps itself at 74ch for the three-column help page; in a
   460px drawer that cap never binds, but the images must not overflow. */
.hd-error,
:deep(.h-article) {
  max-width: 100%;
}
</style>
