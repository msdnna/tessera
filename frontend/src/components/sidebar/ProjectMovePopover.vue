<script setup>
// «Перенести в пространство…» (M) as a popover form at the project row (v2,
// #2969, mockup PopForm kind=move) — it replaced the transfer modal. A list of
// the other spaces (↑/↓, ↵), and who will lose access: members of this space
// that the target doesn't have (their assignments are dropped server-side).
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, useMessage } from 'naive-ui'
import { CheckmarkOutline } from '@vicons/ionicons5'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import { projects as projApi, workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { workspaceName } from '@/utils/defaultNames'
import { tileColor, tileLabel } from '@/components/rail/workspaceTile'
import { hueGrad } from '@/utils/gradient'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  project: { type: Object, required: true },
  escapeBack: { type: Boolean, default: false },
})
const emit = defineEmits(['update:show', 'escape'])
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const store = useWorkspacesStore()
const message = useMessage()

const targets = computed(() => store.list.filter((w) => w.id !== store.currentId))
const picked = ref(null)
const busy = ref(false)
const error = ref('')
const losing = ref(null) // number of people who lose access, null while unknown

watch(
  () => props.show,
  (on) => {
    if (!on) return
    picked.value = targets.value[0]?.id ?? null
    error.value = ''
  },
)

// Who loses access: members of this space missing from the target.
let seq = 0
watch(picked, async (id) => {
  losing.value = null
  if (!id) return
  const mine = ++seq
  try {
    const [here, there] = await Promise.all([wsApi.members(store.currentId), wsApi.members(id)])
    if (mine !== seq) return
    const keep = new Set((there.data || []).map((m) => m.user_id))
    losing.value = (here.data || []).filter((m) => !keep.has(m.user_id)).length
  } catch {
    if (mine === seq) losing.value = null
  }
})

const targetName = computed(() => workspaceName(targets.value.find((w) => w.id === picked.value)))
const note = computed(() => {
  if (!targets.value.length) return t('project.transfer.empty')
  const base = t('project.transfer.travels')
  if (losing.value == null || !picked.value) return base
  if (!losing.value) return base
  return `${base} ${t('project.transfer.losing', { space: targetName.value, n: losing.value })}`
})

function onListKey(e) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  e.preventDefault()
  const list = targets.value
  if (!list.length) return
  const i = list.findIndex((w) => w.id === picked.value)
  const next = (i + (e.key === 'ArrowDown' ? 1 : -1) + list.length) % list.length
  picked.value = list[next].id
}

async function submit() {
  if (!picked.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const res = await projApi.transfer(props.project.id, { workspace_id: picked.value })
    emit('update:show', false)
    const stripped = res.data?.stripped_assignees || 0
    message.success(
      stripped > 0 ? t('project.transfer.doneStripped', { n: stripped }) : t('project.transfer.done'),
    )
    // The project just left the current workspace — if a board of it is open,
    // it no longer belongs here, so return home. Then refresh the tree.
    if (route.params.projectSlug === props.project.slug) router.push('/')
    await store.refresh()
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :width="356"
    :title="t('project.transfer.titleShort', { name: project.name })"
    :subtitle="t('project.transfer.subtitle')"
    :primary="t('project.transfer.submit')"
    :busy-label="t('project.transfer.moving')"
    :busy="busy"
    :disabled="!picked"
    hint="list"
    :escape-back="escapeBack"
    testid="project-move"
    @update:show="emit('update:show', $event)"
    @submit="submit"
    @escape="emit('escape')"
  >
    <FormField :label="t('project.transfer.space')" :error="error" :message="note">
      <div
        v-if="targets.length"
        class="mv-list"
        tabindex="0"
        role="listbox"
        data-no-select
        data-testid="project-move-list"
        @keydown="onListKey"
      >
        <div
          v-for="w in targets"
          :key="w.id"
          class="mv-row"
          :class="{ on: w.id === picked }"
          role="option"
          :aria-selected="w.id === picked"
          @click="picked = w.id"
        >
          <span class="mv-ava" :style="{ background: hueGrad(tileColor(w)) }">{{ tileLabel(w) }}</span>
          <span class="mv-text">
            <span class="mv-name">{{ workspaceName(w) }}</span>
            <span v-if="w.my_role === 'owner' || w.my_role === 'admin'" class="mv-meta">{{
              t('project.transfer.youAdmin')
            }}</span>
          </span>
          <n-icon v-if="w.id === picked" :component="CheckmarkOutline" :size="16" class="grad-icon" />
        </div>
      </div>
    </FormField>
  </PopoverForm>
</template>

<style scoped>
.mv-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 240px;
  overflow-y: auto;
  padding: 4px;
  border-radius: 10px;
  background: var(--t-input-bg);
  outline: 0;
}
.mv-list:focus-visible {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.mv-row {
  position: relative;
  isolation: isolate;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 0 10px 0 6px;
  border-radius: 7px;
  cursor: pointer;
}
.mv-row:hover {
  background: var(--t-hover);
}
.mv-row.on {
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
}
.mv-ava {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
}
.mv-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.mv-name {
  font-size: 13.5px;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mv-meta {
  font-size: 11.5px;
  color: var(--t-text3);
}
</style>
