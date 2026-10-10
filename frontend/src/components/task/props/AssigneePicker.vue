<script setup>
// «Исполнители · A» (#3118, mockup PropPicker f=asg): a search over the
// workspace's people, «Назначить себя», then «Назначены» · «Недавние» ·
// «Участники» · «GitLab», and «Снять всех» (⇧⌫). Several people at once: the
// picker stays open. Nothing found by an email offers «Пригласить» (the
// «Участники» window).
//
// Keys: typing filters; ↑↓; Space toggles the cursor row once the arrows
// picked it; ↵ with a query toggles the found person and clears the query, ↵ on
// «Назначить себя» / «Пригласить» runs them, ↵ elsewhere is «готово»; ⇧⌫ takes
// everyone off; A again (from the list) — the next field.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { PersonOutline } from '@vicons/ionicons5'
import PropPicker from '../../kit/PropPicker.vue'
import { useAuthStore } from '@/stores/auth'
import { useMembersStore } from '@/stores/members'
import { useWorkspacesStore } from '@/stores/workspaces'
import { groupToggle } from '@/utils/memberActivity'
import { hasModKey } from '@/utils/keys'
import {
  assigneeItems,
  footerFor,
  pushRecent,
  readRecent,
  RECENT_ASSIGNEES_KEY,
  WIDTHS,
} from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  members: { type: Array, default: () => [] },
  // User ids on the task, and the GitLab logins pinned to it.
  assigned: { type: Array, default: () => [] },
  gitlab: { type: Array, default: () => [] },
  glAssigned: { type: Array, default: () => [] },
  // Open tasks per user id on this board («6 в работе»).
  load: { type: Object, default: null },
  readonly: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  error: { type: Boolean, default: false },
  keyId: { type: String, default: 'task.assignees' },
  testid: { type: String, default: 'assignee-picker' },
})
const emit = defineEmits([
  'update:show',
  'toggle',
  'toggle-gl',
  'clear',
  'invite',
  'cycle',
  'retry',
])
const { t } = useI18n()
const auth = useAuthStore()
// Member groups of the open workspace (#3074): one pick, several people.
const membersStore = useMembersStore()
const wsStore = useWorkspacesStore()
const groups = computed(() => membersStore.groupsFor(wsStore.currentId))

const query = ref('')
const recent = ref(readRecent(RECENT_ASSIGNEES_KEY))
watch(
  () => props.show,
  (on) => {
    if (!on) return
    query.value = ''
    recent.value = readRecent(RECENT_ASSIGNEES_KEY)
    membersStore.loadGroups(wsStore.currentId)
  },
)
const assignedSet = computed(() => new Set(props.assigned))
// A group: everyone in it who is not on the task yet comes on; when all of them
// already are, all of them come off. One toggle per person — the same calls a
// person's row makes, so nothing new reaches the server.
function toggleGroup(g) {
  const { add, remove } = groupToggle(g, assignedSet.value, props.members)
  for (const uid of [...add, ...remove]) emit('toggle', uid)
}
function toggle(uid) {
  // The MRU is «people you actually assign».
  if (!assignedSet.value.has(uid)) recent.value = pushRecent(RECENT_ASSIGNEES_KEY, uid)
  emit('toggle', uid)
}
const built = computed(() =>
  assigneeItems({
    t,
    query: query.value,
    members: props.members,
    assigned: assignedSet.value,
    recent: recent.value,
    gitlab: props.gitlab,
    glAssigned: new Set(props.glAssigned),
    me: auth.user?.id || '',
    load: props.load,
    groups: groups.value,
    ops: {
      toggle,
      toggleGroup,
      toggleGl: (g) => emit('toggle-gl', g),
      clear: () => emit('clear'),
      invite: (email) => emit('invite', email),
    },
  }),
)
const empty = computed(() =>
  built.value.empty ? { icon: PersonOutline, ...built.value.empty } : null,
)
const footer = computed(() => {
  if (props.readonly) return footerFor('readonly', t)
  if (built.value.items.some((i) => i.id === 'invite'))
    return [{ caps: ['↵'], label: t('props.foot.invite') }, '|', { caps: ['Esc'] }]
  return footerFor('assignees', t)
})
const err = computed(() =>
  props.error ? { title: t('props.assignees.error'), sub: t('props.assignees.errorSub') } : null,
)

function onKey(e, api) {
  if (props.readonly) return false
  if (e.key === 'Backspace' && e.shiftKey && !hasModKey(e)) {
    e.preventDefault()
    emit('clear')
    return true
  }
  if (e.key === 'Enter' && !hasModKey(e) && !e.altKey) {
    e.preventDefault()
    if (props.error) {
      emit('retry')
      return true
    }
    const row = api.row
    if (api.query.trim()) {
      if (row) {
        api.pick(row)
        if (row.check === 'multi') api.query = ''
      }
      return true
    }
    if (row && !row.check) api.pick(row)
    else api.close()
    return true
  }
  return false
}
</script>

<template>
  <PropPicker
    v-model:query="query"
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="WIDTHS.assignees"
    :title="t('props.assignees.title')"
    :header="false"
    :key-id="keyId"
    search
    :placeholder="t('props.assignees.search')"
    :items="built.items"
    :empty="empty"
    :footer="footer"
    :readonly="readonly"
    :loading="loading"
    :error="err"
    cycle-key="a"
    :on-key="onKey"
    :testid="testid"
    @update:show="emit('update:show', $event)"
    @cycle="emit('cycle')"
    @retry="emit('retry')"
  />
</template>
