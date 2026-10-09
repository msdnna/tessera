<script setup>
// One toast (v2, #3117 — mockup Toast): the frame every toast shares since the
// four toast systems were folded into one. --t-pop, radius 14, the drag shadow,
// no border. On the left a person with the badge of what happened, or a 30px
// tile in the tone of the kind; on the right the time and ×; a 3px timer along
// the bottom in the accent gradient (grey while paused). Two looks:
//
//   card — «про меня», reminders, calls, the system, answers to my actions:
//          the sentence, the task as a chip, a third line (where it went, a
//          quote, the changes), the buttons with their keys;
//   act  — the open board's activity: one 44px line.
//
// The look only: the lanes, the clock and the keys are ToastLanes' and
// stores/toasts'.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import {
  AlarmOutline,
  AlertCircleOutline,
  ArchiveOutline,
  ArrowForwardOutline,
  ArrowUndoOutline,
  AtOutline,
  CalendarOutline,
  ChatbubbleEllipsesOutline,
  CheckmarkDoneOutline,
  CheckmarkOutline,
  ChevronForwardOutline,
  CloseOutline,
  CloudDownloadOutline,
  CloudOfflineOutline,
  CreateOutline,
  GitMergeOutline,
  InformationCircleOutline,
  NotificationsOutline,
  PersonAddOutline,
  PersonRemoveOutline,
  TrashOutline,
  VideocamOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'
import TesseraIcon from '@/components/TesseraIcon.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { stubTask } from '@/utils/featureStubs'
import { notificationAge } from '@/utils/notificationFeed'
import { awayLine } from '@/utils/toastModel'
import { useFormat } from '@/composables/useFormat'

const props = defineProps({
  toast: { type: Object, required: true },
  // The lane is hovered or holds the focus: the about-me buttons show, the
  // timer stands still.
  active: { type: Boolean, default: false },
  paused: { type: Boolean, default: false },
  // Keyboard focus is on this toast (F6, ↑↓): the 2px ring.
  focused: { type: Boolean, default: false },
  // The last input was the keyboard: the key caps on the buttons show.
  kbd: { type: Boolean, default: true },
  // A phone: no ×, a 40px button, the whole toast is the main action.
  touch: { type: Boolean, default: false },
  // prefers-reduced-motion: «8 с» on the right instead of the timer bar.
  reduced: { type: Boolean, default: false },
  now: { type: Number, default: 0 },
})
const emit = defineEmits(['run', 'close', 'details', 'copy'])
const { t } = useI18n()
const { formatters } = useFormat()

const ICONS = {
  AlarmOutline,
  AlertCircleOutline,
  ArchiveOutline,
  ArrowForwardOutline,
  ArrowUndoOutline,
  AtOutline,
  CalendarOutline,
  ChatbubbleEllipsesOutline,
  CheckmarkDoneOutline,
  CheckmarkOutline,
  CloudDownloadOutline,
  CloudOfflineOutline,
  CreateOutline,
  GitMergeOutline,
  InformationCircleOutline,
  NotificationsOutline,
  PersonAddOutline,
  PersonRemoveOutline,
  TrashOutline,
  VideocamOutline,
  WarningOutline,
}
const icon = (name) => ICONS[name] || NotificationsOutline
const val = (v) => (typeof v === 'function' ? v() : v) || ''

const x = computed(() => props.toast)
const act = computed(() => x.value.look === 'act')
const lead = computed(() => x.value.lead || {})

const time = computed(() => {
  if (props.reduced && x.value.duration)
    return t('toasts.seconds', { n: Math.max(1, Math.ceil(x.value.left / 1000)) })
  if (!x.value.time) return ''
  return notificationAge(x.value.time, {
    t,
    formatters: formatters.value,
    now: new Date(props.now || Date.now()),
  }).replace(t('notifications.pop.now'), t('toasts.now'))
})

const actions = computed(() =>
  (x.value.actions || [])
    .filter((a) => !a.hidden)
    .map((a) => ({
      ...a,
      text: a.details && x.value.detailsOpen ? t('toasts.err.copy') : val(a.label),
    })),
)
// About-me buttons show on hover or keyboard focus; reminders, calls, the
// system and the answers to my actions always. A phone: the tap is «Открыть»,
// so it is not a button there.
const shownActions = computed(() => {
  let list = actions.value
  if (props.touch) list = list.filter((a) => !(a.primary && a.id === 'open'))
  if (!list.length) return []
  return x.value.always || props.active || props.focused || props.touch ? list : []
})
const chevron = computed(() => props.touch && !shownActions.value.length && !act.value)
const progress = computed(() =>
  x.value.duration ? Math.max(0, Math.min(1, x.value.left / x.value.duration)) : 0,
)
const bumping = computed(() => x.value.bump && (props.now || Date.now()) - x.value.bump < 600)
const away = computed(() => (x.value.awayCounts ? awayLine(x.value.awayCounts, t) : ''))
const stubTip = (a) => t('common.stub.soon', { n: stubTask(a.stub) })

function onAction(a, e) {
  if (a.details) {
    if (x.value.detailsOpen) emit('copy')
    else emit('details')
    return
  }
  emit('run', a.id, e.currentTarget)
}
</script>

<template>
  <div
    class="tt"
    :class="[
      toast.cls,
      {
        act,
        touch,
        focused,
        paused,
        [`k-${toast.kind}`]: true,
      },
    ]"
    data-testid="toast"
    :data-kind="toast.kind"
    :data-toast="toast.id"
  >
    <!-- the lead: a person with a badge, two people, or a tile -->
    <span v-if="lead.avatar" class="tt-av" :class="{ small: act }">
      <UserAvatar
        :user-id="lead.avatar.userId"
        :name="lead.avatar.name"
        :src="lead.avatar.src || ''"
      />
      <span v-if="lead.badge && !act" class="tt-badge" :class="lead.tone || 'acc'">
        <n-icon :component="icon(lead.badge)" :size="10" />
      </span>
    </span>
    <span v-else-if="lead.duo" class="tt-duo">
      <UserAvatar class="a" :user-id="lead.duo[0].userId" :name="lead.duo[0].name" />
      <UserAvatar class="b" :user-id="lead.duo[1].userId" :name="lead.duo[1].name" />
    </span>
    <span v-else-if="lead.tile" class="tt-tile" :class="lead.tone || 'acc'">
      <TesseraSpinner v-if="lead.spin" :size="14" />
      <n-icon v-else :component="icon(lead.tile)" :size="16" />
    </span>

    <!-- the activity line -->
    <span v-if="act" class="tt-act">
      <b>{{ val(toast.bold) }}</b> {{ val(toast.rest) }}
      <template v-if="toast.batch">
        <span v-if="bumping" class="tt-n hot" data-testid="toast-count">{{ toast.count }}</span>
        <b v-else class="tt-n" data-testid="toast-count">{{ toast.count }}</b>
        {{ toast.tail(toast.count) }}
      </template>
    </span>
    <template v-if="act">
      <button
        v-for="a in actions"
        :key="a.id"
        type="button"
        class="tt-btn"
        :data-testid="`toast-action-${a.id}`"
        @click.stop="onAction(a, $event)"
      >
        {{ a.text }}<Kbd v-if="kbd && focused && a.caps?.length && !touch">{{ a.caps[0] }}</Kbd>
      </button>
      <Kbd v-if="focused && kbd && !toast.batch && !touch" class="tt-enter">{{ CAPS.enter }}</Kbd>
    </template>

    <!-- the card body -->
    <div v-else class="tt-body">
      <div class="tt-l1">
        <span class="tt-text">
          <b v-if="val(toast.bold)">{{ val(toast.bold) }}</b>
          {{ val(toast.rest) }}
        </span>
        <span v-if="time" class="tt-time">{{ time }}</span>
      </div>
      <span v-if="toast.chip" class="tt-chip" :class="{ gone: toast.chip.gone }">
        <TesseraIcon
          v-if="toast.chip.status"
          :name="toast.chip.status"
          :size="11"
          class="tt-st"
          :class="toast.chip.status"
        />
        <span v-if="toast.chip.ref != null" class="tt-ref">#{{ toast.chip.ref }}</span>
        <span class="tt-ct">{{ toast.chip.title }}</span>
      </span>
      <span v-if="val(toast.line)" class="tt-l3">{{ val(toast.line) }}</span>
      <span v-if="away" class="tt-l3" data-testid="toast-away">{{ away }}</span>
      <span v-if="toast.diff" class="tt-diff">
        <n-icon :component="ArrowForwardOutline" :size="12" />
        <b>{{ val(toast.diff.to) }}</b>
      </span>
      <div v-if="toast.list?.length" class="tt-list">
        <span v-for="(l, i) in toast.list" :key="i" class="tt-l3">{{ val(l) }}</span>
      </div>
      <span v-if="toast.quote" class="tt-quote">{{
        t('notifications.pop.quote', { text: toast.quote })
      }}</span>
      <div v-if="toast.details && toast.detailsOpen" class="tt-det" data-testid="toast-details">
        <span v-for="(l, i) in toast.details" :key="i">{{ l }}</span>
      </div>
      <div v-if="shownActions.length" class="tt-acts" :class="{ touch }">
        <template v-for="a in shownActions" :key="a.id">
          <n-tooltip v-if="a.stub" :delay="300">
            <template #trigger>
              <span class="tt-stub">
                <button
                  type="button"
                  class="tt-b"
                  :class="{ primary: a.primary }"
                  disabled
                  :data-testid="`toast-action-${a.id}`"
                >
                  {{ a.text }}
                </button>
              </span>
            </template>
            {{ stubTip(a) }}
          </n-tooltip>
          <button
            v-else
            type="button"
            class="tt-b"
            :class="{ primary: a.primary }"
            :data-testid="`toast-action-${a.id}`"
            @click.stop="onAction(a, $event)"
          >
            {{ a.text }}
            <span v-if="kbd && !touch && a.caps?.length" class="tt-caps">
              <Kbd v-for="c in a.caps" :key="c">{{ c }}</Kbd>
            </span>
          </button>
        </template>
      </div>
    </div>

    <button
      v-if="!touch"
      type="button"
      class="tt-x"
      :title="t('toasts.closeEsc')"
      :aria-label="t('toasts.close')"
      data-testid="toast-close"
      @click.stop="emit('close')"
    >
      <n-icon :component="CloseOutline" :size="15" />
    </button>
    <span v-if="chevron" class="tt-chev"
      ><n-icon :component="ChevronForwardOutline" :size="16"
    /></span>

    <span
      v-if="toast.duration && !reduced"
      class="tt-bar"
      :class="{ paused }"
      :style="{ width: `${progress * 100}%` }"
    />
  </div>
</template>

<style scoped>
.tt {
  position: relative;
  overflow: hidden;
  display: flex;
  gap: 11px;
  padding: 10px 6px 10px 12px;
  box-sizing: border-box;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  font-size: 13px;
  text-align: left;
  pointer-events: auto;
  /* The avatar's cut-out ring and the chip read --t-surface. */
  --t-surface: var(--t-pop);
}
.tt.focused::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 14px;
  box-shadow: inset 0 0 0 2px var(--t-primary);
  pointer-events: none;
}

/* ── the lead ── */
.tt-av {
  position: relative;
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  margin-top: 1px;
}
.tt-av :deep(.ua),
.tt-duo :deep(.ua) {
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
}
.tt-av.small {
  flex: 0 0 22px;
  width: 22px;
  height: 22px;
  margin-top: 0;
}
.tt-av.small :deep(.ua) {
  font-size: 9px;
}
.tt-badge {
  position: absolute;
  right: -5px;
  bottom: -5px;
  width: 16px;
  height: 16px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  background: var(--t-accent-grad);
  box-shadow: 0 0 0 2px var(--t-pop);
}
.tt-badge.red {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
}
.tt-badge.or {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
}
.tt-badge.gr {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 86%, #000),
    #18a058 50%,
    color-mix(in srgb, #18a058 86%, #fff)
  );
}
.tt-duo {
  position: relative;
  flex: 0 0 30px;
  width: 30px;
  height: 30px;
}
.tt-duo :deep(.ua) {
  position: absolute;
  width: 22px;
  height: 22px;
  font-size: 9px;
}
.tt-duo :deep(.a) {
  left: 0;
  top: 0;
}
.tt-duo :deep(.b) {
  left: 9px;
  top: 9px;
  box-shadow: 0 0 0 2px var(--t-pop);
}
.tt-tile {
  flex: 0 0 30px;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  --tone: var(--t-primary);
  color: var(--tone);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--tone) 16%, var(--t-pop)),
    color-mix(in srgb, var(--tone) 30%, var(--t-pop))
  );
}
.tt-tile.red {
  --tone: #e0533d;
}
.tt-tile.or {
  --tone: #f0a020;
}
.tt-tile.gr {
  --tone: #18a058;
}
.tt-tile.neu {
  background: var(--t-pill);
  color: var(--t-text2);
}

/* ── the card body ── */
.tt-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}
.tt-l1 {
  align-self: stretch;
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-height: 20px;
}
.tt-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.45;
  color: var(--t-text2);
  text-wrap: pretty;
  overflow-wrap: anywhere;
}
.tt-text b,
.tt-act b,
.tt-diff b {
  color: var(--t-text1);
  font-weight: 600;
}
.tt-time {
  flex: none;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.tt-chip {
  height: 22px;
  max-width: 100%;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 0 8px 0 6px;
  border-radius: 999px;
  background: var(--t-pill);
  font-size: 12px;
  color: var(--t-text1);
}
.tt-st {
  flex: none;
  color: #2f80ed;
}
.tt-st.status-todo {
  color: #0eb0a9;
}
.tt-st.status-review {
  color: #f0a020;
}
.tt-st.status-done {
  color: #18a058;
}
.tt-ref {
  flex: none;
  color: var(--t-text3);
  white-space: nowrap;
}
.tt-ct {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tt-chip.gone .tt-ct {
  text-decoration: line-through;
  color: var(--t-text3);
}
.tt-l3 {
  max-width: 100%;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tt-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-width: 100%;
}
.tt-diff {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text3);
  white-space: nowrap;
}
.tt-quote {
  max-width: 100%;
  box-sizing: border-box;
  padding: 4px 9px;
  border-radius: 9px;
  background: var(--t-sub);
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tt-det {
  align-self: stretch;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 2px;
  padding: 8px 10px;
  border-radius: 10px;
  background: var(--t-sub);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--t-text2);
  word-break: break-all;
  user-select: text;
}

/* ── buttons ── */
.tt-acts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}
.tt-b,
.tt-btn {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.tt-b:hover,
.tt-btn:hover {
  background: var(--t-hover);
}
.tt-b.primary {
  font-weight: 600;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
}
.tt-b.primary:hover {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 24%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 38%, var(--t-pop))
  );
}
.tt-b:disabled {
  opacity: 0.55;
  cursor: default;
}
.tt-b:focus-visible,
.tt-btn:focus-visible,
.tt-x:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.tt-caps {
  display: inline-flex;
  gap: 3px;
  margin-right: -5px;
}
.tt-stub {
  display: inline-flex;
}
.tt-acts.touch .tt-b {
  height: 40px;
  padding: 0 14px;
  border-radius: 10px;
  font-size: 14px;
}
.tt-x {
  flex: 0 0 26px;
  width: 26px;
  height: 26px;
  margin-top: -3px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tt-x:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tt-chev {
  flex: 0 0 28px;
  align-self: center;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--t-text3);
}

/* ── the activity line ── */
.tt.act {
  align-items: center;
  gap: 9px;
  height: 44px;
  padding: 0 6px 0 11px;
}
.tt.act .tt-x {
  margin-top: 0;
}
.tt-act {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tt-n {
  color: var(--t-text1);
  font-weight: 600;
}
.tt-n.hot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  margin: 0 2px;
  box-sizing: border-box;
  border-radius: 10px;
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 700;
  font-size: 12px;
  vertical-align: 1px;
}
.tt-enter {
  flex: none;
}

/* ── the timer ── */
.tt-bar {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 3px;
  border-radius: 0 2px 2px 0;
  background: var(--t-accent-grad);
  transition: width 0.1s linear;
}
.tt-bar.paused {
  background: var(--t-text3);
  opacity: 0.55;
}
@media (prefers-reduced-motion: reduce) {
  .tt-bar {
    transition: none;
  }
}
</style>
