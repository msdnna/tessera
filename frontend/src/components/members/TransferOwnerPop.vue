<script setup>
// «Передать владение…» (#3072, MembersWin role menu): the popover form drawn in
// place right under the member's row, with the workspace's name typed in — the
// irreversible-action rule of the v2 model (#2969: two steps or the name).
// In place rather than at an anchor: the members window is an NModal, and a
// layer teleported out of its card has its field's focus pulled back by the
// modal's focus trap (the e2e caught the typed name landing in the invite field).
//
// The server holds the real guard (only the owner may transfer); the typed name
// only stops a slip of the hand.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import { workspaces as wsApi } from '@/api'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  show: { type: Boolean, default: false },
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
    nextTick(() => field.value?.focus())
  },
  { immediate: true },
)
const field = ref(null)
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
  <PopoverForm
    inline
    :show="show"
    :width="420"
    :title="t('shell.members.transferTitle', { name: who })"
    :subtitle="t('shell.members.transferSub')"
    :primary="t('shell.members.transferGo')"
    :busy="busy"
    :disabled="!matches"
    danger
    class="tr-pop"
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
        ref="field"
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
</template>

<style scoped>
.tr-pop {
  margin: 4px 0 8px auto;
}
</style>
