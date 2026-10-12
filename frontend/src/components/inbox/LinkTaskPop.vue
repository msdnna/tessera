<script setup>
// «L — привязать к задаче» (#2997): a tier-1 form at the record — a number or a
// few words of the title, the matches below, ↑↓ and ↵ to tie.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { workspaces as wsApi } from '@/api'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import { taskStatus } from '@/components/palette/rows'
import { useWorkspacesStore } from '@/stores/workspaces'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
})
const emit = defineEmits(['update:show', 'pick'])
const { t } = useI18n()
const ws = useWorkspacesStore()

const q = ref('')
const rows = ref([])
const active = ref(0)
const busy = ref(false)
let seq = 0
let timer = 0

watch(
  () => props.show,
  (on) => {
    if (!on) return
    q.value = ''
    rows.value = []
    active.value = 0
  },
)
watch(q, (v) => {
  clearTimeout(timer)
  const term = v.trim().replace(/^[/#]/, '')
  if (!term || !ws.currentId) {
    rows.value = []
    return
  }
  const mine = ++seq
  timer = setTimeout(async () => {
    busy.value = true
    try {
      const { data } = await wsApi.search(ws.currentId, term)
      if (mine !== seq) return
      rows.value = (data?.tasks || []).slice(0, 6)
      active.value = 0
    } catch {
      if (mine === seq) rows.value = []
    } finally {
      if (mine === seq) busy.value = false
    }
  }, 180)
})

const chosen = computed(() => rows.value[active.value] || null)
function pick(task = chosen.value) {
  if (!task) return
  emit('pick', task)
  emit('update:show', false)
}
function onKey(e) {
  const n = rows.value.length
  if (!n) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = (active.value + 1) % n
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = (active.value + n - 1) % n
  }
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    placement="bottom-end"
    :offset="{ x: 0, y: 4 }"
    :width="340"
    :title="t('inbox.link.title')"
    :primary="t('inbox.link.apply')"
    :disabled="!chosen"
    hint="list"
    testid="inbox-link"
    @update:show="(v) => emit('update:show', v)"
    @submit="pick()"
  >
    <input
      v-model="q"
      class="kf-input"
      :placeholder="t('inbox.link.placeholder')"
      data-testid="inbox-link-input"
      @keydown="onKey"
    />
    <div v-if="rows.length" class="lt-list" role="listbox">
      <button
        v-for="(r, i) in rows"
        :key="r.id"
        type="button"
        class="lt-row"
        :class="{ on: i === active }"
        tabindex="-1"
        @mouseenter="active = i"
        @click="pick(r)"
      >
        <TesseraIcon
          :name="taskStatus(r).name"
          :size="14"
          :style="{ color: taskStatus(r).color }"
        />
        <span class="lt-num">#{{ r.number }}</span>
        <span class="lt-title">{{ r.title }}</span>
      </button>
    </div>
    <div v-else-if="q.trim() && !busy" class="lt-none">{{ t('inbox.link.none') }}</div>
  </PopoverForm>
</template>

<style scoped>
.lt-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  margin: -6px -6px 0;
}
.lt-row {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.lt-row.on {
  background: var(--t-hover);
}
.lt-num {
  color: var(--t-text3);
}
.lt-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lt-none {
  font-size: 12.5px;
  color: var(--t-text3);
}
</style>
