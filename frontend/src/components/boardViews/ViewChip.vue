<script setup>
// The loaded view's chip in the board's sub-toolbar (v2, #2987 — mockups
// ViewMenu / CatalogD): right after the view switch, with a dot when the board
// has moved away from the view. No view loaded — no chip. Its menu switches
// views (1–9, 0 — none), saves the board as a new one and leads to the panel.
//
// `bv` is the board's useBoardViews(), reactive.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  BookmarkOutline,
  ChevronDownOutline,
  ChevronUpOutline,
  CloseCircleOutline,
  OptionsOutline,
  SaveOutline,
} from '@vicons/ionicons5'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import { LAYOUT_ICONS, viewLayout } from '@/utils/boardViews'

const props = defineProps({
  bv: { type: Object, required: true },
})
const emit = defineEmits(['open-panel'])
const { t } = useI18n()

const open = ref(false)
const el = ref(null)

function row(v, i) {
  const loaded = props.bv.loadedId === v.id
  return {
    key: `v:${v.id}`,
    label: v.name,
    glyph: LAYOUT_ICONS[viewLayout(v)] || LAYOUT_ICONS.board,
    active: loaded,
    note: loaded && props.bv.changed ? t('board.views.changed') : '',
    noteWarn: true,
    locked: !v.editable,
    caps: i < 9 ? [String(i + 1)] : undefined,
  }
}
const items = computed(() => {
  const all = props.bv.views
  const out = []
  if (props.bv.mine.length) {
    out.push({ heading: t('board.views.mine') })
    for (const v of props.bv.mine) out.push(row(v, all.indexOf(v)))
  }
  if (props.bv.shared.length) {
    out.push({ heading: t('board.views.shared') })
    for (const v of props.bv.shared) out.push(row(v, all.indexOf(v)))
  }
  out.push(
    { gap: true },
    { key: 'none', label: t('board.views.none'), icon: CloseCircleOutline, caps: ['0'] },
    { key: 'saveAs', label: t('board.views.saveAs'), icon: SaveOutline, keys: 'views.saveAs' },
    { key: 'panel', label: t('board.views.allInPanel'), icon: OptionsOutline, keys: 'view.panel' },
  )
  return out
})

function onSelect(key) {
  if (key.startsWith('v:')) {
    const v = props.bv.views.find((x) => x.id === key.slice(2))
    if (v) props.bv.apply(v)
  } else if (key === 'none') props.bv.clear()
  else if (key === 'saveAs') {
    props.bv.requestSaveAs('')
    emit('open-panel')
  } else if (key === 'panel') emit('open-panel')
}
defineExpose({ toggle: () => (open.value = !open.value) })
</script>

<template>
  <button
    v-if="bv.loaded"
    ref="el"
    type="button"
    class="vchip"
    :class="{ open }"
    :title="t('board.views.chip')"
    data-testid="view-chip"
    @click="open = !open"
  >
    <n-icon :component="BookmarkOutline" :size="14" class="vchip-ic" />
    <span class="vchip-name">{{ bv.loaded.name }}</span>
    <span
      v-if="bv.changed"
      class="vchip-dot"
      :title="t('board.views.changed')"
      data-testid="view-chip-changed"
    />
    <n-icon :component="open ? ChevronUpOutline : ChevronDownOutline" :size="12" class="vchip-ic" />
  </button>
  <ActionMenu
    v-model:show="open"
    :anchor="() => el"
    placement="bottom-start"
    :offset="{ x: 0, y: 6 }"
    :width="300"
    :items="items"
    testid="view-chip-menu"
    @select="onSelect"
  />
</template>

<style scoped>
.vchip {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  max-width: 220px;
  padding: 0 9px 0 10px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.vchip:hover {
  background: var(--t-hover);
}
.vchip.open {
  background: var(--t-active);
}
.vchip:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.vchip-ic {
  flex: none;
  color: var(--t-text3);
}
.vchip.open .vchip-ic:first-child {
  color: var(--t-primary);
}
.vchip-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 600;
}
.vchip-dot {
  flex: 0 0 7px;
  width: 7px;
  height: 7px;
  border-radius: 4px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
}
</style>
