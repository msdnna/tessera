<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { NIcon, NTooltip } from 'naive-ui'
import { HelpCircleOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import { helpSlugForPath } from '@/utils/helpContext'

// The «?» that opens the matching help article in a drawer over the current
// screen (#2794). With no slug it asks helpContext which article this route is
// about, so one instance in the topbar follows the reader around the app.
//
// data-help carries the resolved slug: it is the anchor the screenshot specs
// and the e2e suite click, and it makes «which article does this button open»
// visible in the DOM.
const props = defineProps({
  slug: { type: String, default: '' },
  size: { type: Number, default: 16 },
  label: { type: String, default: '' },
})

const { t } = useI18n()
const route = useRoute()
const help = useHelpStore()

const slug = computed(() => props.slug || helpSlugForPath(route.path))
// An unknown slug renders nothing rather than a button that opens «Статья не
// найдена» — a dead ? is worse than no ?.
const article = computed(() => (slug.value ? help.bySlug(slug.value) : null))
const tip = computed(
  () => props.label || t('help.hintTip', { title: article.value?.title || '' }),
)
</script>

<template>
  <n-tooltip v-if="article" placement="bottom">
    <template #trigger>
      <button
        type="button"
        class="help-hint"
        :data-help="slug"
        :aria-label="tip"
        @click.stop="help.openDrawer(slug)"
      >
        <n-icon :component="HelpCircleOutline" :size="size" />
      </button>
    </template>
    {{ tip }}
  </n-tooltip>
</template>

<style scoped>
.help-hint {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 50%;
  width: 26px;
  height: 26px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  transition:
    color 0.15s,
    background 0.15s;
}
.help-hint:hover {
  color: var(--t-primary);
  background: var(--t-hover);
}
.help-hint:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
</style>
