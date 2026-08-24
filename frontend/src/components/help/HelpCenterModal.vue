<script setup>
import { NModal, NIcon } from 'naive-ui'
import { HelpCircleOutline, CloseOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import HelpCenter from './HelpCenter.vue'

// The help centre lives in a modal, not on a route (#2792): a question about how
// something works is asked from the board you are working on, and answering it
// must not throw away that screen. Mounted once in AppLayout; the sidebar's
// «Помощь» menu and the drawer's «Открыть в справке» both drive it through the
// store. n-modal destroys its content when hidden, so the article chunk, the
// observer and the scroll position all start clean on the next open.
const help = useHelpStore()
</script>

<template>
  <n-modal v-model:show="help.centerShown" :auto-focus="false" class="hcm-modal">
    <div class="hcm" data-help-center>
      <div class="hcm-head">
        <n-icon :component="HelpCircleOutline" :size="18" class="grad-icon" />
        <span class="hcm-title">Справочный центр</span>
        <button
          type="button"
          class="hcm-close"
          title="Закрыть (Esc)"
          aria-label="Закрыть"
          data-help-center-close
          @click="help.closeCenter()"
        >
          <n-icon :component="CloseOutline" :size="18" />
        </button>
      </div>
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
  width: min(1180px, 94vw);
  height: min(820px, 88vh);
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 12px;
  overflow: hidden;
}
.hcm-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--t-border);
  flex: none;
}
.hcm-title {
  flex: 1;
  font-size: 14px;
  font-weight: 650;
  color: var(--t-text1);
}
.hcm-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--t-text2);
  border-radius: 6px;
  padding: 4px;
  cursor: pointer;
}
.hcm-close:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* The working area owns the remaining height and does its own scrolling — the
   modal shell must not grow with the article. */
.hcm-body {
  flex: 1;
  min-height: 0;
}
</style>
