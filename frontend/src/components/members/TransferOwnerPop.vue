<script setup>
// «Передать владение…» (#3072, MembersWin role menu): a popover form at the
// member's row with the workspace's name typed in — the irreversible-action
// rule of the v2 model (#2969: two steps or the name). Lives inside the members
// window's NModal, hence AnchorLayer + the inline form (a teleport to <body>
// would lose the modal's focus trap).
//
// The server holds the real guard (only the owner may transfer); the typed name
// only stops a slip of the hand.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AnchorLayer from '@/components/kit/AnchorLayer.vue'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import { workspaces as wsApi } from '@/api'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  wsId: { type: String, default: '' },
  // The workspace's name as the reader sees it (personal spaces are localised).
  wsName: { type: String, default: '' },
  member: { type: Object, default: null },
})
const emit = defineEmits(['update:show', 'done'])
const { t } = useI18n()

const typed = ref('')
const busy = ref(false)
const error = ref('')
watch(
  () => props.show,
  (on) => {
    if (!on) return
    typed.value = ''
    error.value = ''
  },
)
const matches = computed(() => !!props.wsName && typed.value.trim() === props.wsName.trim())
const who = computed(() => props.member?.name || props.member?.email || '')

async function submit() {
  if (!matches.value || busy.value || !props.member) return
  busy.value = true
  error.value = ''
  try {
    await wsApi.transferOwner(props.wsId, props.member.user_id)
    emit('update:show', false)
    emit('done', props.member)
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AnchorLayer
    :show="show"
    :anchor="anchor"
    placement="bottom-end"
    :offset="{ x: 0, y: 6 }"
    focus=".kf-input"
    :title="t('shell.members.transferTitle', { name: who })"
    testid="transfer-owner-layer"
    @update:show="emit('update:show', $event)"
  >
    <PopoverForm
      inline
      :show="show"
      :width="340"
      :title="t('shell.members.transferTitle', { name: who })"
      :subtitle="t('shell.members.transferSub')"
      :primary="t('shell.members.transferGo')"
      :busy="busy"
      :disabled="!matches"
      danger
      testid="transfer-owner"
      @update:show="emit('update:show', $event)"
      @submit="submit"
    >
      <FormField
        :label="t('shell.members.transferType')"
        :error="error"
        :message="t('shell.members.transferHint')"
      >
        <input
          v-model="typed"
          class="kf-input"
          :placeholder="wsName"
          :disabled="busy"
          autocomplete="off"
          spellcheck="false"
          data-testid="transfer-owner-name"
        />
      </FormField>
    </PopoverForm>
  </AnchorLayer>
</template>
