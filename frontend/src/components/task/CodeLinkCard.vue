<script setup>
// One merge request in the task window's «Разработка» (#2964), drawn after the
// v2 mockup (TaskModal.dc.html): «!418 · title · status pill» over
// «source → target · pipeline». The card opens the MR in GitLab; the pipeline
// status opens the pipeline.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  GitMergeOutline,
  CheckmarkCircleOutline,
  SyncOutline,
  CloseCircleOutline,
  RemoveCircleOutline,
  PlayCircleOutline,
} from '@vicons/ionicons5'
import { changeStatus, ciMeta, branchLine, changeRef } from '@/utils/codeLinks'
import { readableHue } from '@/utils/gradient'
import { useThemeStore } from '@/stores/theme'

const props = defineProps({
  link: { type: Object, required: true },
})
const { t } = useI18n()
const theme = useThemeStore()

const CI_ICONS = {
  success: CheckmarkCircleOutline,
  running: SyncOutline,
  failed: CloseCircleOutline,
  canceled: RemoveCircleOutline,
  manual: PlayCircleOutline,
}

const status = computed(() => changeStatus(props.link))
const ci = computed(() => ciMeta(props.link.ci_status))
const branches = computed(() => branchLine(props.link))
// «!418», or «group/proj!12» for an MR of another GitLab project.
const mrRef = computed(() => changeRef(props.link))
// The mockup's tinted pill: a soft same-hue diagonal over the card surface. A
// draft is neutral — flat grey, no hue. The text is clamped for legibility on the
// active theme (the merged blue is too dark on the dark surface as is).
const pillStyle = computed(() => {
  const c = status.value.color
  if (!c) return {}
  return {
    color: readableHue(c, theme.isDark),
    background: `linear-gradient(to top right, color-mix(in srgb, ${c} 13%, var(--t-pop)), color-mix(in srgb, ${c} 22%, var(--t-pop)))`,
  }
})

function open() {
  if (props.link.url) window.open(props.link.url, '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <div
    class="mr-card"
    role="link"
    tabindex="0"
    data-testid="task-mr"
    :data-mr="link.number"
    :title="t('task.dev.openMr', { ref: mrRef })"
    @click="open"
    @keydown.enter.prevent="open"
  >
    <div class="mr-line">
      <n-icon :component="GitMergeOutline" :size="15" class="mr-ico" />
      <b class="mr-num" :title="mrRef">{{ mrRef }}</b>
      <span class="mr-title">{{ link.title }}</span>
      <span
        class="mr-pill"
        :class="{ neutral: !status.color }"
        :style="pillStyle"
        data-testid="task-mr-state"
        >{{ t(`task.dev.state.${status.key}`) }}</span
      >
    </div>
    <div v-if="branches || ci" class="mr-sub">
      <span class="mr-branches" :title="branches">{{ branches }}</span>
      <span class="grow" />
      <a
        v-if="ci"
        class="mr-ci"
        :class="`ci-${ci.key}`"
        :href="link.ci_url || undefined"
        target="_blank"
        rel="noopener noreferrer"
        data-testid="task-mr-ci"
        @click.stop
      >
        <n-icon :component="CI_ICONS[ci.icon]" :size="13" :color="ci.color" />
        <span>{{ t(`task.dev.ci.${ci.key}`) }}</span>
      </a>
    </div>
  </div>
</template>

<style scoped>
.mr-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 2px;
  padding: 10px 12px;
  border-radius: 11px;
  background: var(--t-pop);
  cursor: pointer;
  outline: none;
  transition: box-shadow 0.12s ease;
}
.mr-card:hover,
.mr-card:focus-visible {
  box-shadow: var(--t-sh-card);
}
.mr-card:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--t-primary) 55%, transparent);
  outline-offset: 1px;
}
.mr-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 13px;
  color: var(--t-text1);
}
.mr-ico {
  flex: none;
  color: var(--t-text2);
}
.mr-num {
  flex: none;
  /* Another project's path can be long: it yields to the title, never pushes the pill. */
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}
.mr-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text2);
}
.mr-pill {
  flex: none;
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 12px;
  font-size: 11.5px;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
}
.mr-pill.neutral {
  background: var(--t-pill);
  color: var(--t-text2);
}
.mr-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 12px;
  color: var(--t-text3);
}
.mr-branches {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.grow {
  flex: 1;
}
.mr-ci {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: inherit;
  text-decoration: none;
  white-space: nowrap;
}
.mr-ci[href]:hover {
  color: var(--t-text1);
}
.ci-running :deep(svg) {
  animation: mr-spin 1.6s linear infinite;
}
@keyframes mr-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .ci-running :deep(svg) {
    animation: none;
  }
}
</style>
