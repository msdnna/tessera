<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTooltip } from 'naive-ui'
import { useVersionInfo } from '@/composables/useVersionInfo'
import { useFormat } from '@/composables/useFormat'
import { useWhatsNewStore } from '@/stores/whatsNew'

// The running Web/API versions, shown low-contrast in the sidebar footer.
// A neutral element by the design language — flat, no accent gradient.
//
// mode:
//   'row'   — the compact "web x · api y" line under the user block (default);
//             hover reveals the full tooltip (commit + build date), a click
//             opens the full changelog (#2812).
//   'block' — the same detail rendered inline, for the collapsed rail's
//             avatar popover, where a hover tooltip on a popover is awkward.
defineProps({
  mode: { type: String, default: 'row' },
})

const { t } = useI18n()
const { web, api } = useVersionInfo()
const { formatDateTime } = useFormat()
// The modal itself is mounted once at the layout level, so the store is the
// channel — no emit to thread through AppLayout.
const whatsNew = useWhatsNewStore()

function fmtDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return formatDateTime(d, { year: 'numeric', month: '2-digit', day: '2-digit' })
}

// One service's detail lines for the tooltip / block.
function lines(label, info) {
  if (!info || !info.version) return null
  const out = [`${label} ${info.version}`]
  if (info.commit) out.push(t('app.version.commit', { commit: info.commit }))
  const built = fmtDate(info.builtAt)
  if (built) out.push(t('app.version.built', { date: built }))
  return out
}

// Both computed: the labels come from the catalogue, so the tooltip is
// re-rendered in the new language after a switch instead of keeping the one it
// was first built in.
const webLines = computed(() => lines(t('app.version.web'), web))
const apiLines = computed(() => lines(t('app.version.api'), api.value))
const rowText = computed(() => {
  const parts = []
  if (web.version) parts.push(`web ${web.version}`)
  if (api.value?.version) parts.push(`api ${api.value.version}`)
  return parts.join(' · ')
})
</script>

<template>
  <div v-if="mode === 'block'" class="ver-block">
    <template v-for="(group, gi) in [webLines, apiLines]" :key="gi">
      <div v-if="group" class="ver-group">
        <div v-for="(l, li) in group" :key="li" :class="li === 0 ? 'ver-head' : 'ver-sub'">
          {{ l }}
        </div>
      </div>
    </template>
  </div>

  <n-tooltip v-else placement="top" :disabled="!webLines && !apiLines">
    <template #trigger>
      <div
        class="ver-row"
        data-testid="version-badge"
        role="button"
        tabindex="0"
        :aria-label="t('app.whatsNew.historyTitle')"
        @click="whatsNew.openHistory()"
        @keydown.enter.prevent="whatsNew.openHistory()"
        @keydown.space.prevent="whatsNew.openHistory()"
      >
        {{ rowText }}
      </div>
    </template>
    <div class="ver-tip">
      <template v-for="(group, gi) in [webLines, apiLines]" :key="gi">
        <div v-if="group" class="ver-group">
          <div v-for="(l, li) in group" :key="li" :class="li === 0 ? 'ver-head' : 'ver-sub'">
            {{ l }}
          </div>
        </div>
      </template>
    </div>
  </n-tooltip>
</template>

<style scoped>
.ver-row {
  font-size: 11px;
  /* Deliberately very faint — a corner build stamp that shouldn't catch the eye
     (#2747 rework); it firms up on hover. */
  color: var(--t-text3);
  opacity: 0.5;
  line-height: 1.4;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: opacity 0.15s ease;
}
.ver-row:hover {
  opacity: 0.9;
}
.ver-tip,
.ver-block {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
}
.ver-block {
  gap: 6px;
}
.ver-head {
  font-weight: 600;
}
.ver-sub {
  opacity: 0.72;
  font-size: 11px;
}
.ver-block .ver-head {
  color: var(--t-text2);
}
.ver-block .ver-sub {
  color: var(--t-text3);
}
</style>
