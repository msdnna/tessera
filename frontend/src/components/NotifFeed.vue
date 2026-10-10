<script setup>
// The notification feed (v2, #3107 — PackageE «NotifPop»), shown by the bell in
// the header (NotificationsBell): a popover on a desktop, a page of its own on
// a phone (#2976 — package G «Уведомления», `page`). The feed spans every
// workspace (opening an item may switch to another one), so it belongs to the
// shell rather than to the current workspace.
//
// The feed is a 420px card: a head with the unread count, «Прочитать все»
// (⇧E) and a way to the notification settings; filters «Все · Упоминания ·
// Назначения · GitLab»; rows grouped «Новые» / «Ранее». A row is the person
// (or the system's glyph), the sentence with the name in bold, what it is about
// as a chip or a quote, and the time; an unread row sits on a soft accent with
// a dot. Keys while it is open: ↑↓ pick, ↵ open, E mark read, ⇧E read all.
//
// In the popover the filters and groups are the client's own reading of the 50
// rows the store holds. `all` is the page of all notifications (#3049): the
// rows come from the server a page at a time (useNotificationPages), filtered
// there, grouped by day, with «Непрочитанные» among the filters and more rows
// loaded as the list is scrolled to its end. An invitation to a meeting is
// answered right in its row (#3051); a sync conflict opens its resolver (#3048).
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { NIcon, NTooltip } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  NotificationsOutline,
  SettingsOutline,
  AlarmOutline,
  TimeOutline,
  GitBranchOutline,
  ChevronForwardOutline,
  ChevronBackOutline,
  DocumentTextOutline,
  MoonOutline,
  VideocamOutline,
  TrailSignOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/EmptyState.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import { notificationText } from '@/utils/notificationText'
import { notificationRoute } from '@/utils/notificationRoute'
import {
  notificationGroup,
  notificationAge,
  groupByDay,
  NOTIFICATION_FILTERS,
  PAGE_FILTERS,
} from '@/utils/notificationFeed'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import { useConflictsStore } from '@/stores/conflicts'
import { useNotificationPages } from '@/composables/useNotificationPages'
import { useToastsStore } from '@/stores/toasts'
import { dndPreset } from '@/utils/toastModel'
import { useFormat } from '@/composables/useFormat'
import './kit/kit.css'

const props = defineProps({
  // A phone (#2976): the feed is the page — a back button, no footer of key
  // hints, rows on the page's own surface.
  page: { type: Boolean, default: false },
  // The page of all notifications (#3049): rows from the server, paged.
  all: { type: Boolean, default: false },
})
// `close` before the feed navigates anywhere: the popover steps aside, the
// page goes back.
const emit = defineEmits(['close', 'back'])

const ws = useWorkspacesStore()
const notes = useNotificationsStore()
const toasts = useToastsStore()
const router = useRouter()
const message = useMessage()
const { formatters } = useFormat()
const { t, te } = useI18n()
const conflicts = useConflictsStore()

// The page of all notifications reads its own rows from the server.
const pager = props.all ? useNotificationPages() : null
const filters = props.all ? PAGE_FILTERS : NOTIFICATION_FILTERS

// ── the filter and the groups ─────────────────────────────────────────
const filter = ref('all')
const visible = computed(() =>
  pager
    ? pager.items.value
    : notes.items.filter((n) => filter.value === 'all' || notificationGroup(n) === filter.value),
)
const fresh = computed(() => visible.value.filter((n) => !n.read_at))
// The groups as drawn: «Новые / Ранее» in the popover, days on the page.
const groups = computed(() => {
  if (pager) return groupByDay(visible.value, { t, formatters: formatters.value })
  const earlier = visible.value.filter((n) => n.read_at)
  return [
    {
      key: 'fresh',
      label: t('notifications.pop.fresh'),
      count: fresh.value.length,
      rows: fresh.value,
    },
    { key: 'earlier', label: t('notifications.pop.earlier'), rows: earlier },
  ].filter((g) => g.rows.length)
})
// The keyboard walks the rows in the order they are drawn.
const ordered = computed(() => groups.value.flatMap((g) => g.rows))
// Unread per filter, for the counts on the tabs — the server's on the page.
const unreadBy = computed(() => {
  if (pager) {
    const c = pager.counts
    return { unread: c.total, mentions: c.mentions, assigned: c.assigned, gitlab: c.gitlab }
  }
  const out = {}
  for (const n of notes.items) {
    if (n.read_at) continue
    const g = notificationGroup(n)
    out[g] = (out[g] || 0) + 1
  }
  return out
})
const unreadCount = computed(() => (pager ? pager.counts.total : notes.unread))
function pickFilter(f) {
  if (filter.value === f) return
  filter.value = f
  if (pager) pager.reset(f)
}
if (pager) onMounted(() => pager.reset('all'))

// ── one row ───────────────────────────────────────────────────────────
// The sentence is rendered from the payload in the reader's language; the
// content it carries (a task title, a comment excerpt) is drawn under it as a
// chip or a quote instead, so it is not said twice.
function sentence(n) {
  return notificationText(n, { t, te, formatters: formatters.value, ctx: !chipOf(n) })
}
// The actor's name leads the sentence in every language we ship; drawn bold.
function parts(n) {
  const text = sentence(n)
  const actor = n.payload?.actor || ''
  if (actor && text.startsWith(actor)) return { actor, rest: text.slice(actor.length) }
  return { actor: '', rest: text }
}
function chipOf(n) {
  const p = n.payload || {}
  // The quote is the comment cut for one line (#3117); the excerpt is only the
  // short inline context and is absent for a longer comment.
  if (p.quote || p.excerpt) return { quote: p.quote || p.excerpt }
  // A deleted task (#3114) has no row to join: the number and title come from
  // the payload alone, and the row opens nothing (notificationRoute → null).
  // A closed stage (#3048) names itself in the sentence; the chip says where.
  if (p.event === 'milestone_closed')
    return { stage: true, title: t('notifications.pop.stageOf', { project: p.project || '' }) }
  if (
    p.title &&
    p.event !== 'task_due_soon' &&
    p.event !== 'conference_invited' &&
    p.event !== 'conference_starting'
  )
    return {
      title: p.title,
      ref: n.task_number ?? p.task_number,
      gone: p.event === 'task_deleted',
    }
  return null
}
// A row without a person carries the system's glyph in its own tone.
const SYSTEM = {
  reminder: { icon: AlarmOutline, tone: '#f0a020' },
  due_soon: { icon: TimeOutline, tone: '#e0533d' },
  integration_sync: { icon: GitBranchOutline, tone: '#fc6d26' },
  // v2 notifications (#3048).
  integration_conflict: { icon: GitBranchOutline, tone: '#fc6d26' },
  conference_soon: { icon: VideocamOutline, tone: '#18a058' },
}
function systemOf(n) {
  if (n.payload?.actor) return null
  return SYSTEM[n.kind] || { icon: NotificationsOutline, tone: '' }
}
function tileStyle(tone) {
  const hue = tone || 'var(--t-primary)'
  return { background: `color-mix(in srgb, ${hue} 16%, transparent)`, color: hue }
}
function age(n) {
  return notificationAge(n.created_at, { t, formatters: formatters.value })
}
const isInvite = (n) => n.kind === 'conference_invite' || n.payload?.event === 'conference_invited'
const isConflict = (n) => n.payload?.event === 'integration_conflict' && !!n.task_id
// «в 15:00» / «завтра, 15:00» beside an invitation to a planned meeting.
function inviteWhen(n) {
  const at = n.payload?.scheduled_at
  if (!at) return ''
  const d = new Date(at)
  if (Number.isNaN(d.getTime())) return ''
  const f = formatters.value
  const rel = f.formatRelative(d, { dateOnly: false })
  const time = f.formatTime(d)
  return rel ? `${rel}, ${time}` : f.formatDateTime(d)
}

// ── actions ───────────────────────────────────────────────────────────
function openNotification(n) {
  // The toast travels as an argument because the store cannot reach useMessage()
  // (see stores/notifications.js): a refused mark rolls the row back to unread,
  // and the reader is told why instead of watching the dot reappear (#2932).
  notes.markRead(n.id, message)
  pager?.markLocal(n)
  // Switch the active workspace if the task lives in another one, so the sidebar
  // tree reflects where the opened task actually is.
  if (n.workspace_id && n.workspace_id !== ws.currentId) {
    ws.selectWorkspace(n.workspace_id)
  }
  const to = notificationRoute(n)
  if (!to) return
  emit('close')
  router.push(to)
}
function markOne(n) {
  if (!n || n.read_at) return
  notes.markRead(n.id, message)
  pager?.markLocal(n)
}
function markAll() {
  if (!unreadCount.value) return
  notes.markAllRead(message)
  pager?.markAllLocal()
}
// «Принять / Отклонить» (#3051): the answer shows at once, a refusal undoes it.
function answer(n, response) {
  if (n.rsvp === response) return
  notes.rsvp(n, response, message)
}
// «Разрешить конфликт» (#3048): the resolver, on this task's conflict.
function resolveConflict(n) {
  markOne(n)
  if (n.workspace_id && n.workspace_id !== ws.currentId) ws.selectWorkspace(n.workspace_id)
  emit('close')
  conflicts.openResolver(n.task_id)
}
function openAll() {
  emit('close')
  router.push('/notifications')
}
// The end of the page's list loads the next page as it comes into view.
const sentinel = ref(null)
let io = null
watch(sentinel, (el) => {
  io?.disconnect()
  if (!el || !pager || typeof IntersectionObserver === 'undefined') return
  // The viewport as the root: whichever box scrolls (the list, or the page
  // around it), an end clipped out of view does not intersect.
  io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && pager.loadMore(), {
    rootMargin: '200px',
  })
  io.observe(el)
})
onBeforeUnmount(() => io?.disconnect())
function openSettings() {
  emit('close')
  router.push({ path: '/settings', query: { s: 'notifications' } })
}

// ── «Не беспокоить» (#3117): the moon in the head ───────────────────────
// Silences the toasts «про меня», activity and the system for an hour or till
// tomorrow 9:00; the feed itself goes on filling, the count goes on growing.
const moonEl = ref(null)
const dndMenu = ref(false)
// The menu floats in <body>: inside the bell's popover a fixed layer is placed
// against the popover's transform, not the window. A click in it is outside
// the popover, so the bell is asked to hold the feed open meanwhile.
watch(dndMenu, (v) => (toasts.feedHold = v ? Infinity : Date.now() + 400))
onBeforeUnmount(() => (toasts.feedHold = 0))
const dndUntilText = computed(() =>
  toasts.server.dnd_until ? formatters.value.formatTime(new Date(toasts.server.dnd_until)) : '',
)
const dndItems = computed(() => {
  const items = [
    {
      key: 'hour',
      label: t('toasts.dnd.hour'),
      icon: TimeOutline,
      note: t('toasts.dnd.until', { time: formatters.value.formatTime(dndPreset('hour')) }),
      caps: ['1'],
    },
    { key: 'tomorrow', label: t('toasts.dnd.tomorrow'), icon: MoonOutline, caps: ['2'] },
  ]
  if (toasts.dnd) items.push({ gap: true }, { key: 'off', label: t('toasts.dnd.off') })
  items.push({ hint: t('toasts.dnd.note') })
  return items
})
async function setDnd(key) {
  dndMenu.value = false
  try {
    await toasts.setDnd(key === 'off' ? null : dndPreset(key))
  } catch (e) {
    message.error(e.message)
  }
}

// ── keys while the feed is open ───────────────────────────────────────
// The cursor is shown only once the keyboard is used: a mouse reader gets the
// hover level, not a second highlight that follows nothing they did.
const cursor = ref(-1)
const kbd = ref(false)
const list = ref(null)
// The feed is mounted only while it is shown (the popover renders its content
// lazily; the page is a route), so its keys live and die with it.
onMounted(() => window.addEventListener('keydown', onKey, true))
watch(filter, () => (cursor.value = -1))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))

function move(step) {
  const n = ordered.value.length
  if (!n) return
  kbd.value = true
  cursor.value = cursor.value < 0 ? (step > 0 ? 0 : n - 1) : (cursor.value + step + n) % n
  nextTick(() =>
    list.value
      ?.querySelector(`[data-row="${ordered.value[cursor.value]?.id}"]`)
      ?.scrollIntoView?.({ block: 'nearest' }),
  )
}
function onKey(e) {
  if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return
  // The moon's menu has the keys while it is open.
  if (dndMenu.value) return
  if (e.target?.closest?.('input, textarea, [contenteditable="true"]')) return
  const k = e.key
  if (k === 'ArrowDown' || k === 'ArrowUp') move(k === 'ArrowDown' ? 1 : -1)
  else if (k === 'Enter' && cursor.value >= 0) openNotification(ordered.value[cursor.value])
  else if ((k === 'e' || k === 'E' || e.code === 'KeyE') && e.shiftKey) markAll()
  else if (k === 'e' || k === 'E' || e.code === 'KeyE')
    markOne(ordered.value[cursor.value] || fresh.value[0])
  else return
  e.preventDefault()
  e.stopPropagation()
}
</script>

<template>
  <div
    class="np"
    :class="{ page, all }"
    data-testid="notifications-feed"
    :data-all="all ? '' : undefined"
    @mousemove="kbd = false"
  >
    <header class="np-head">
      <button
        v-if="page"
        type="button"
        class="np-back"
        :aria-label="t('shell.mobile.back')"
        data-testid="notifications-back"
        @click="emit('back')"
      >
        <n-icon :component="ChevronBackOutline" :size="22" />
      </button>
      <span class="np-title">{{
        all && !page ? t('notifications.pop.page') : t('shell.tools.notifications')
      }}</span>
      <span v-if="unreadCount" class="np-count">{{ unreadCount }}</span>
      <span class="np-spacer" />
      <button
        type="button"
        class="np-quiet"
        :disabled="!unreadCount"
        data-testid="notifications-read-all"
        @click="markAll"
      >
        {{ t('shell.tools.markAllRead') }}
        <ShortcutKeys id="notifications.readAll" joined />
      </button>
      <n-tooltip :delay="400">
        <template #trigger>
          <button
            ref="moonEl"
            type="button"
            class="np-icon"
            :class="{ on: toasts.dnd || dndMenu }"
            :aria-label="t('toasts.dnd.title')"
            :aria-pressed="toasts.dnd"
            data-testid="notifications-dnd"
            @click="dndMenu = !dndMenu"
          >
            <n-icon :component="MoonOutline" :size="15" />
          </button>
        </template>
        {{ toasts.dnd ? t('toasts.dnd.bell', { time: dndUntilText }) : t('toasts.dnd.title') }}
      </n-tooltip>
      <ActionMenu
        v-model:show="dndMenu"
        :anchor="() => moonEl"
        placement="bottom-end"
        :offset="{ x: 0, y: 6 }"
        :width="248"
        :header="t('toasts.dnd.title')"
        :items="dndItems"
        testid="dnd-menu"
        @select="setDnd"
      />
      <n-tooltip :delay="400">
        <template #trigger>
          <button
            type="button"
            class="np-icon"
            :aria-label="t('notifications.pop.settings')"
            data-testid="notifications-settings"
            @click="openSettings"
          >
            <n-icon :component="SettingsOutline" :size="16" />
          </button>
        </template>
        {{ t('notifications.pop.settings') }}
      </n-tooltip>
    </header>

    <div v-if="toasts.dnd" class="np-dnd" data-testid="notifications-dnd-banner">
      <n-icon :component="MoonOutline" :size="16" class="np-dnd-ic" />
      <div class="np-dnd-t">
        <span class="np-dnd-title">{{ t('toasts.dnd.banner', { time: dndUntilText }) }}</span>
        <span class="np-dnd-sub">{{ t('toasts.dnd.bannerSub') }}</span>
      </div>
      <button
        type="button"
        class="kit-btn small"
        data-testid="notifications-dnd-off"
        @click="setDnd('off')"
      >
        {{ t('toasts.dnd.off') }}
      </button>
    </div>

    <div class="np-tabs" role="tablist">
      <button
        v-for="f in filters"
        :key="f"
        type="button"
        role="tab"
        class="np-tab"
        :class="{ on: filter === f }"
        :aria-selected="filter === f"
        :data-testid="`notifications-filter-${f}`"
        @click="pickFilter(f)"
      >
        {{ t(`notifications.pop.filter.${f}`) }}
        <span v-if="f !== 'all' && unreadBy[f]" class="np-tab-n">{{ unreadBy[f] }}</span>
      </button>
    </div>

    <div ref="list" class="np-list">
      <template v-for="grp in groups" :key="grp.key">
        <template v-if="grp.rows.length">
          <div class="np-group" data-testid="notifications-group">
            <span>{{ grp.label }}</span>
            <span v-if="grp.count" class="np-group-n">{{ grp.count }}</span>
          </div>
          <div
            v-for="n in grp.rows"
            :key="n.id"
            class="np-row"
            :class="{ unread: !n.read_at, cur: kbd && ordered[cursor]?.id === n.id }"
            :data-row="n.id"
            data-testid="notification-row"
            role="button"
            tabindex="-1"
            @click="openNotification(n)"
          >
            <span v-if="!n.read_at" class="np-dot" />
            <span
              v-if="systemOf(n)"
              class="np-tile"
              :style="tileStyle(systemOf(n).tone)"
              aria-hidden="true"
            >
              <n-icon :component="systemOf(n).icon" :size="16" />
            </span>
            <user-avatar
              v-else
              :user-id="n.actor_id || ''"
              :name="n.payload?.actor || ''"
              class="np-av"
            />
            <div class="np-body">
              <div class="np-line">
                <span class="np-text">
                  <b v-if="parts(n).actor">{{ parts(n).actor }}</b
                  >{{ parts(n).rest }}
                </span>
                <span class="np-age">{{ age(n) }}</span>
              </div>
              <div v-if="chipOf(n)?.quote" class="np-quote">
                {{ t('notifications.pop.quote', { text: chipOf(n).quote }) }}
              </div>
              <span v-else-if="chipOf(n)" class="np-chip" :class="{ gone: chipOf(n).gone }">
                <n-icon
                  :component="chipOf(n).stage ? TrailSignOutline : DocumentTextOutline"
                  :size="12"
                />
                <span v-if="chipOf(n).ref" class="np-chip-ref">#{{ chipOf(n).ref }}</span>
                <span class="np-chip-t">{{ chipOf(n).title }}</span>
              </span>
              <!-- «Принять / Отклонить» (#3051): the answer stays drawn, and
                   the other button changes it. No seat (the call is gone) —
                   nothing to answer. -->
              <div
                v-if="isInvite(n) && n.rsvp"
                class="np-actions"
                data-testid="notification-rsvp"
                :data-rsvp="n.rsvp"
                @click.stop
              >
                <button
                  type="button"
                  class="kit-btn small"
                  :class="{ primary: n.rsvp === 'pending', chosen: n.rsvp === 'accepted' }"
                  :aria-pressed="n.rsvp === 'accepted'"
                  data-testid="notification-accept"
                  @click="answer(n, 'accepted')"
                >
                  <n-icon v-if="n.rsvp === 'accepted'" :component="CheckmarkOutline" :size="13" />
                  {{
                    n.rsvp === 'accepted'
                      ? t('notifications.pop.accepted')
                      : t('notifications.pop.accept')
                  }}
                </button>
                <button
                  type="button"
                  class="kit-btn small"
                  :class="{ chosen: n.rsvp === 'declined' }"
                  :aria-pressed="n.rsvp === 'declined'"
                  data-testid="notification-decline"
                  @click="answer(n, 'declined')"
                >
                  {{
                    n.rsvp === 'declined'
                      ? t('notifications.pop.declined')
                      : t('notifications.pop.decline')
                  }}
                </button>
                <span v-if="inviteWhen(n)" class="np-when">{{ inviteWhen(n) }}</span>
              </div>
              <div v-else-if="isConflict(n)" class="np-actions" @click.stop>
                <button
                  type="button"
                  class="kit-btn small primary"
                  data-testid="notification-resolve"
                  @click="resolveConflict(n)"
                >
                  {{ t('notifications.pop.resolve') }}
                </button>
              </div>
            </div>
          </div>
        </template>
      </template>
      <empty-state
        v-if="!visible.length && !pager?.loading.value"
        :icon="NotificationsOutline"
        :text="filter === 'all' ? t('shell.tools.feedEmpty') : t('notifications.pop.emptyFilter')"
        size="small"
      />
      <!-- The page's end: the next page loads as this comes into view; the
           button is there for a reader who cannot scroll it into view. -->
      <div v-if="pager" class="np-more" data-testid="notifications-more">
        <template v-if="!pager.done.value">
          <span ref="sentinel" class="np-sentinel" />
          <button
            type="button"
            class="kit-btn small"
            :disabled="pager.loading.value"
            data-testid="notifications-load-more"
            @click="pager.loadMore()"
          >
            {{ pager.loading.value ? t('notifications.pop.loading') : t('notifications.pop.more') }}
          </button>
        </template>
        <span v-else-if="visible.length" class="np-end">{{ t('notifications.pop.end') }}</span>
        <span v-if="pager.error.value" class="np-err">{{ pager.error.value }}</span>
      </div>
    </div>

    <footer v-if="!page && !all" class="np-foot">
      <span class="np-hint">
        <ShortcutKeys id="notifications.select" joined />{{ t('notifications.pop.select') }}
      </span>
      <span class="np-hint">
        <ShortcutKeys id="notifications.open" />{{ t('notifications.pop.open') }}
      </span>
      <span class="np-hint">
        <ShortcutKeys id="notifications.read" />{{ t('notifications.pop.read') }}
      </span>
      <span class="np-spacer" />
      <button type="button" class="np-all" data-testid="notifications-all" @click="openAll">
        {{ t('notifications.pop.all') }}
        <n-icon :component="ChevronForwardOutline" :size="12" />
      </button>
    </footer>
  </div>
</template>

<style scoped>
/* The feed card: popover level, radius 16, the popover shadow. */
.np {
  display: flex;
  flex-direction: column;
  width: 420px;
  max-width: calc(100vw - 16px);
  max-height: min(600px, calc(100vh - 80px));
  border-radius: 16px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  overflow: hidden;
}
.np-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 14px 10px 16px;
}
.np-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--t-text1);
}
.np-count {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
}
.np-spacer {
  flex: 1;
}
.np-quiet {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 6px 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.np-quiet:hover:not(:disabled) {
  background: var(--t-hover);
  color: var(--t-text1);
}
.np-quiet:disabled {
  opacity: 0.45;
  cursor: default;
}
.np-icon {
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
.np-icon:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.np-icon.on {
  background: var(--t-active);
  color: var(--t-primary);
}

/* «Не беспокоить» on (#3117): a soft accent banner over the filters. */
.np-dnd {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 12px 8px;
  padding: 8px 8px 8px 10px;
  border-radius: 12px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 10%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 20%, var(--t-pop))
  );
}
.np-dnd-ic {
  flex: none;
  color: var(--t-primary);
}
.np-dnd-t {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.np-dnd-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.np-dnd-sub {
  font-size: 12px;
  color: var(--t-text3);
}

/* Filters: a segmented control on the sunken level. */
.np-tabs {
  display: grid;
  grid-auto-columns: 1fr;
  grid-auto-flow: column;
  gap: 2px;
  margin: 0 12px 6px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-sub);
}
.np-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.np-tab:hover {
  color: var(--t-text1);
}
.np-tab.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 600;
  box-shadow: var(--t-sh-card);
}
.np-tab-n {
  font-size: 11px;
  color: var(--t-primary);
  font-weight: 700;
}

.np-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px 8px;
}
.np-group {
  display: flex;
  justify-content: space-between;
  padding: 10px 8px 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.np-group-n {
  letter-spacing: 0;
}
/* A row is its own surface only when unread (a soft accent) or under the
   pointer / keyboard cursor; a read row lies flat. */
.np-row {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 4px;
  padding: 10px 10px 10px 16px;
  border-radius: 12px;
  cursor: pointer;
  outline: none;
}
.np-row:hover,
.np-row.cur {
  background: var(--t-hover);
}
.np-row.unread {
  background: color-mix(in srgb, var(--t-primary) 10%, var(--t-pop));
}
.np-row.unread:hover,
.np-row.unread.cur {
  background: color-mix(in srgb, var(--t-primary) 16%, var(--t-pop));
}
.np-row.cur {
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--t-primary) 60%, transparent);
}
.np-dot {
  position: absolute;
  left: 6px;
  top: 22px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--t-primary);
}
.np-av,
.np-tile {
  flex: none;
  width: 32px;
  height: 32px;
  border-radius: 50%;
}
.np-av {
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: var(--t-accent-grad);
}
.np-tile {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
}
.np-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
.np-line {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  width: 100%;
}
.np-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.45;
  color: var(--t-text2);
}
.np-text b {
  font-weight: 650;
  color: var(--t-text1);
}
.np-row.unread .np-text {
  color: var(--t-text1);
}
.np-age {
  flex: none;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.np-quote {
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--t-text2);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.np-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 100%;
  height: 22px;
  padding: 0 8px;
  box-sizing: border-box;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
}
.np-chip-ref {
  color: var(--t-text3);
}
.np-chip-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text1);
}
/* A deleted task (#3114): named, but there is nothing left to open. */
.np-chip.gone .np-chip-t {
  color: var(--t-text3);
  text-decoration: line-through;
}
.np-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}
/* The answer given (#3051): the chosen button stays as a pressed pill. */
.np-actions .kit-btn.chosen {
  background: color-mix(in srgb, var(--t-primary) 14%, var(--t-pop));
  color: var(--t-primary);
  font-weight: 600;
}
.np-when {
  font-size: 12px;
  color: var(--t-text3);
}
.np-more {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 10px 0 6px;
}
.np-sentinel {
  display: block;
  width: 1px;
  height: 1px;
}
.np-end,
.np-err {
  font-size: 12px;
  color: var(--t-text3);
}
.np-err {
  color: var(--t-error, #e0533d);
}

.np-foot {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 9px 10px;
  background: var(--t-foot);
  font-size: 12px;
  color: var(--t-text3);
}
.np-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11.5px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
}
/* «Все уведомления» never gives way (#3107): the key hints before it are the
   ones that shrink. */
.np-all {
  flex: none;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 24px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.np-all:hover {
  background: var(--t-hover);
  color: var(--t-primary);
}

/* ── the page of all notifications on a wide window (#3049) ── */
.np.all:not(.page) {
  width: 100%;
  max-width: 760px;
  max-height: none;
  height: 100%;
  margin: 0 auto;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}
.np.all:not(.page) .np-head {
  padding: 22px 12px 14px;
}
.np.all:not(.page) .np-title {
  font-size: 20px;
}
.np.all:not(.page) .np-tabs {
  margin: 0 0 8px;
}
.np.all:not(.page) .np-list {
  padding: 0 0 24px;
}
@media (hover: none) {
  .np-hint {
    display: none;
  }
}

/* ── a phone: the feed is the page (#2976 — package G «Уведомления») ── */
.np.page {
  width: auto;
  max-width: none;
  height: 100%;
  max-height: none;
  border-radius: 0;
  background: var(--t-bg);
  box-shadow: none;
}
.np.page .np-head {
  min-height: 56px;
  box-sizing: border-box;
  padding: calc(6px + var(--safe-top)) 8px 6px 4px;
  gap: 4px;
}
.np-back {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--t-text1);
  cursor: pointer;
}
.np.page .np-title {
  font-size: 18px;
}
.np.page .np-quiet {
  white-space: nowrap;
  height: 44px;
  padding: 0 10px;
  color: var(--t-primary);
  font-size: 14px;
  font-weight: 650;
}
.np.page .np-quiet:disabled {
  color: var(--t-text3);
}
.np.page .np-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
}
.np.page .np-tabs {
  margin: 4px 14px 8px;
}
.np.page .np-tab {
  height: 36px;
  font-size: 13.5px;
}
.np.page .np-list {
  padding: 0 10px 16px;
}
.np.page .np-group {
  padding: 14px 8px 8px;
}
.np.page .np-row {
  min-height: 64px;
  box-sizing: border-box;
  padding: 12px 12px 12px 18px;
  margin-bottom: 6px;
  border-radius: 14px;
}
.np.page .np-row.unread {
  background: color-mix(in srgb, var(--t-primary) 10%, var(--t-card));
}
.np.page .np-text {
  font-size: 14.5px;
}
.np.page .np-av,
.np.page .np-tile {
  width: 36px;
  height: 36px;
}
</style>
