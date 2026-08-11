<script setup>
import { ref, computed, h, render } from 'vue'
import { NIcon, NDropdown, NModal, NCard, NInput, useMessage } from 'naive-ui'
import { FolderOutline } from '@vicons/ionicons5'
import { PROJECT_ICONS, sanitizeIconSvg } from '@/utils/projectIcons'
import TesseraIcon from './TesseraIcon.vue'

const props = defineProps({
  icon: { type: String, default: '' },
  color: { type: String, default: '' },
  mode: { type: String, default: 'badge' }, // 'badge' (colour the box) | 'icon' (colour the glyph)
  initials: { type: String, default: '?' },
  allowUpload: { type: Boolean, default: false }, // projects: upload SVG/PNG
  fallbackFolder: { type: Boolean, default: false }, // groups: "no icon" = folder
  fallbackKanban: { type: Boolean, default: false }, // boards: "no icon" = kanban glyph
  transparentDefault: { type: Boolean, default: false }, // groups: empty ≡ transparent
})
const emit = defineEmits(['update:icon', 'update:color', 'update:mode'])
const message = useMessage()

const COLORS = [
  '#7c5cff',
  '#2f80ed',
  '#0eb0a9',
  '#18a058',
  '#f0a020',
  '#e0533d',
  '#eb2f96',
  '#9aa0aa',
]
const swatches = computed(() => {
  // In "icon" mode the badge box is transparent regardless, so the explicit
  // "transparent" swatch is meaningless — drop it.
  if (props.mode === 'icon') return props.transparentDefault ? [...COLORS] : ['', ...COLORS]
  return props.transparentDefault ? ['transparent', ...COLORS] : ['', 'transparent', ...COLORS]
})
function swatchActive(s) {
  if (s === props.color) return true
  return props.transparentDefault && s === 'transparent' && !props.color
}
// Same-hue diagonal gradient for a colour swatch (flat fallback for "default").
function swatchBg(s) {
  if (!s) return 'var(--t-border)'
  return `linear-gradient(to top right, color-mix(in srgb, ${s} 86%, #000), ${s} 50%, color-mix(in srgb, ${s} 86%, #fff))`
}

const addIconOptions = [
  { label: 'Найти иконку', key: 'search' },
  { label: 'Загрузить SVG / PNG', key: 'upload' },
]
const iconFileInput = ref(null)
const iconSearchShow = ref(false)
const iconQuery = ref('')
const allIcons = ref([])
const iconsLoading = ref(false)
const MAX_ICON = 40 * 1024

function onAddMenu(key) {
  if (key === 'search') openIconSearch()
  else if (key === 'upload') iconFileInput.value?.click?.()
}
async function openIconSearch() {
  iconSearchShow.value = true
  if (allIcons.value.length) return
  iconsLoading.value = true
  try {
    const mod = await import('virtual:icon-catalog')
    allIcons.value = Object.entries(mod)
      .filter(([name, c]) => name !== 'default' && c)
      .map(([name, comp]) => ({ name, comp }))
  } catch (e) {
    message.error(e.message)
  } finally {
    iconsLoading.value = false
  }
}
const iconResults = computed(() => {
  const q = iconQuery.value.trim().toLowerCase()
  const list = q ? allIcons.value.filter((i) => i.name.toLowerCase().includes(q)) : allIcons.value
  return list.slice(0, 90)
})
function extractSvg(comp) {
  const div = document.createElement('div')
  render(h(comp), div)
  const svg = div.querySelector('svg')
  const markup = svg ? svg.outerHTML : ''
  render(null, div)
  return markup
}
function pickIcon(comp) {
  const svg = sanitizeIconSvg(extractSvg(comp))
  if (!svg) return
  iconSearchShow.value = false
  emit('update:icon', svg)
}
function onIconFile(e) {
  const file = e.target.files && e.target.files[0]
  e.target.value = ''
  if (!file) return
  const isSvg = file.type === 'image/svg+xml' || file.name.toLowerCase().endsWith('.svg')
  const isPng = file.type === 'image/png'
  if (!isSvg && !isPng) {
    message.warning('Поддерживаются только SVG или PNG')
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    let icon = String(reader.result || '')
    if (isSvg) icon = sanitizeIconSvg(icon)
    if (!icon || icon.length > MAX_ICON) {
      message.warning('Файл повреждён или слишком большой (макс. 40 КБ)')
      return
    }
    emit('update:icon', icon)
  }
  if (isSvg) reader.readAsText(file)
  else reader.readAsDataURL(file)
}
</script>

<template>
  <div class="icp">
    <div class="icons">
      <button
        class="ic"
        :class="{ active: !icon }"
        :title="
          fallbackFolder
            ? 'Папка (по умолчанию)'
            : fallbackKanban
              ? 'Канбан (по умолчанию)'
              : 'Инициалы'
        "
        @click="emit('update:icon', '')"
      >
        <n-icon v-if="fallbackFolder" :component="FolderOutline" :size="16" />
        <TesseraIcon v-else-if="fallbackKanban" name="layout-kanban" :size="16" />
        <template v-else>{{ initials }}</template>
      </button>
      <button
        v-for="i in PROJECT_ICONS"
        :key="i.key"
        class="ic"
        :class="{ active: icon === i.key }"
        @click="emit('update:icon', i.key)"
      >
        <n-icon :component="i.component" :size="16" />
      </button>
      <n-dropdown v-if="allowUpload" trigger="click" :options="addIconOptions" @select="onAddMenu">
        <button class="ic ic-more" title="Ещё иконки: поиск или загрузка">＋</button>
      </n-dropdown>
      <button v-else class="ic ic-more" title="Найти иконку" @click="openIconSearch">＋</button>
    </div>

    <div class="swatches">
      <button
        v-for="s in swatches"
        :key="s || 'none'"
        class="sw"
        :class="{ active: swatchActive(s), 'sw-bare': s === 'transparent' }"
        :style="s === 'transparent' ? {} : { backgroundImage: swatchBg(s) }"
        :title="s === 'transparent' ? 'Без фона' : ''"
        @click="emit('update:color', s)"
      />
    </div>

    <!-- Where the colour lands: the badge box (default) or the glyph itself. -->
    <div class="mode-toggle" role="group" aria-label="Что красить">
      <button
        class="mt-opt"
        :class="{ active: mode !== 'icon' }"
        @click="emit('update:mode', 'badge')"
      >
        Бейдж
      </button>
      <button
        class="mt-opt"
        :class="{ active: mode === 'icon' }"
        @click="emit('update:mode', 'icon')"
      >
        Иконка
      </button>
    </div>

    <input
      v-if="allowUpload"
      ref="iconFileInput"
      type="file"
      accept="image/svg+xml,image/png"
      hidden
      @change="onIconFile"
    />
    <n-modal v-model:show="iconSearchShow">
      <n-card title="Иконка из коллекции" style="max-width: 440px" role="dialog">
        <n-input
          v-model:value="iconQuery"
          placeholder="Поиск по названию (англ.): home, code, rocket…"
          clearable
        />
        <div v-if="iconsLoading" class="icp-hint">Загрузка коллекции…</div>
        <div v-else-if="!iconResults.length" class="icp-hint">Ничего не найдено</div>
        <div v-else class="icp-grid">
          <button
            v-for="i in iconResults"
            :key="i.name"
            class="ic"
            :title="i.name"
            @click="pickIcon(i.comp)"
          >
            <n-icon :component="i.comp" :size="18" />
          </button>
        </div>
      </n-card>
    </n-modal>
  </div>
</template>

<style scoped>
.icp {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.icons {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
}
.ic {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  line-height: 1;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
}
.ic.active {
  border-color: var(--t-primary);
  color: var(--t-primary);
}
.ic-more {
  font-size: 18px;
  font-weight: 400;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
}
.sw {
  appearance: none;
  -webkit-appearance: none;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 2px solid transparent;
  /* Gradient must span the whole circle incl. the transparent border, else it
     repeats in the border ring and shows a square-ish seam inside the circle. */
  background-origin: border-box;
  cursor: pointer;
}
.sw.active {
  border-color: var(--t-text1);
}
.sw-bare {
  background-color: var(--t-surface);
  background-image:
    linear-gradient(
      45deg,
      var(--t-border) 25%,
      transparent 25%,
      transparent 75%,
      var(--t-border) 75%
    ),
    linear-gradient(
      45deg,
      var(--t-border) 25%,
      transparent 25%,
      transparent 75%,
      var(--t-border) 75%
    );
  background-size: 10px 10px;
  background-position:
    0 0,
    5px 5px;
}
/* Segmented "what to colour" toggle — badge box vs the glyph. */
.mode-toggle {
  display: flex;
  gap: 0;
  align-self: center;
  border: 1px solid var(--t-border);
  border-radius: 7px;
  overflow: hidden;
}
.mt-opt {
  appearance: none;
  border: none;
  background: var(--t-surface);
  color: var(--t-text2);
  font-size: 12px;
  padding: 4px 14px;
  cursor: pointer;
}
.mt-opt + .mt-opt {
  border-left: 1px solid var(--t-border);
}
.mt-opt.active {
  background: var(--t-accent-grad-subtle);
  color: var(--t-on-primary);
}
.icp-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  gap: 6px;
  max-height: 320px;
  overflow-y: auto;
  margin-top: 12px;
}
.icp-hint {
  margin-top: 12px;
  font-size: 13px;
  color: var(--t-text3);
  text-align: center;
}
</style>
