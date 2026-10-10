<script setup>
// «Группы» of the members window (#3074): named sets of members — «Фронтенд»,
// «Бэкенд» — that the assignee picker and the editor's @-popup offer as one
// pick (it expands into the people; groups grant nothing).
//
// Managers edit in place, like the rows of «Команды пространства»: N (or the
// field on top) names a new group, a click on a name renames it, the row's
// «Состав» opens its people as toggles right under it, ⋯ → «Удалить» deletes at
// once with an undo toast. Everyone else reads the list.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  CheckmarkOutline,
  ChevronDownOutline,
  EllipsisHorizontal,
  PeopleCircleOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { workspaces as wsApi, memberGroups as groupsApi } from '@/api'
import { useMessage } from '@/composables/useMessage'
import { useUndoStore } from '@/stores/undo'
import { useMembersStore } from '@/stores/members'
import { groupMembers } from '@/utils/memberActivity'
import { PICKER_COLORS } from '@/utils/pickerColors'
import { hueGrad } from '@/utils/gradient'
import { humanizeError } from '@/utils/errors'
import { CAPS } from '@/utils/keyCaps'
import ActionMenu from '../kit/ActionMenu.vue'
import InlineEdit from '../kit/InlineEdit.vue'
import UserAvatar from '../UserAvatar.vue'
import Kbd from '../Kbd.vue'

const props = defineProps({
  wsId: { type: String, required: true },
  members: { type: Array, default: () => [] },
  canManage: { type: Boolean, default: false },
})
const { t } = useI18n()
const message = useMessage()
const undo = useUndoStore()
const store = useMembersStore()

const groups = computed(() => store.groupsFor(props.wsId))
watch(
  () => props.wsId,
  (id) => id && store.loadGroups(id, { force: true }),
  { immediate: true },
)
const reload = () => store.loadGroups(props.wsId, { force: true })

// ── a new group ──
const newName = ref('')
const newEl = ref(null)
const creating = ref(false)
const createError = ref('')
function nextColor() {
  return PICKER_COLORS[groups.value.length % PICKER_COLORS.length].hex
}
async function create() {
  const name = newName.value.trim()
  if (!name || creating.value) return
  creating.value = true
  createError.value = ''
  try {
    await wsApi.createMemberGroup(props.wsId, { name, color: nextColor(), member_ids: [] })
    newName.value = ''
    const list = await reload()
    // Straight into its people: a group is made to be filled.
    const made = list.find((g) => g.name === name)
    if (made) open.value = made.id
  } catch (e) {
    createError.value = humanizeError(e)
  } finally {
    creating.value = false
  }
}
function focusNew() {
  nextTick(() => newEl.value?.focus())
}

// ── rename ──
const renaming = ref('')
const renameError = ref('')
async function rename(g, value) {
  const name = (value || '').trim()
  if (!name || name === g.name) {
    renaming.value = ''
    return
  }
  try {
    await groupsApi.update(g.id, { name })
    renaming.value = ''
    renameError.value = ''
    await reload()
  } catch (e) {
    renameError.value = humanizeError(e)
  }
}

// ── the people of a group ──
const open = ref('')
const busy = ref('')
async function toggleMember(g, uid) {
  if (!props.canManage || busy.value) return
  const ids = new Set(g.member_ids || [])
  if (ids.has(uid)) ids.delete(uid)
  else ids.add(uid)
  busy.value = uid
  try {
    await groupsApi.update(g.id, { member_ids: [...ids] })
    await reload()
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = ''
  }
}

// ── the row menu: colour and delete ──
const menuFor = ref(null)
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => !!menuFor.value,
  set: (v) => {
    if (!v) menuFor.value = null
  },
})
const menuItems = computed(() => {
  const g = groups.value.find((x) => x.id === menuFor.value)
  if (!g) return []
  return [
    ...PICKER_COLORS.map((c) => ({
      key: `color:${c.hex}`,
      label: t(`common.colors.${c.key}`),
      dot: c.hex,
      checked: (g.color || '').toLowerCase() === c.hex,
    })),
    { gap: true },
    {
      key: 'delete',
      label: t('shell.members.groupDelete'),
      icon: TrashOutline,
      danger: true,
      caps: [CAPS.backspace],
      testid: 'member-group-delete',
    },
  ]
})
function openMenu(g, el) {
  menuAnchor.value = el
  menuFor.value = g.id
}
async function onMenu(key) {
  const g = groups.value.find((x) => x.id === menuFor.value)
  if (!g) return
  if (key.startsWith('color:')) {
    try {
      await groupsApi.update(g.id, { color: key.slice(6) })
      await reload()
    } catch (e) {
      message.error(humanizeError(e))
    }
    return
  }
  if (key === 'delete') {
    menuFor.value = null
    await remove(g)
  }
}
// Deleting is reversible: the undo makes the same group again — name, colour
// and people (a new id; nothing points at a group's id but this list).
async function remove(g) {
  try {
    await groupsApi.remove(g.id)
    await reload()
  } catch (e) {
    message.error(humanizeError(e))
    return
  }
  const wsId = props.wsId
  undo.push({
    text: { key: 'shell.members.groupDeleted', params: { name: g.name } },
    doneText: { key: 'shell.members.groupRestored', params: { name: g.name } },
    undo: async () => {
      await wsApi.createMemberGroup(wsId, {
        name: g.name,
        color: g.color,
        member_ids: g.member_ids || [],
      })
      await store.loadGroups(wsId, { force: true })
    },
  })
}

function peopleOf(g) {
  return groupMembers(g, props.members)
}
function escape() {
  if (menuFor.value) {
    menuFor.value = null
    return true
  }
  if (renaming.value) {
    renaming.value = ''
    return true
  }
  if (open.value) {
    open.value = ''
    return true
  }
  return false
}
defineExpose({ focusNew, escape })
</script>

<template>
  <div class="mg" data-testid="member-groups">
    <p class="mg-lead">{{ t('shell.members.groupsLead') }}</p>

    <label v-if="canManage" class="mg-new" :class="{ error: !!createError }">
      <n-icon :component="AddOutline" :size="15" class="mg-new-ic" />
      <input
        ref="newEl"
        v-model="newName"
        maxlength="64"
        spellcheck="false"
        :placeholder="t('shell.members.groupNew')"
        data-testid="member-group-new"
        @input="createError = ''"
        @keydown.enter.prevent="create"
        @keydown.esc.stop="((newName = ''), $event.target.blur())"
      />
      <Kbd soft>{{ CAPS.enter }}</Kbd>
    </label>
    <p v-if="createError" class="mg-err">{{ createError }}</p>

    <div class="mg-list">
      <div
        v-for="g in groups"
        :key="g.id"
        class="mg-item"
        :class="{ open: open === g.id }"
        data-testid="member-group"
      >
        <div class="mg-row">
          <span class="mg-tile" :style="{ background: hueGrad(g.color || '#7c5cff') }">
            <n-icon :component="PeopleCircleOutline" :size="15" color="#fff" />
          </span>
          <div class="mg-name">
            <InlineEdit
              v-if="renaming === g.id"
              :model-value="g.name"
              :error="renameError"
              :show-hint="false"
              @commit="(v) => rename(g, v)"
              @cancel="((renaming = ''), (renameError = ''))"
            />
            <button
              v-else
              type="button"
              class="mg-name-t"
              :disabled="!canManage"
              data-testid="member-group-name"
              @click="canManage && ((renaming = g.id), (renameError = ''))"
            >
              {{ g.name }}
            </button>
            <span class="mg-sub">{{ t('shell.members.groupCount', peopleOf(g).length) }}</span>
          </div>
          <span class="mg-faces">
            <UserAvatar
              v-for="m in peopleOf(g).slice(0, 5)"
              :key="m.user_id"
              :user-id="m.user_id"
              :name="m.name"
              class="mg-face"
            />
          </span>
          <button
            type="button"
            class="mg-btn"
            data-testid="member-group-open"
            @click="open = open === g.id ? '' : g.id"
          >
            {{ canManage ? t('shell.members.groupEdit') : t('shell.members.groupPeople') }}
            <n-icon :component="ChevronDownOutline" :size="12" class="mg-chev" />
          </button>
          <button
            v-if="canManage"
            type="button"
            class="mg-more"
            :title="t('common.action.more')"
            data-testid="member-group-menu"
            @click="openMenu(g, $event.currentTarget)"
          >
            <n-icon :component="EllipsisHorizontal" :size="16" />
          </button>
        </div>
        <div v-if="open === g.id" class="mg-people" data-testid="member-group-people">
          <template v-if="canManage">
            <button
              v-for="m in members"
              :key="m.user_id"
              type="button"
              class="mg-chip"
              :class="{ on: (g.member_ids || []).includes(m.user_id) }"
              :disabled="busy === m.user_id"
              data-testid="member-group-toggle"
              @click="toggleMember(g, m.user_id)"
            >
              <UserAvatar :user-id="m.user_id" :name="m.name" class="mg-chip-av" />
              {{ m.name || m.email }}
              <n-icon
                v-if="(g.member_ids || []).includes(m.user_id)"
                :component="CheckmarkOutline"
                :size="13"
              />
            </button>
          </template>
          <template v-else>
            <span v-for="m in peopleOf(g)" :key="m.user_id" class="mg-chip on">
              <UserAvatar :user-id="m.user_id" :name="m.name" class="mg-chip-av" />
              {{ m.name || m.email }}
            </span>
            <span v-if="!peopleOf(g).length" class="mg-sub">{{
              t('shell.members.groupEmpty')
            }}</span>
          </template>
        </div>
      </div>
      <p v-if="!groups.length" class="mg-empty">
        {{ canManage ? t('shell.members.groupsNoneManager') : t('shell.members.groupsNone') }}
      </p>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="240"
      testid="member-group-actions"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.mg-lead {
  margin: 0 0 14px;
  font-size: 13px;
  color: var(--t-text2);
}
.mg-new {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 38px;
  padding: 0 8px 0 12px;
  border-radius: var(--t-r-ctl);
  background: var(--t-sub);
}
.mg-new:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.mg-new.error {
  box-shadow: 0 0 0 2px var(--t-error);
}
.mg-new-ic {
  color: var(--t-text3);
}
.mg-new input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.mg-new input::placeholder {
  color: var(--t-placeholder);
}
.mg-err {
  margin: 6px 2px 0;
  font-size: 12px;
  color: var(--t-error);
}
.mg-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 14px;
}
.mg-item {
  border-radius: 12px;
}
.mg-item.open {
  background: var(--t-sub);
}
.mg-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 46px;
  padding: 3px 10px;
  border-radius: 12px;
}
.mg-item:not(.open) .mg-row:hover {
  background: var(--t-hover);
}
.mg-tile {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 9px;
}
.mg-name {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.mg-name-t {
  align-self: flex-start;
  max-width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  font-weight: 600;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: text;
}
.mg-name-t:disabled {
  cursor: default;
}
.mg-sub {
  font-size: 12px;
  color: var(--t-text3);
}
.mg-faces {
  display: inline-flex;
  flex: none;
}
.mg-face {
  width: 22px;
  height: 22px;
  margin-left: -6px;
  border-radius: 50%;
  box-shadow: 0 0 0 2px var(--t-surface);
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9px;
  font-weight: 700;
}
.mg-btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 28px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.mg-btn:hover {
  color: var(--t-text1);
}
.mg-item.open .mg-chev {
  transform: rotate(180deg);
}
.mg-more {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.mg-more:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.mg-people {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 4px 12px 12px 50px;
}
.mg-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px 0 4px;
  border: 0;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
span.mg-chip {
  cursor: default;
}
.mg-chip.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, transparent),
    color-mix(in srgb, var(--t-primary) 24%, transparent)
  );
  color: var(--t-text1);
}
.mg-chip-av {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 8.5px;
  font-weight: 700;
}
.mg-empty {
  margin: 18px 0;
  text-align: center;
  font-size: 13px;
  color: var(--t-text3);
}
.lw-mobile .mg-faces {
  display: none;
}
</style>
