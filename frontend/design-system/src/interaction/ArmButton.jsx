/**
 * ArmButton — the two-step danger action as a row's icon button (a comment, a
 * file, a link in the task window): the first press arms it — red, «Удалить?»
 * with ↵ — the second does it. It disarms after 4 s, when the pointer or the
 * focus leaves, or on Esc. For a row whose only destructive action does not need
 * a whole menu; inside a menu use ActionMenu's `confirm` item. `armed` shows the
 * armed state.
 * @category Interaction
 */
import { defineComponent, h } from 'vue'
import * as ionicons from '@vicons/ionicons5'
import ArmButtonVue from '@/components/kit/ArmButton.vue'
import { vueIsland } from '../runtime/island'

const Arm = defineComponent({
  name: 'TdsArmButton',
  props: {
    title: { type: String, default: '' },
    confirm: { type: String, default: 'Удалить?' },
    icon: { type: String, default: 'TrashOutline' },
    armed: { type: Boolean, default: false },
  },
  emits: ['confirm'],
  setup(props, { emit }) {
    return () =>
      h(ArmButtonVue, {
        armedStill: props.armed,
        title: props.title,
        confirm: props.confirm,
        icon: ionicons[props.icon] || ionicons.TrashOutline,
        onConfirm: () => emit('confirm'),
      })
  },
})

export const ArmButton = vueIsland(Arm, {
  name: 'ArmButton',
  display: 'inline-flex',
  events: { onConfirm: 'confirm' },
})
