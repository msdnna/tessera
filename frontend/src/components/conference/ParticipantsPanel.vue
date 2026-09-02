<script setup>
// Who is in the call, and what can be done about them (#2864, subtask #2872).
//
// The panel deliberately mixes two sources that look alike and are not:
//
//   - the roster comes from the room socket (useConfRoom). It is the server's
//     truth about presence, roles, raised hands and force-mutes.
//   - the volume controls come from the media transport (useConfTransport) and
//     never leave this browser.
//
// Keeping that line visible is the point of the layout below: the slider and the
// local mute sit under the name, while the two host actions are pushed to their
// own row with a confirmation. "I can't hear them" and "nobody may hear them"
// must not be two buttons that look the same.
import { computed } from 'vue'
import { NButton, NIcon, NPopconfirm, NSlider, NTooltip } from 'naive-ui'
import {
  HandRightOutline,
  MicOffOutline,
  MicOutline,
  RemoveCircleOutline,
  VideocamOutline,
  VolumeHighOutline,
  VolumeMuteOutline,
} from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'
import { VOLUME_DEFAULT, VOLUME_MAX } from '@/composables/useConfTransport'

const props = defineProps({
  // Roster from the room socket: [{user_id, name, role, mic, cam, force_muted,
  // hand_at, conns}].
  people: { type: Array, default: () => [] },
  // Media descriptors from the transport, for the local volume controls.
  peers: { type: Array, default: () => [] },
  // My own user id — the one row that gets no controls at all.
  meId: { type: String, default: '' },
  // Whether the server welcomed me as a host. Mirrors its own check rather than
  // replacing it: a member who forces the buttons visible still gets refused.
  canModerate: { type: Boolean, default: false },
})

const emit = defineEmits(['volume', 'local-mute', 'kick', 'force-mute'])

// Media state is keyed by identity, which is the user id we mint tokens with.
const media = computed(() => {
  const out = {}
  for (const p of props.peers) out[p.id] = p
  return out
})

// Hands first, then the room's own order (join time). A raised hand is a request
// waiting on someone in this panel, so it should not need scrolling to notice.
const ordered = computed(() =>
  [...props.people].sort((a, b) => {
    if (!!a.hand_at !== !!b.hand_at) return a.hand_at ? -1 : 1
    if (a.hand_at && b.hand_at) return String(a.hand_at).localeCompare(String(b.hand_at))
    return 0
  }),
)

function volumeOf(id) {
  const m = media.value[id]
  return typeof m?.volume === 'number' ? m.volume : VOLUME_DEFAULT
}
function locallyMuted(id) {
  return !!media.value[id]?.localMuted
}
// Someone we have no media descriptor for is in the room socket but not (yet)
// publishing to the SFU — joining, or in the call with no devices. There is no
// audio to turn down, so the controls stay out rather than pretending.
function hasAudio(id) {
  return !!media.value[id] && id !== props.meId
}
</script>

<template>
  <div class="panel" data-testid="conference-participants">
    <div v-if="!ordered.length" class="none">{{ $t('conferences.panel.empty') }}</div>

    <div
      v-for="p in ordered"
      :key="p.user_id"
      class="row"
      :class="{ me: p.user_id === meId }"
      data-testid="conference-participant"
    >
      <div class="head">
        <user-avatar :user-id="p.user_id" :name="p.name" class="av" />
        <div class="who">
          <div class="name">
            <span class="label">{{ p.name }}</span>
            <n-tooltip v-if="p.hand_at">
              <template #trigger>
                <n-icon
                  :component="HandRightOutline"
                  :size="14"
                  class="hand"
                  data-testid="conference-hand-up"
                />
              </template>
              {{ $t('conferences.panel.handUp') }}
            </n-tooltip>
          </div>
          <div class="meta">
            <span class="role">{{ $t(`conferences.role.${p.role}`) }}</span>
            <span class="dot">·</span>
            <!-- Two different silences, and the panel says which: the badge
                 reads "muted" for their own choice and "muted by a host" when
                 it was taken away from them. -->
            <span v-if="p.force_muted" class="forced" data-testid="conference-forced">
              {{ $t('conferences.panel.forceMuted') }}
            </span>
            <span v-else-if="!p.mic" class="off">{{ $t('conferences.panel.micOff') }}</span>
            <span v-else class="on">{{ $t('conferences.panel.speaking') }}</span>
            <n-icon v-if="p.cam" :component="VideocamOutline" :size="13" class="cam" />
          </div>
        </div>
        <n-icon
          :component="p.mic && !p.force_muted ? MicOutline : MicOffOutline"
          :size="15"
          class="mic"
          :class="{ live: p.mic && !p.force_muted }"
        />
      </div>

      <!-- Local playback. Nothing here reaches the server or the other person. -->
      <div v-if="hasAudio(p.user_id)" class="local">
        <n-tooltip>
          <template #trigger>
            <n-button
              quaternary
              circle
              size="tiny"
              :data-testid="`conference-local-mute-${p.user_id}`"
              @click="emit('local-mute', p.user_id)"
            >
              <n-icon
                :component="locallyMuted(p.user_id) ? VolumeMuteOutline : VolumeHighOutline"
                :size="15"
              />
            </n-button>
          </template>
          {{
            locallyMuted(p.user_id)
              ? $t('conferences.panel.unmuteForMe')
              : $t('conferences.panel.muteForMe')
          }}
        </n-tooltip>
        <n-slider
          :value="volumeOf(p.user_id)"
          :min="0"
          :max="VOLUME_MAX"
          :step="0.05"
          :format-tooltip="(v) => `${Math.round(v * 100)}%`"
          class="vol"
          @update:value="(v) => emit('volume', p.user_id, v)"
        />
      </div>

      <!-- Moderation. Behind a confirmation because both are visible to the
           whole room and neither can be taken back by the person who did it. -->
      <div v-if="canModerate && p.user_id !== meId" class="mod">
        <n-button
          quaternary
          size="tiny"
          :data-testid="`conference-force-mute-${p.user_id}`"
          @click="emit('force-mute', p.user_id, !p.force_muted)"
        >
          {{
            p.force_muted ? $t('conferences.panel.unforceMute') : $t('conferences.panel.forceMute')
          }}
        </n-button>
        <n-popconfirm
          :positive-text="$t('conferences.panel.kick')"
          @positive-click="emit('kick', p.user_id)"
        >
          <template #trigger>
            <n-button
              quaternary
              size="tiny"
              type="error"
              :data-testid="`conference-kick-${p.user_id}`"
            >
              <template #icon>
                <n-icon :component="RemoveCircleOutline" />
              </template>
              {{ $t('conferences.panel.kick') }}
            </n-button>
          </template>
          {{ $t('conferences.panel.confirmKick', { name: p.name }) }}
        </n-popconfirm>
      </div>
    </div>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.none {
  padding: 12px 0;
  text-align: center;
  font-size: 12px;
  color: var(--t-text3);
}
.row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border-radius: 8px;
}
.row:hover {
  background: var(--t-hover);
}
/* My own row carries no controls, so the tint is what explains the empty space
   rather than it reading as something that failed to load. */
.row.me {
  background: var(--t-hover);
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.av {
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  font-size: 12px;
  color: #fff;
  background: var(--t-accent-grad);
}
.who {
  flex: 1;
  min-width: 0;
}
.name {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 13px;
}
.label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* The one warm colour in the panel: a raised hand is the only thing here that
   is waiting on a person rather than describing one. */
.hand {
  flex: none;
  color: #f0a020;
}
/* Wraps as a whole, and each badge stays intact: without the nowrap the rail's
   width lands mid-word and the roster reads "Участни / к". */
.meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px 5px;
  font-size: 11px;
  color: var(--t-text3);
}
.meta > span {
  white-space: nowrap;
}
.forced {
  color: #d03050;
}
.on {
  color: #18a058;
}
.mic {
  flex: none;
  color: var(--t-text3);
}
.mic.live {
  color: #18a058;
}
.cam {
  color: var(--t-text3);
}
.local {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 36px;
}
.vol {
  flex: 1;
}
/* Wraps rather than overflows. The rail has a fixed width and the two labels are
   half again as long in Russian as in English, so a single row fits in one
   locale and clips the kick button mid-word in the other — and a clipped
   destructive action is the one thing here that must never be ambiguous. The
   indent is dropped for the same reason: this row needs every pixel the row has,
   while the volume slider above it does not. */
.mod {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}
</style>
