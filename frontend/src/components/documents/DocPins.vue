<script setup>
// The document head's chips (#3032, mockup DocsPage «Спринт 42»): the stage the
// document belongs to (#3044) and the meeting it is the agenda of (#3046).
//
// A chip opens an ActionMenu at itself: an empty stage chip lists the stages
// straight away; a set one offers «Открыть», «Сменить» (a submenu) and
// «Открепить». The meeting chip is drawn only while the document has a meeting
// — pinning one starts from the document's «⋯» menu (openMeeting) or from the
// «Повестка встречи» template, which is born pinned.
//
// The chips' captions come from the lists the pickers need anyway (the
// workspace's stages and meetings), loaded once per workspace and only when a
// chip has something to show or a picker opens — the document carries ids only.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  CloseOutline,
  OpenOutline,
  SwapHorizontalOutline,
  TrailSignOutline,
  VideocamOutline,
} from '@vicons/ionicons5'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import { conferences as confApi, documents as docsApi, workspaces as wsApi } from '@/api'
import { useMessage } from '@/composables/useMessage'
import { useFormat } from '@/composables/useFormat'
import { sortStages, stagePath, stageState } from '@/utils/milestones'
import { meetingChoices, stageChoices } from '@/utils/docPins'

const props = defineProps({
  doc: { type: Object, required: true },
  wsId: { type: String, default: '' },
})
const emit = defineEmits(['update'])
const { t } = useI18n()
const router = useRouter()
const message = useMessage()
const { formatDateTime } = useFormat()

const stages = ref(null)
const meetings = ref(null)
let stagesFor = ''
let meetingsFor = ''

async function loadStages() {
  if (!props.wsId || stagesFor === props.wsId) return
  stagesFor = props.wsId
  try {
    stages.value = sortStages((await wsApi.milestones(props.wsId)).data || [])
  } catch {
    stagesFor = ''
    stages.value = []
  }
}
async function loadMeetings() {
  if (!props.wsId || meetingsFor === props.wsId) return
  meetingsFor = props.wsId
  try {
    meetings.value = (await confApi.list(props.wsId)).data || []
  } catch {
    meetingsFor = ''
    meetings.value = []
  }
}
watch(
  () => [props.wsId, props.doc?.milestone_id, props.doc?.conference_id],
  () => {
    if (props.wsId !== stagesFor) stages.value = null
    if (props.wsId !== meetingsFor) meetings.value = null
    if (props.doc?.milestone_id) loadStages()
    if (props.doc?.conference_id) loadMeetings()
  },
  { immediate: true },
)

const stage = computed(
  () => (stages.value || []).find((m) => m.id === props.doc?.milestone_id) || null,
)
const meeting = computed(
  () => (meetings.value || []).find((c) => c.id === props.doc?.conference_id) || null,
)
function meetingWhen(c) {
  const at = c?.scheduled_at || c?.started_at || c?.ended_at
  return at ? formatDateTime(at, { day: 'numeric', month: 'short' }) : ''
}

// ── menus ──
const menu = ref('')
const anchor = ref(null)
const stageEl = ref(null)
const meetingEl = ref(null)

async function openStage(el) {
  anchor.value = el || stageEl.value
  await loadStages()
  menu.value = 'stage'
}
async function openMeeting(el) {
  anchor.value = el || meetingEl.value
  await loadMeetings()
  menu.value = 'meeting'
}
defineExpose({ openStage, openMeeting })

const stageRows = computed(() =>
  stageChoices(stages.value || [], props.doc?.milestone_id, {
    noteOf: (m) => t(`documents.pins.stageState.${stageState(m)}`),
  }),
)
const meetingRows = computed(() =>
  meetingChoices(meetings.value || [], props.doc?.conference_id, { noteOf: meetingWhen }),
)
const empty = (key) => [{ hint: t(key) }]

const stageItems = computed(() => {
  const list = stageRows.value.length ? stageRows.value : empty('documents.pins.noStages')
  if (!stage.value) return list
  return [
    { key: 'open', label: t('documents.pins.openStage'), icon: OpenOutline },
    {
      key: 'change',
      label: t('documents.pins.changeStage'),
      icon: SwapHorizontalOutline,
      children: list,
      subWidth: 300,
    },
    { gap: true },
    { key: 'unpin', label: t('documents.pins.unpinStage'), icon: CloseOutline },
  ]
})
const meetingItems = computed(() => {
  const list = meetingRows.value.length ? meetingRows.value : empty('documents.pins.noMeetings')
  if (!props.doc?.conference_id) return list
  return [
    { key: 'open', label: t('documents.pins.openMeeting'), icon: OpenOutline },
    {
      key: 'change',
      label: t('documents.pins.changeMeeting'),
      icon: SwapHorizontalOutline,
      children: list,
      subWidth: 340,
    },
    { gap: true },
    { key: 'unpin', label: t('documents.pins.unpinMeeting'), icon: CloseOutline },
  ]
})

async function setStage(id) {
  try {
    const res = await docsApi.setMilestone(props.doc.id, id)
    emit('update', res.data)
  } catch (e) {
    message.error(e.message)
  }
}
async function setMeeting(id) {
  try {
    const res = await docsApi.setConference(props.doc.id, id)
    emit('update', res.data)
  } catch (e) {
    message.error(e.message)
  }
}

function onStage(key) {
  menu.value = ''
  if (key === 'open') {
    if (stage.value) router.push(stagePath(stage.value.project_slug, stage.value))
  } else if (key === 'unpin') setStage(null)
  else if (key.startsWith('ms:')) setStage(key.slice(3))
}
function onMeeting(key) {
  menu.value = ''
  if (key === 'open') router.push(`/conferences/${props.doc.conference_id}`)
  else if (key === 'unpin') setMeeting(null)
  else if (key.startsWith('conf:')) setMeeting(key.slice(5))
}
</script>

<template>
  <button
    ref="stageEl"
    type="button"
    class="pin"
    :class="{ empty: !stage }"
    :title="stage ? `${stage.project_name} · ${stage.title}` : t('documents.pins.pickStage')"
    data-testid="doc-milestone"
    @click="openStage($event.currentTarget)"
  >
    <n-icon :component="TrailSignOutline" :size="12" />
    <span class="pin-t">{{ stage ? stage.title : t('documents.view.milestone') }}</span>
  </button>
  <button
    v-if="doc.conference_id"
    ref="meetingEl"
    type="button"
    class="pin"
    :title="meeting ? `${meeting.title} · ${meetingWhen(meeting)}` : ''"
    data-testid="doc-meeting"
    @click="openMeeting($event.currentTarget)"
  >
    <n-icon :component="VideocamOutline" :size="12" />
    <span class="pin-t">{{ meeting ? meeting.title : t('documents.pins.meeting') }}</span>
    <span v-if="meeting && meetingWhen(meeting)" class="pin-when">{{ meetingWhen(meeting) }}</span>
  </button>

  <ActionMenu
    :show="menu === 'stage'"
    :anchor="() => anchor"
    placement="bottom-start"
    :width="300"
    :header="stage ? stage.title : t('documents.pins.pickStage')"
    :header-aside="stage ? stage.project_name : ''"
    :items="stageItems"
    testid="doc-stage-menu"
    @update:show="(v) => !v && (menu = '')"
    @select="onStage"
  />
  <ActionMenu
    :show="menu === 'meeting'"
    :anchor="() => anchor"
    placement="bottom-start"
    :width="340"
    :header="meeting ? meeting.title : t('documents.pins.pickMeeting')"
    :items="meetingItems"
    testid="doc-meeting-menu"
    @update:show="(v) => !v && (menu = '')"
    @select="onMeeting"
  />
</template>

<style scoped>
/* The mockup's meta pills: 24px, --v-pill, text2, a 12px glyph. */
.pin {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 280px;
  height: 24px;
  padding: 0 9px 0 7px;
  border: 0;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.pin:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
.pin:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px var(--t-primary);
}
/* No stage yet: the chip is an invitation, quieter than a set one. */
.pin.empty {
  background: transparent;
  color: var(--t-text3);
}
.pin.empty:hover {
  background: var(--t-hover);
}
.pin-t {
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pin-when {
  flex: none;
  color: var(--t-text3);
}
</style>
