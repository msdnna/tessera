<script setup>
import { useI18n } from 'vue-i18n'
import { NModal, NIcon } from 'naive-ui'
import { HelpCircleOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import { CAPS } from '@/utils/keyCaps'
import HelpCenter from './HelpCenter.vue'
import HelpSearch from './HelpSearch.vue'
import Kbd from '@/components/Kbd.vue'

const { t } = useI18n()

// The help centre lives in a window, not on a route (#2792): a question about
// how something works is asked from the board you are working on, and answering
// it must not throw away that screen. Mounted once in AppLayout; the sidebar's
// «Помощь» menu, F1 and the drawer's «Открыть в справке» drive it through the
// store. n-modal destroys its content when hidden, so the article chunk, the
// observer and the scroll position all start clean on the next open.
//
// v2 (#3107 — PackageE «HelpWin»): a tier-2 window, 1000×680, radius 18 on the
// popover level; the head carries the title, the search (/) and Esc.
const help = useHelpStore()
</script>

<template>
  <n-modal v-model:show="help.centerShown" :auto-focus="false" class="hcm-modal">
    <div class="hcm" data-help-center role="dialog" :aria-label="t('help.title')">
      <header class="hcm-head">
        <n-icon :component="HelpCircleOutline" :size="19" class="grad-icon hcm-ic" />
        <span class="hcm-title">{{ t('help.title') }}</span>
        <HelpSearch class="hcm-search" />
        <span class="hcm-spacer" />
        <button
          type="button"
          class="hcm-close"
          :title="t('help.closeHint')"
          :aria-label="t('help.close')"
          data-help-center-close
          @click="help.closeCenter()"
        >
          <Kbd>{{ CAPS.escape }}</Kbd>
        </button>
      </header>
      <div class="hcm-body">
        <HelpCenter />
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.hcm {
  display: flex;
  flex-direction: column;
  width: min(1000px, 94vw);
  height: min(680px, 88vh);
  background: var(--t-pop);
  border-radius: 18px;
  box-shadow: var(--t-sh-pop);
  overflow: hidden;
}
.hcm-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  padding: 0 14px 0 20px;
}
.hcm-ic {
  flex: none;
}
.hcm-title {
  flex: none;
  font-size: 16px;
  font-weight: 700;
  color: var(--t-text1);
  margin-right: 12px;
}
.hcm-search {
  flex: 1;
  max-width: 420px;
}
.hcm-spacer {
  flex: 1;
}
.hcm-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  padding: 0 4px;
  border: none;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
}
.hcm-close:hover {
  background: var(--t-hover);
}
.hcm-close:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* The working area owns the remaining height and does its own scrolling — the
   window must not grow with the article. */
.hcm-body {
  flex: 1;
  min-height: 0;
}
@media (max-width: 760px) {
  .hcm {
    width: 100vw;
    height: 100dvh;
    border-radius: 0;
  }
  .hcm-head {
    padding: 0 10px 0 14px;
  }
  .hcm-title {
    display: none;
  }
}
</style>
