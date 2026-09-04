<script setup>
// A conference's recordings (#2877, parent #2864).
//
// Sits under the room on the conference page. Everyone who may enter the call
// may see this list — the recording *is* the meeting, and whoever could have
// attended can watch it back — but only a moderator may delete a row, which
// mirrors exactly who may start one.
//
// The list is refetched on a nudge from the parent rather than polled: the
// backend broadcasts `conference.recording.started` / `.finished` to the
// workspace, and the parent already listens to that stream for everything else
// on this page.
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NPopconfirm, NSpin } from 'naive-ui'
import { DownloadOutline, TrashOutline, WarningOutline } from '@vicons/ionicons5'
import { conferences as confApi } from '@/api'
import { useFormat } from '@/composables/useFormat'

const props = defineProps({
  conferenceId: { type: String, required: true },
  // Who may delete a recording — the same test as who may start one, decided by
  // the parent page (creator, host, or a workspace manager).
  canModerate: { type: Boolean, default: false },
  // Bumped by the parent when the server says a recording started or finished.
  nudge: { type: Number, default: 0 },
})

const { formatDateTime } = useFormat()

const rows = ref([])
const loading = ref(false)
// Failures are printed in place rather than toasted: this panel is below the
// fold of a call screen, and a toast about a list the user is not looking at is
// noise. The download button's own failure lands here too.
const error = ref('')
const busyId = ref('')

// Nothing to show and nothing being recorded: no empty frame under the room.
// A team that never records should not carry a permanent "Записей пока нет".
const show = computed(() => rows.value.length > 0)

async function load() {
  if (!props.conferenceId) return
  loading.value = true
  try {
    const { data } = await confApi.recordings(props.conferenceId)
    rows.value = data || []
    error.value = ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

/**
 * Fetches an mp4 with our bearer credential and saves it under its own name.
 *
 * The same blob dance as the chat attachments next door, and for the same
 * reason: an `<a href>` straight at the API carries no Authorization header, and
 * the alternative — a public URL guarded only by being unguessable — is not what
 * a private meeting should be protected by.
 */
async function download(rec) {
  if (busyId.value) return
  busyId.value = rec.id
  try {
    const { data } = await confApi.recording(rec.id)
    const url = URL.createObjectURL(data)
    const a = document.createElement('a')
    a.href = url
    a.download = rec.file_name || 'recording.mp4'
    a.click()
    // Revoked late, not immediately: revoking before the browser has started the
    // download cancels it — and these files are large enough for that gap to be
    // real rather than theoretical.
    setTimeout(() => URL.revokeObjectURL(url), 60000)
    error.value = ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    busyId.value = ''
  }
}

async function remove(rec) {
  try {
    await confApi.removeRecording(rec.id)
    rows.value = rows.value.filter((r) => r.id !== rec.id)
    error.value = ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

function humanSize(bytes) {
  if (!bytes) return ''
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${Math.round(bytes / 1024 / 1024)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

/** h:mm:ss, dropping the hour when there is none — meetings run both lengths. */
function humanDuration(sec) {
  if (!sec) return ''
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  const mm = String(m).padStart(h ? 2 : 1, '0')
  return h ? `${h}:${mm}:${String(s).padStart(2, '0')}` : `${mm}:${String(s).padStart(2, '0')}`
}

function when(ts) {
  return formatDateTime(ts, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })
}

function expiryDate(ts) {
  return formatDateTime(ts, { day: '2-digit', month: 'short', year: 'numeric' })
}

watch(() => props.conferenceId, load, { immediate: true })
watch(() => props.nudge, load)

defineExpose({ load })
</script>

<template>
  <div v-if="show" class="recs" data-testid="conference-recordings">
    <div class="recs-h">{{ $t('conferences.rec.title') }}</div>

    <div v-if="error" class="recs-err" data-testid="conference-recordings-error">
      <n-icon :component="WarningOutline" :size="14" />
      <span>{{ error }}</span>
    </div>

    <n-spin :show="loading">
      <div class="rec-rows">
        <div v-for="r in rows" :key="r.id" class="rec-row" data-testid="conference-recording-row">
          <div class="rec-main">
            <div class="rec-title">{{ when(r.started_at) }}</div>
            <div class="rec-meta">
              <!-- An active row is the recording currently running, listed here so
                   the panel does not look empty while the first one is being made. -->
              <span v-if="r.status === 'active'" class="rec-live">
                {{ $t('conferences.rec.live') }}
              </span>
              <!-- The server's own reason, not a generic "не удалось": whoever
                   comes back for a lost meeting deserves to know why it is lost. -->
              <span v-else-if="r.status === 'failed'" class="rec-failed">
                {{ $t('conferences.rec.failed') }}{{ r.error ? ` — ${r.error}` : '' }}
              </span>
              <template v-else>
                <span v-if="r.duration_sec">{{ humanDuration(r.duration_sec) }}</span>
                <span v-if="r.size_bytes">{{ humanSize(r.size_bytes) }}</span>
              </template>
              <span v-if="r.started_by_name">
                {{ $t('conferences.rec.by', { name: r.started_by_name }) }}
              </span>
              <!-- Said out loud rather than left to a settings page: a file that
                   deletes itself on a date is not something to discover after it
                   is gone. A row with no expiry is kept indefinitely (ttl 0). -->
              <span v-if="r.expires_at" class="rec-exp">
                {{ $t('conferences.rec.expires', { when: expiryDate(r.expires_at) }) }}
              </span>
            </div>
          </div>

          <n-button
            v-if="r.status === 'completed'"
            size="tiny"
            quaternary
            :loading="busyId === r.id"
            data-testid="conference-recording-download"
            @click="download(r)"
          >
            <template #icon><n-icon :component="DownloadOutline" /></template>
            {{ $t('conferences.rec.download') }}
          </n-button>

          <!-- No delete while it is still being written: the row is the only
               handle on a running egress worker, and dropping it would leave the
               worker recording into a file nothing points at. Stop it first. -->
          <n-popconfirm
            v-if="canModerate && r.status !== 'active'"
            :positive-button-props="{ type: 'error' }"
            :positive-text="$t('conferences.rec.delete')"
            @positive-click="remove(r)"
          >
            <template #trigger>
              <n-button text size="tiny" type="error" data-testid="conference-recording-delete">
                <n-icon :component="TrashOutline" />
              </n-button>
            </template>
            {{ $t('conferences.rec.confirmDelete') }}
          </n-popconfirm>
        </div>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.recs {
  margin-top: 16px;
}
.recs-h {
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--t-text2);
}
.recs-err {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  font-size: 12px;
  color: #d03050;
}
.rec-rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rec-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
}
.rec-main {
  flex: 1;
  min-width: 0;
}
.rec-title {
  font-size: 13px;
  color: var(--t-text1);
}
.rec-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--t-text3);
}
.rec-live {
  color: #d03050;
}
.rec-failed {
  color: #d03050;
}
.rec-exp {
  font-style: italic;
}
</style>
