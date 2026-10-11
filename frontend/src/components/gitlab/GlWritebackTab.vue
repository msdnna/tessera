<script setup>
// «Обратная запись» of the GitLab window (#2975, mockup GitlabWin fGlWrite):
// pairs «event in Tessera → action in GitLab», what else a task may push
// (creating issues, subtasks as child items, attachments), the conflict policy
// and the sync schedule. A pair opens (click / Enter) into its editor in place.
//
// The policy choices other than «Спросить меня» are #3066 — drawn, not wired.
import { computed, inject, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NSelect, NSwitch } from 'naive-ui'
import {
  AddOutline,
  ArrowForwardOutline,
  EllipsisHorizontal,
  TrashOutline,
  CreateOutline,
  InformationCircleOutline,
  SyncOutline,
  LogoGitlab,
  SwapVerticalOutline,
  CheckmarkDoneOutline,
  FlagOutline,
  CalendarOutline,
  PeopleOutline,
  TimeOutline,
  TrailSignOutline,
  TextOutline,
  PricetagsOutline,
  ChatbubbleOutline,
} from '@vicons/ionicons5'
import { GITLAB_FORM } from './useGitlabForm'
import { useUndoStore } from '@/stores/undo'
import { cap, CAPS } from '@/utils/keyCaps'
import ActionMenu from '../kit/ActionMenu.vue'
import Kbd from '../Kbd.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const f = inject(GITLAB_FORM)
const { t } = useI18n()
const undo = useUndoStore()
const ro = computed(() => !f.isAdmin)

// The conflict policy (#3088): what a field changed on both sides turns into.
const POLICIES = [
  { key: 'ask', label: 'gitlab.window.wb.askMe' },
  { key: 'tessera', label: 'gitlab.window.wb.tesseraWins' },
  { key: 'gitlab', label: 'gitlab.window.wb.gitlabWins' },
]
const policyHint = computed(
  () =>
    ({
      tessera: 'gitlab.window.wb.tesseraWinsHint',
      gitlab: 'gitlab.window.wb.gitlabWinsHint',
    })[f.wbConflictPolicy] || 'gitlab.window.wb.askMeHint',
)

const TRIGGER_ICON = {
  column: SwapVerticalOutline,
  completion: CheckmarkDoneOutline,
  priority: FlagOutline,
  due: CalendarOutline,
  assignees: PeopleOutline,
  estimate: TimeOutline,
  milestone: TrailSignOutline,
  title_desc: TextOutline,
  labels: PricetagsOutline,
  comment: ChatbubbleOutline,
}

const openIdx = ref(-1)
function toggle(i) {
  openIdx.value = openIdx.value === i ? -1 : i
}

// ── keys on the list ──
const cursor = ref(-1)
const rowEls = {}
function focusRow(i) {
  cursor.value = i
  nextTick(() => rowEls[i]?.focus())
}
function remove(i) {
  if (ro.value) return
  const b = f.bindings[i]
  f.removeBinding(i)
  if (openIdx.value === i) openIdx.value = -1
  undo.push({
    text: { key: 'gitlab.window.wb.deleted' },
    doneText: { key: 'gitlab.window.wb.restored' },
    undo: async () => {
      const next = f.bindings.slice()
      next.splice(Math.min(i, next.length), 0, b)
      f.bindings = next
    },
  })
}
function onListKey(e) {
  if (e.isComposing) return
  const tag = e.target?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return
  const i = cursor.value
  const plain = !e.metaKey && !e.ctrlKey && !e.altKey
  if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && plain) {
    e.preventDefault()
    const n = f.bindings.length
    if (n) focusRow(Math.max(0, Math.min(n - 1, i + (e.key === 'ArrowDown' ? 1 : -1))))
  } else if (e.key === 'Enter' && plain && i >= 0) {
    e.preventDefault()
    toggle(i)
  } else if (e.key === ' ' && plain && i >= 0 && !ro.value) {
    e.preventDefault()
    f.bindings[i].enabled = !f.bindings[i].enabled
  } else if ((e.key === 'Backspace' || e.key === 'Delete') && plain && i >= 0) {
    e.preventDefault()
    remove(i)
  }
}

// ── the row menu ──
const menuFor = ref(-1)
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => menuFor.value >= 0,
  set: (v) => {
    if (!v) menuFor.value = -1
  },
})
const menuItems = computed(() => [
  { key: 'edit', label: t('gitlab.window.wb.edit'), icon: CreateOutline, caps: [CAPS.enter] },
  { gap: true },
  {
    key: 'delete',
    label: t('gitlab.modal.actions.remove'),
    icon: TrashOutline,
    danger: true,
    caps: [CAPS.backspace],
  },
])
function openMenu(i, el) {
  cursor.value = i
  menuAnchor.value = el
  menuFor.value = i
}
function onMenu(key) {
  const i = menuFor.value
  menuFor.value = -1
  if (key === 'edit') openIdx.value = i
  else if (key === 'delete') remove(i)
}

function addNew() {
  if (ro.value || !f.wbEnabled) return
  f.addBinding()
  openIdx.value = f.bindings.length - 1
  focusRow(openIdx.value)
}
function escape() {
  if (menuFor.value >= 0) {
    menuFor.value = -1
    return true
  }
  if (openIdx.value >= 0) {
    openIdx.value = -1
    return true
  }
  return false
}
defineExpose({ addNew, escape })
</script>

<template>
  <div class="glw gwb" data-testid="gitlab-writeback">
    <div class="glw-title">
      <h2>{{ t('gitlab.window.nav.writeback') }}</h2>
      <span class="glw-note">{{ t('gitlab.window.wb.note') }}</span>
      <span class="glw-grow" />
      <button
        v-if="!ro"
        type="button"
        class="kit-btn primary"
        :disabled="!f.wbEnabled"
        data-testid="gitlab-wb-add"
        @click="addNew"
      >
        <n-icon :component="AddOutline" :size="15" />
        {{ t('gitlab.window.wb.add') }}
        <Kbd>{{ cap('n') }}</Kbd>
      </button>
    </div>

    <div class="glw-scroll t-hoverscroll">
      <div class="glw-card gwb-master">
        <span class="glw-tile"><n-icon :component="LogoGitlab" :size="20" /></span>
        <div class="glw-main">
          <span class="gwb-master-t">{{ t('gitlab.modal.writeback.title') }}</span>
          <span class="glw-sub">{{ t('gitlab.window.wb.masterSub') }}</span>
        </div>
        <n-switch v-model:value="f.wbEnabled" :disabled="ro" data-testid="gitlab-wb-enabled" />
      </div>

      <template v-if="f.wbEnabled">
        <div class="glw-thead gwb-grid">
          <span>{{ t('gitlab.modal.actions.trigger') }}</span>
          <span />
          <span>{{ t('gitlab.modal.actions.action') }}</span>
          <span />
          <span />
        </div>
        <div class="gwb-body" tabindex="-1" @keydown="onListKey">
          <p v-if="!f.bindings.length" class="gwb-empty">{{ t('gitlab.modal.actions.empty') }}</p>
          <template v-for="(b, bi) in f.bindings" :key="bi">
            <div
              :ref="(el) => (rowEls[bi] = el)"
              class="glw-trow gwb-grid"
              :class="{ open: openIdx === bi, off: !b.enabled, ctx: menuFor === bi }"
              tabindex="-1"
              data-testid="gitlab-wb-row"
              @focus="cursor = bi"
              @click="toggle(bi)"
            >
              <span class="gwb-cell">
                <span class="glw-tile sm accent">
                  <n-icon :component="TRIGGER_ICON[b.trigger.type] || SyncOutline" :size="15" />
                </span>
                <span class="gwb-t">{{ f.bindingTriggerText(b) }}</span>
              </span>
              <n-icon :component="ArrowForwardOutline" :size="14" class="glw-arrow" />
              <span class="gwb-cell">
                <span class="glw-tile sm soft">
                  <n-icon :component="LogoGitlab" :size="15" />
                </span>
                <span class="gwb-t">{{ f.bindingActionText(b) }}</span>
              </span>
              <span class="glw-keep" @click.stop>
                <n-switch v-model:value="b.enabled" size="small" :disabled="ro" />
              </span>
              <button
                v-if="!ro"
                type="button"
                class="glw-more glw-keep"
                :title="t('common.action.more')"
                data-testid="gitlab-wb-menu"
                @click.stop="openMenu(bi, $event.currentTarget)"
              >
                <n-icon :component="EllipsisHorizontal" :size="16" />
              </button>
              <span v-else />
            </div>

            <div v-if="openIdx === bi" class="glw-edit" data-testid="gitlab-wb-edit">
              <span class="glw-edit-l">{{ t('gitlab.modal.actions.trigger') }}</span>
              <n-select
                :value="b.trigger.type"
                :options="f.triggerTypeOptions"
                size="small"
                :disabled="ro"
                @update:value="(v) => f.onBindingTrigger(b, v)"
              />
              <template v-if="b.trigger.type === 'column'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.targetColumn') }}</span>
                <n-select
                  v-model:value="b.trigger.column_id"
                  :options="f.columnIdOptions"
                  size="small"
                  :placeholder="t('gitlab.modal.actions.columnPlaceholder')"
                  :disabled="ro"
                />
              </template>
              <template v-else-if="b.trigger.type === 'priority'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.priority') }}</span>
                <n-select
                  v-model:value="b.trigger.priority"
                  :options="f.priorityQualOptions"
                  size="small"
                  :disabled="ro"
                />
              </template>
              <template v-else-if="b.trigger.type === 'completion'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.condition') }}</span>
                <n-select
                  v-model:value="b.trigger.completed"
                  :options="f.completionOptions"
                  size="small"
                  :disabled="ro"
                />
              </template>
              <template v-else-if="b.trigger.type === 'due'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.dateKind') }}</span>
                <n-select
                  v-model:value="b.trigger.date_kind"
                  :options="f.dateKindOptions"
                  size="small"
                  :disabled="ro"
                />
              </template>

              <span class="glw-edit-l">{{ t('gitlab.modal.actions.action') }}</span>
              <n-select
                :value="b.action.type"
                :options="f.actionTypeOptions"
                size="small"
                :disabled="ro"
                @update:value="(v) => f.onBindingAction(b, v)"
              />
              <template v-if="b.action.type === 'set_label'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.label') }}</span>
                <n-input
                  v-model:value="b.action.label"
                  size="small"
                  class="glw-mono"
                  :placeholder="t('gitlab.modal.actions.labelPlaceholder')"
                  :disabled="ro"
                />
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.clearPrefix') }}</span>
                <div>
                  <n-switch v-model:value="b.action.clear_prefix" size="small" :disabled="ro" />
                </div>
              </template>
              <template v-else-if="b.action.type === 'set_state'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.state') }}</span>
                <n-select
                  v-model:value="b.action.state"
                  :options="f.stateOptions"
                  size="small"
                  :disabled="ro"
                />
              </template>
              <template v-else-if="b.action.type === 'set_due'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.dateKind') }}</span>
                <n-select
                  v-model:value="b.action.date_kind"
                  :options="f.dateKindOptions"
                  size="small"
                  :disabled="ro"
                />
              </template>
              <template v-else-if="b.action.type === 'post_comment'">
                <span class="glw-edit-l">{{ t('gitlab.modal.actions.addMarker') }}</span>
                <div>
                  <n-switch v-model:value="b.action.add_marker" size="small" :disabled="ro" />
                </div>
              </template>
            </div>
          </template>
          <p class="glw-hint gwb-pane-hint">{{ t('gitlab.modal.actions.paneHint') }}</p>
        </div>
      </template>

      <!-- What else a task may push. -->
      <div class="glw-h">{{ t('gitlab.window.wb.tasks') }}</div>
      <div class="glw-card col gwb-flags">
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.writeback.create') }}</span>
          </div>
          <div class="glw-ctl"><n-switch v-model:value="f.wbCreate" :disabled="ro" /></div>
        </div>
        <div v-if="f.wbCreate" class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.writeback.fetchTemplates') }}</span>
          </div>
          <div class="glw-ctl">
            <n-switch v-model:value="f.wbFetchTemplates" size="small" :disabled="ro" />
          </div>
        </div>
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.writeback.children') }}</span>
          </div>
          <div class="glw-ctl"><n-switch v-model:value="f.wbChildren" :disabled="ro" /></div>
        </div>
        <template v-if="f.wbChildren">
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.writeback.groupLabel') }}</span>
            </div>
            <div class="glw-ctl">
              <!-- The placeholder is the backend's own default label
                   (gitlab.DefaultGroupLabel) — a value written into GitLab, so
                   both locales spell it the same. -->
              <n-input
                v-model:value="f.wbGroupLabel"
                :placeholder="t('gitlab.modal.writeback.groupLabelPlaceholder')"
                :disabled="ro"
              />
            </div>
          </div>
          <div class="glw-row">
            <div class="glw-lbl">
              <span class="glw-lbl-t">{{ t('gitlab.modal.writeback.autoGroup') }}</span>
            </div>
            <div class="glw-ctl">
              <n-switch v-model:value="f.wbAutoGroup" size="small" :disabled="ro" />
            </div>
          </div>
        </template>
        <div class="glw-row">
          <div class="glw-lbl">
            <span class="glw-lbl-t">{{ t('gitlab.modal.writeback.attachments') }}</span>
          </div>
          <div class="glw-ctl">
            <n-switch v-model:value="f.wbAttachments" size="small" :disabled="ro" />
          </div>
        </div>
        <p class="glw-hint">
          <!-- The template path stays in <code> and never translates. -->
          <i18n-t keypath="gitlab.modal.writeback.hint" scope="global">
            <template #templates>
              <code>.gitlab/issue_templates/*.md</code>
            </template>
          </i18n-t>
        </p>
        <p v-if="f.wbChildren" class="glw-hint">{{ t('gitlab.modal.writeback.childrenHint') }}</p>
      </div>

      <!-- On conflict (#3088): ask, or let one side win without asking. -->
      <div class="glw-row gwb-line">
        <div class="glw-lbl">
          <span class="glw-lbl-t">{{ t('gitlab.window.wb.onConflict') }}</span>
          <span class="glw-lbl-s">{{ t('gitlab.window.wb.onConflictSub') }}</span>
        </div>
        <div class="glw-ctl">
          <span class="glw-seg" role="radiogroup" data-testid="gitlab-conflict-policy">
            <button
              v-for="p in POLICIES"
              :key="p.key"
              type="button"
              role="radio"
              :aria-checked="f.wbConflictPolicy === p.key"
              :class="{ on: f.wbConflictPolicy === p.key }"
              :disabled="ro"
              :data-testid="`gitlab-conflict-policy-${p.key}`"
              @click="f.wbConflictPolicy = p.key"
            >
              {{ t(p.label) }}
            </button>
          </span>
          <span class="glw-hint gwb-inline">{{ t(policyHint) }}</span>
        </div>
      </div>

      <!-- The schedule: the incremental pull and the full sweep. -->
      <div class="glw-row gwb-line">
        <div class="glw-lbl">
          <span class="glw-lbl-t">{{ t('gitlab.window.wb.schedule') }}</span>
          <span class="glw-lbl-s">{{ t('gitlab.window.wb.scheduleSub') }}</span>
        </div>
        <div class="glw-ctl gwb-sched">
          <label class="gwb-sel">
            <span class="glw-lbl-s">{{ t('gitlab.modal.binding.interval') }}</span>
            <n-select
              v-model:value="f.intervalSec"
              :options="f.intervalOptions"
              size="small"
              :disabled="ro"
              data-testid="gitlab-interval"
            />
          </label>
          <label class="gwb-sel">
            <span class="glw-lbl-s">{{ t('gitlab.modal.binding.fullInterval') }}</span>
            <n-select
              v-model:value="f.fullIntervalSec"
              :options="f.fullIntervalOptions"
              size="small"
              :disabled="ro"
            />
          </label>
          <button
            type="button"
            class="kit-btn gwb-now"
            :disabled="!f.currentId || f.syncing"
            @click="f.syncNow()"
          >
            <n-icon :component="SyncOutline" :size="15" />
            {{ t('gitlab.window.wb.now') }}
            <ShortcutKeys id="gitlab.sync" joined />
          </button>
        </div>
      </div>
    </div>

    <div class="glw-info">
      <n-icon :component="InformationCircleOutline" :size="16" />
      <span>{{ t('gitlab.window.wb.notInGitlab') }}</span>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="240"
      testid="gitlab-wb-row-menu"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.gwb-master {
  margin-bottom: 14px;
}
.gwb-master-t {
  font-size: 13.5px;
  font-weight: 650;
}
.gwb-grid {
  grid-template-columns: minmax(0, 1fr) 16px minmax(0, 1fr) 40px 28px;
}
.gwb-body {
  outline: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.gwb-body .glw-trow {
  cursor: pointer;
}
.gwb-empty {
  margin: 14px 10px;
  font-size: 13px;
  color: var(--t-text3);
}
.gwb-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.gwb-t {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gwb-pane-hint {
  margin: 8px 10px 0;
}
.gwb-flags {
  gap: 2px;
}
.gwb-line {
  margin-top: 14px;
}
.gwb-inline {
  margin: 0;
}
.gwb-sched {
  align-items: flex-end;
}
.gwb-sel {
  display: flex;
  flex-direction: column;
  gap: 5px;
  width: 190px;
}
.lw-mobile .gwb-grid {
  grid-template-columns: minmax(0, 1fr) 40px 28px;
}
.lw-mobile .gwb-grid > :nth-child(2),
.lw-mobile .glw-thead.gwb-grid {
  display: none;
}
.lw-mobile .gwb-grid > :nth-child(3) {
  grid-column: 1;
  grid-row: 2;
}
.lw-mobile .gwb-now :deep(.kbd) {
  display: none;
}
</style>
