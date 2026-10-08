<script setup>
// The notification bell and its feed (v2, #3107 — PackageE «NotifPop»): in the
// header on every screen. The feed spans every workspace (opening an item may
// switch to another one), so it belongs to the shell rather than to the
// current workspace.
//
// The feed is a 420px popover: a head with the unread count, «Прочитать все»
// (⇧E) and a way to the notification settings; filters «Все · Упоминания ·
// Назначения · GitLab»; rows grouped «Новые» / «Ранее». A row is the person
// (or the system's glyph), the sentence with the name in bold, what it is about
// as a chip or a quote, and the time; an unread row sits on a soft accent with
// a dot. Keys while it is open: ↑↓ pick, ↵ open, E mark read, ⇧E read all.
//
// The filters and groups are the client's own reading of the 50 rows the
// server sends — paging and a page of all notifications are #3049, accepting a
// meeting from here is #3051 (both visible stubs).
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { NIcon, NPopover, NTooltip, useMessage } from 'naive-ui'
import {
  NotificationsOutline,
  SettingsOutline,
  AlarmOutline,
  TimeOutline,
  GitBranchOutline,
  ChevronForwardOutline,
  DocumentTextOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/EmptyState.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import { notificationText } from '@/utils/notificationText'
import { notificationRoute } from '@/utils/notificationRoute'
import { notificationGroup, notificationAge, NOTIFICATION_FILTERS } from '@/utils/notificationFeed'
import { stubTask } from '@/utils/featureStubs'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import { useFormat } from '@/composables/useFormat'
import './kit/kit.css'

defineProps({ placement: { type: String, default: 'bottom-end' } })

const ws = useWorkspacesStore()
const notes = useNotificationsStore()
const router = useRouter()
const message = useMessage()
const { formatters } = useFormat()
const { t, te } = useI18n()

const rsvpTask = stubTask('notifications.rsvp')
const allTask = stubTask('notifications.all')

// Bound so opening a notification can close the feed before navigating — a
// conference invite (#2875) opens a whole page the popover would otherwise sit
// on top of. `g n` and other screens ask for the feed through `feedRequest`.
const showFeed = ref(false)
watch(
  () => notes.feedRequest,
  () => (showFeed.value = true),
)

// ── the filter and the groups ─────────────────────────────────────────
const filter = ref('all')
const visible = computed(() =>
  notes.items.filter((n) => filter.value === 'all' || notificationGroup(n) === filter.value),
)
const fresh = computed(() => visible.value.filter((n) => !n.read_at))
const earlier = computed(() => visible.value.filter((n) => n.read_at))
// The keyboard walks the rows in the order they are drawn: new first.
const ordered = computed(() => [...fresh.value, ...earlier.value])
// Unread per filter, for the counts on the tabs.
const unreadBy = computed(() => {
  const out = {}
  for (const n of notes.items) {
    if (n.read_at) continue
    const g = notificationGroup(n)
    out[g] = (out[g] || 0) + 1
  }
  return out
})

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
  if (p.excerpt) return { quote: p.excerpt }
  if (p.title && p.event !== 'task_due_soon' && p.event !== 'conference_invited')
    return { title: p.title, ref: n.task_number ?? p.task_number }
  return null
}
// A row without a person carries the system's glyph in its own tone.
const SYSTEM = {
  reminder: { icon: AlarmOutline, tone: '#f0a020' },
  due_soon: { icon: TimeOutline, tone: '#e0533d' },
  integration_sync: { icon: GitBranchOutline, tone: '#fc6d26' },
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

// ── actions ───────────────────────────────────────────────────────────
function openNotification(n) {
  // The toast travels as an argument because the store cannot reach useMessage()
  // (see stores/notifications.js): a refused mark rolls the row back to unread,
  // and the reader is told why instead of watching the dot reappear (#2932).
  notes.markRead(n.id, message)
  // Switch the active workspace if the task lives in another one, so the sidebar
  // tree reflects where the opened task actually is.
  if (n.workspace_id && n.workspace_id !== ws.currentId) {
    ws.selectWorkspace(n.workspace_id)
  }
  const to = notificationRoute(n)
  if (!to) return
  showFeed.value = false
  router.push(to)
}
function markOne(n) {
  if (n && !n.read_at) notes.markRead(n.id, message)
}
function markAll() {
  if (notes.unread) notes.markAllRead(message)
}
function openSettings() {
  showFeed.value = false
  router.push({ path: '/settings', query: { s: 'notifications' } })
}

// ── keys while the feed is open ───────────────────────────────────────
// The cursor is shown only once the keyboard is used: a mouse reader gets the
// hover level, not a second highlight that follows nothing they did.
const cursor = ref(-1)
const kbd = ref(false)
const list = ref(null)
watch(showFeed, (open) => {
  cursor.value = -1
  kbd.value = false
  if (open) window.addEventListener('keydown', onKey, true)
  else window.removeEventListener('keydown', onKey, true)
})
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
  <n-popover
    v-model:show="showFeed"
    trigger="click"
    :placement="placement"
    :show-arrow="false"
    raw
    class="np-pop"
  >
    <template #trigger>
      <button
        type="button"
        class="bell-btn"
        :class="{ on: showFeed }"
        :aria-label="t('shell.tools.notifications')"
        data-testid="notifications-bell"
        data-tour="footer-notifications"
      >
        <n-icon :component="NotificationsOutline" :size="18" />
        <span v-if="notes.unread" class="bell-n" data-testid="notifications-unread">{{
          notes.unread > 9 ? '9+' : notes.unread
        }}</span>
      </button>
    </template>

    <div class="np" data-testid="notifications-feed" @mousemove="kbd = false">
      <header class="np-head">
        <span class="np-title">{{ t('shell.tools.notifications') }}</span>
        <span v-if="notes.unread" class="np-count">{{ notes.unread }}</span>
        <span class="np-spacer" />
        <button
          type="button"
          class="np-quiet"
          :disabled="!notes.unread"
          data-testid="notifications-read-all"
          @click="markAll"
        >
          {{ t('shell.tools.markAllRead') }}
          <ShortcutKeys id="notifications.readAll" joined />
        </button>
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

      <div class="np-tabs" role="tablist">
        <button
          v-for="f in NOTIFICATION_FILTERS"
          :key="f"
          type="button"
          role="tab"
          class="np-tab"
          :class="{ on: filter === f }"
          :aria-selected="filter === f"
          :data-testid="`notifications-filter-${f}`"
          @click="filter = f"
        >
          {{ t(`notifications.pop.filter.${f}`) }}
          <span v-if="f !== 'all' && unreadBy[f]" class="np-tab-n">{{ unreadBy[f] }}</span>
        </button>
      </div>

      <div ref="list" class="np-list">
        <template v-for="grp in ['fresh', 'earlier']" :key="grp">
          <template v-if="(grp === 'fresh' ? fresh : earlier).length">
            <div class="np-group">
              <span>{{ t(`notifications.pop.${grp}`) }}</span>
              <span v-if="grp === 'fresh'" class="np-group-n">{{ fresh.length }}</span>
            </div>
            <div
              v-for="n in grp === 'fresh' ? fresh : earlier"
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
                <span v-else-if="chipOf(n)" class="np-chip">
                  <n-icon :component="DocumentTextOutline" :size="12" />
                  <span v-if="chipOf(n).ref" class="np-chip-ref">#{{ chipOf(n).ref }}</span>
                  <span class="np-chip-t">{{ chipOf(n).title }}</span>
                </span>
                <!-- RSVP from the feed is #3051: visible, inactive. -->
                <div v-if="isInvite(n)" class="np-actions" @click.stop>
                  <n-tooltip :delay="300">
                    <template #trigger>
                      <span class="np-stub">
                        <button type="button" class="kit-btn small primary" disabled>
                          {{ t('notifications.pop.accept') }}
                        </button>
                        <button type="button" class="kit-btn small" disabled>
                          {{ t('notifications.pop.decline') }}
                        </button>
                      </span>
                    </template>
                    {{ t('common.stub.soon', { n: rsvpTask }) }}
                  </n-tooltip>
                </div>
              </div>
            </div>
          </template>
        </template>
        <empty-state
          v-if="!visible.length"
          :icon="NotificationsOutline"
          :text="filter === 'all' ? t('shell.tools.feedEmpty') : t('notifications.pop.emptyFilter')"
          size="small"
        />
      </div>

      <footer class="np-foot">
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
        <n-tooltip :delay="300">
          <template #trigger>
            <span class="np-all">
              {{ t('notifications.pop.all') }}
              <n-icon :component="ChevronForwardOutline" :size="12" />
            </span>
          </template>
          {{ t('common.stub.soon', { n: allTask }) }}
        </n-tooltip>
      </footer>
    </div>
  </n-popover>
</template>

<style scoped>
/* The bell: a quiet header button like «Входящие» next to it; the count is the
   accent gradient. */
.bell-btn {
  position: relative;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.bell-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.bell-btn.on {
  background: color-mix(in srgb, var(--t-primary) 18%, transparent);
  color: var(--t-primary);
}
.bell-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.bell-n {
  position: absolute;
  top: 3px;
  right: 2px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
  pointer-events: none;
}

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
.np-actions {
  display: flex;
  gap: 6px;
}
.np-stub {
  display: inline-flex;
  gap: 6px;
}

.np-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  background: var(--t-foot);
  font-size: 12px;
  color: var(--t-text3);
}
.np-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}
.np-all {
  white-space: nowrap;
  min-width: 0;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--t-text3);
  cursor: default;
}
@media (hover: none) {
  .np-hint {
    display: none;
  }
}
</style>
