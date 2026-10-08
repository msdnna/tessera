<script setup>
// «Правила лейблов» of the GitLab window (#2975, mockup GitlabWin fGlRules): how
// incoming GitLab labels become a status, a priority, a board, tags — read top to
// bottom, the first rule that fits wins. Rows are edited in place; a row opens
// (Enter / click) into its details — the friendly prefix name, «keep the
// prefix», the value map. ⌥↑/⌥↓ reorder, ⌫ deletes with an undo toast.
//
// Label → stage rules, per-rule switches with counters and «Проверить лейбл»
// are #3066 — drawn, not wired.
import { computed, inject, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NSelect, NSwitch, NTooltip } from 'naive-ui'
import {
  AddOutline,
  ArrowForwardOutline,
  ReorderTwoOutline,
  EllipsisHorizontal,
  ChevronDownOutline,
  ChevronForwardOutline,
  ArrowUpOutline,
  ArrowDownOutline,
  TrashOutline,
  FlaskOutline,
  CloseOutline,
} from '@vicons/ionicons5'
import { GITLAB_FORM } from './useGitlabForm'
import { useUndoStore } from '@/stores/undo'
import { stubTask } from '@/utils/featureStubs'
import { cap, CAPS } from '@/utils/keyCaps'
import { isMac } from '@/utils/keys'
import ActionMenu from '../kit/ActionMenu.vue'
import Kbd from '../Kbd.vue'

const f = inject(GITLAB_FORM)
const { t } = useI18n()
const undo = useUndoStore()
const stub = stubTask('gitlab.rules')
const ro = computed(() => !f.isAdmin)

// ── filter by what a rule does ──
const filter = ref('all')
const ACTIONS = ['status', 'priority', 'board', 'tag', 'group', 'ignore']
const actionName = (a) => f.actionOptions.find((o) => o.value === a)?.label || a
const segs = computed(() => [
  { key: 'all', label: t('gitlab.window.rules.all'), n: f.rules.length },
  ...ACTIONS.map((a) => ({
    key: a,
    label: actionName(a),
    n: f.rules.filter((r) => r.action === a).length,
  })).filter((s) => s.n > 0),
])
const shown = computed(() =>
  f.rules
    .map((rule, i) => ({ rule, i }))
    .filter(({ rule }) => filter.value === 'all' || rule.action === filter.value),
)

// The dot by a label: the hue of what it turns into.
const TONE = {
  status: '#3b82f6',
  priority: '#f0a020',
  board: '#2fa86b',
  tag: '#a855f7',
  group: '#14b8a6',
  ignore: '#8b8b96',
}
const hasMap = (rule) => f.mapActions.includes(rule.action)
function summary(rule) {
  const parts = []
  if (rule.match_type === 'prefix' && rule.label) parts.push(`«${rule.label}»`)
  if (hasMap(rule)) parts.push(t('gitlab.window.rules.values', rule.map.length))
  if (rule.action === 'tag')
    parts.push(
      rule.keep_prefix ? t('gitlab.window.rules.keepPrefix') : t('gitlab.window.rules.dropPrefix'),
    )
  return parts.join(' · ')
}

// ── a row open into its details ──
const openIdx = ref(-1)
function toggle(i) {
  openIdx.value = openIdx.value === i ? -1 : i
}

// ── keys on the table ──
const cursor = ref(-1)
const rowEls = {}
function focusRow(i) {
  cursor.value = i
  nextTick(() => rowEls[i]?.focus())
}
function move(i, delta) {
  if (ro.value) return
  const j = i + delta
  if (j < 0 || j >= f.rules.length) return
  f.moveRule(i, delta)
  if (openIdx.value === i) openIdx.value = j
  focusRow(j)
}
function remove(i) {
  if (ro.value) return
  const rule = f.rules[i]
  f.removeRule(i)
  if (openIdx.value === i) openIdx.value = -1
  undo.push({
    text: { key: 'gitlab.window.rules.deleted', params: { match: rule.match || '…' } },
    doneText: { key: 'gitlab.window.rules.restored', params: { match: rule.match || '…' } },
    undo: async () => {
      const next = f.rules.slice()
      next.splice(Math.min(i, next.length), 0, rule)
      f.rules = next
    },
  })
}
function onTableKey(e) {
  if (e.isComposing) return
  const tag = e.target?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return
  const i = cursor.value
  const plain = !e.metaKey && !e.ctrlKey && !e.altKey
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const step = e.key === 'ArrowDown' ? 1 : -1
    if (e.altKey && i >= 0) {
      move(i, step)
      return
    }
    const order = shown.value.map((s) => s.i)
    const at = order.indexOf(i)
    const next = order[Math.max(0, Math.min(order.length - 1, at + step))]
    if (next != null) focusRow(next)
  } else if (e.key === 'Enter' && plain && i >= 0) {
    e.preventDefault()
    toggle(i)
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
const ALT = isMac ? '⌥' : 'Alt'
const ARROW_UP = '↑'
const ARROW_DOWN = '↓'
// A GitLab label as the check field's sample — data, not interface text.
const CHECK_SAMPLE = 'priority::high'
const menuItems = computed(() => [
  {
    key: 'up',
    label: t('gitlab.window.rules.up'),
    icon: ArrowUpOutline,
    caps: [ALT, '↑'],
    disabled: menuFor.value <= 0,
  },
  {
    key: 'down',
    label: t('gitlab.window.rules.down'),
    icon: ArrowDownOutline,
    caps: [ALT, '↓'],
    disabled: menuFor.value >= f.rules.length - 1,
  },
  { gap: true },
  {
    key: 'delete',
    label: t('common.action.delete'),
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
  if (key === 'up') move(i, -1)
  else if (key === 'down') move(i, 1)
  else if (key === 'delete') remove(i)
}

function addNew() {
  if (ro.value) return
  filter.value = 'all'
  f.addRule()
  const i = f.rules.length - 1
  openIdx.value = i
  nextTick(() => rowEls[i]?.querySelector('input')?.focus())
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
  <div class="glw gr" data-testid="gitlab-rules">
    <div class="glw-title">
      <h2>{{ t('gitlab.window.nav.rules') }}</h2>
      <span class="glw-note">{{ t('gitlab.window.rules.note') }}</span>
      <span class="glw-grow" />
      <button
        v-if="!ro"
        type="button"
        class="kit-btn primary"
        data-testid="gitlab-rule-add"
        @click="addNew"
      >
        <n-icon :component="AddOutline" :size="15" />
        {{ t('gitlab.modal.rules.add') }}
        <Kbd>{{ cap('n') }}</Kbd>
      </button>
    </div>

    <div class="glw-scroll t-hoverscroll">
      <!-- What happens to every label no rule catches. -->
      <div class="glw-card gr-defaults">
        <label class="gr-def">
          <span class="glw-lbl-s">{{ t('gitlab.modal.rules.defaultColumn') }}</span>
          <n-select
            v-model:value="f.defaultColumn"
            :options="f.columnOptions"
            size="small"
            :placeholder="t('gitlab.modal.rules.defaultColumnPlaceholder')"
            :disabled="ro"
          />
        </label>
        <label class="gr-def">
          <span class="glw-lbl-s">{{ t('gitlab.modal.rules.otherLabels') }}</span>
          <n-select
            v-model:value="f.defaultAction"
            :options="f.defaultActionOptions"
            size="small"
            :disabled="ro"
          />
        </label>
        <label class="gr-def gr-def-sw">
          <span class="glw-lbl-s">{{ t('gitlab.modal.rules.tagKeepPrefix') }}</span>
          <n-switch v-model:value="f.tagKeepPrefix" :disabled="ro" />
        </label>
      </div>

      <div class="gr-tools">
        <span class="glw-seg gr-filter">
          <button
            v-for="s in segs"
            :key="s.key"
            type="button"
            :class="{ on: filter === s.key }"
            :data-testid="`gitlab-rules-filter-${s.key}`"
            @click="filter = s.key"
          >
            {{ s.label }} <span class="glw-seg-n">{{ s.n }}</span>
          </button>
          <n-tooltip :delay="300">
            <template #trigger>
              <span class="stub" aria-disabled="true">{{ t('gitlab.window.rules.stage') }}</span>
            </template>
            {{ t('common.stub.soon', { n: stub }) }}
          </n-tooltip>
        </span>
      </div>

      <div class="glw-thead gr-grid">
        <span />
        <span>{{ t('gitlab.window.rules.colLabel') }}</span>
        <span>{{ t('gitlab.window.rules.colMatch') }}</span>
        <span />
        <span>{{ t('gitlab.window.rules.colAction') }}</span>
        <span>{{ t('gitlab.window.rules.colDetails') }}</span>
        <span />
        <span />
      </div>

      <div class="gr-body" tabindex="-1" @keydown="onTableKey">
        <p v-if="!f.rules.length" class="gr-empty">{{ t('gitlab.window.rules.empty') }}</p>
        <template v-for="{ rule, i } in shown" :key="i">
          <div
            :ref="(el) => (rowEls[i] = el)"
            class="glw-trow gr-grid"
            :class="{ open: openIdx === i, ctx: menuFor === i }"
            tabindex="-1"
            data-testid="gitlab-rule-row"
            @focus="cursor = i"
          >
            <n-icon :component="ReorderTwoOutline" :size="15" class="gr-handle" />
            <span class="gr-match">
              <span class="gr-dot" :style="{ background: TONE[rule.action] || TONE.ignore }" />
              <input
                v-model="rule.match"
                class="gr-in glw-mono"
                spellcheck="false"
                :placeholder="t('gitlab.modal.rules.matchPlaceholder')"
                :disabled="ro"
                data-testid="gitlab-rule-match"
              />
            </span>
            <n-select
              v-model:value="rule.match_type"
              :options="f.matchTypeOptions"
              size="small"
              :disabled="ro"
            />
            <n-icon :component="ArrowForwardOutline" :size="14" class="glw-arrow" />
            <n-select
              v-model:value="rule.action"
              :options="f.actionOptions"
              size="small"
              :disabled="ro"
            />
            <button type="button" class="gr-sum" @click="toggle(i)">
              <n-icon
                :component="openIdx === i ? ChevronDownOutline : ChevronForwardOutline"
                :size="13"
              />
              <span class="gr-sum-t">{{
                summary(rule) || t('gitlab.window.rules.noDetails')
              }}</span>
            </button>
            <n-tooltip :delay="300">
              <template #trigger>
                <span class="glw-stub gr-sw" aria-disabled="true"
                  ><n-switch size="small" :value="true" disabled
                /></span>
              </template>
              {{ t('common.stub.soon', { n: stub }) }}
            </n-tooltip>
            <button
              v-if="!ro"
              type="button"
              class="glw-more"
              :title="t('common.action.more')"
              data-testid="gitlab-rule-menu"
              @click="openMenu(i, $event.currentTarget)"
            >
              <n-icon :component="EllipsisHorizontal" :size="16" />
            </button>
            <span v-else />
          </div>

          <div v-if="openIdx === i" class="glw-edit" data-testid="gitlab-rule-edit">
            <template v-if="rule.match_type === 'prefix'">
              <span class="glw-edit-l">{{ t('gitlab.modal.rules.friendlyName') }}</span>
              <n-input
                v-model:value="rule.label"
                size="small"
                :placeholder="t('gitlab.modal.rules.friendlyNamePlaceholder')"
                :disabled="ro"
              />
            </template>
            <template v-if="rule.action === 'tag'">
              <span class="glw-edit-l">{{ t('gitlab.modal.rules.keepPrefix') }}</span>
              <div><n-switch v-model:value="rule.keep_prefix" size="small" :disabled="ro" /></div>
            </template>
            <template v-if="hasMap(rule)">
              <span class="glw-edit-l">{{ t('gitlab.window.rules.valueMap') }}</span>
              <div class="gr-map">
                <div v-for="(m, mi) in rule.map" :key="mi" class="gr-map-row">
                  <n-input
                    v-model:value="m.k"
                    size="small"
                    class="glw-mono"
                    :placeholder="
                      rule.action === 'board'
                        ? 'Future'
                        : rule.action === 'priority'
                          ? 'Critical'
                          : 'In review'
                    "
                    :disabled="ro"
                  />
                  <n-icon :component="ArrowForwardOutline" :size="13" class="glw-arrow" />
                  <n-select
                    v-model:value="m.v"
                    :options="f.mapTargetOptions(rule.action)"
                    size="small"
                    :placeholder="t('gitlab.modal.rules.valuePlaceholder')"
                    :disabled="ro"
                  />
                  <button
                    v-if="!ro"
                    type="button"
                    class="glw-more"
                    :title="t('common.action.delete')"
                    @click="rule.map.splice(mi, 1)"
                  >
                    <n-icon :component="CloseOutline" :size="14" />
                  </button>
                </div>
                <button v-if="!ro" type="button" class="kit-btn small" @click="f.addMapRow(rule)">
                  <n-icon :component="AddOutline" :size="13" />
                  {{ t('gitlab.modal.rules.addValue') }}
                </button>
              </div>
            </template>
            <span
              v-if="rule.match_type !== 'prefix' && rule.action !== 'tag' && !hasMap(rule)"
              class="glw-edit-full glw-hint"
              >{{ t('gitlab.window.rules.nothingToSet') }}</span
            >
          </div>
        </template>
        <p v-if="f.rules.length" class="gr-keys">
          <Kbd>{{ ALT }}</Kbd
          ><Kbd>{{ ARROW_UP }}</Kbd
          ><Kbd>{{ ARROW_DOWN }}</Kbd> {{ t('gitlab.window.rules.keyOrder') }}
          <Kbd>{{ CAPS.enter }}</Kbd> {{ t('gitlab.window.rules.keyOpen') }}
          <Kbd>{{ CAPS.backspace }}</Kbd> {{ t('gitlab.window.rules.keyDelete') }}
        </p>
      </div>
    </div>

    <!-- «Проверить лейбл» (#3066): which rule fires and how many tasks it hits. -->
    <n-tooltip :delay="300">
      <template #trigger>
        <div
          class="glw-info gr-check glw-stub"
          aria-disabled="true"
          data-testid="gitlab-rule-check"
        >
          <n-icon :component="FlaskOutline" :size="16" />
          <span class="gr-check-t">{{ t('gitlab.window.rules.check') }}</span>
          <span class="glw-mono gr-check-in">{{ CHECK_SAMPLE }}</span>
        </div>
      </template>
      {{ t('common.stub.soon', { n: stub }) }}
    </n-tooltip>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="240"
      testid="gitlab-rule-row-menu"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.gr-defaults {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
  gap: 14px;
  align-items: end;
}
.gr-def {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.gr-def-sw {
  align-items: flex-start;
  padding-bottom: 4px;
}
.gr-tools {
  display: flex;
  align-items: center;
  margin: 16px 0 12px;
  overflow-x: auto;
  scrollbar-width: none;
}
.gr-grid {
  grid-template-columns: 16px minmax(150px, 1.3fr) 112px 14px 150px minmax(0, 1fr) 36px 28px;
}
.gr-body {
  outline: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.gr-empty {
  margin: 18px 10px;
  font-size: 13px;
  color: var(--t-text3);
}
.gr-handle {
  color: var(--t-text3);
  opacity: 0.6;
}
.gr-match {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 10px;
  border-radius: 8px;
  background: var(--t-pill);
  min-width: 0;
}
.gr-match:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.gr-dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: 2px;
}
.gr-in {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font-size: 12.5px;
}
.gr-in::placeholder {
  color: var(--t-placeholder);
}
.gr-sum {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  height: 30px;
  padding: 0 6px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  text-align: left;
  cursor: pointer;
}
.gr-sum:hover {
  background: var(--t-ctl);
  color: var(--t-text1);
}
.gr-sum-t {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gr-sw {
  display: inline-flex;
}
.gr-map {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
.gr-map-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 14px minmax(0, 1fr) 28px;
  gap: 8px;
  align-items: center;
  width: 100%;
}
.gr-keys {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  margin: 12px 10px 0;
  font-size: 12px;
  color: var(--t-text3);
}
.gr-check {
  align-items: center;
}
.gr-check-t {
  font-weight: 650;
  color: var(--t-text1);
}
.gr-check-in {
  color: var(--t-text3);
}
.lw-mobile .gr-defaults {
  grid-template-columns: minmax(0, 1fr);
}
.lw-mobile .gr-grid {
  grid-template-columns: minmax(0, 1fr) 100px 28px;
}
.lw-mobile .gr-grid > :nth-child(1),
.lw-mobile .gr-grid > :nth-child(4),
.lw-mobile .gr-grid > :nth-child(6),
.lw-mobile .gr-grid > :nth-child(7) {
  display: none;
}
.lw-mobile .glw-thead.gr-grid {
  display: none;
}
.lw-mobile .gr-match {
  grid-column: 1 / -1;
}
.lw-mobile .gr-keys {
  display: none;
}
</style>
