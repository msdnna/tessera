/**
 * TagPicker — the v2 tag picker of a task (300px, mockup TagPicker): a search
 * field on top, «Теги задачи · N» (checked rows), «Все теги» / «Найдено» with the
 * task counts, «Создать тег «…» ⌘↵» when nothing is called that, «Управлять
 * тегами…» (the «Проект» window) and a key footer. In the app it floats at the
 * task modal's «Теги» row or a card's tag pill (a bottom sheet on a phone); here
 * it is drawn in place. `query` opens it with that search typed.
 * @category Tags
 */
import { defineComponent, h } from 'vue'
import TagPickerVue from '@/components/task/TagPicker.vue'
import { vueIsland } from '../runtime/island'

const Picker = defineComponent({
  name: 'TdsTagPicker',
  props: {
    tags: { type: Array, default: () => [] },
    selected: { type: Array, default: () => [] },
    prefixNames: { type: Object, default: () => ({}) },
    lockedPrefixes: { type: Array, default: () => [] },
    query: { type: String, default: '' },
    canManage: { type: Boolean, default: true },
    creating: { type: Boolean, default: false },
  },
  emits: ['toggle', 'create', 'manage'],
  setup(props, { emit }) {
    return () =>
      h(TagPickerVue, {
        show: true,
        inline: true,
        tags: props.tags,
        selected: props.selected,
        prefixNames: props.prefixNames,
        lockedPrefixes: props.lockedPrefixes.length ? props.lockedPrefixes : null,
        initialQuery: props.query,
        canManage: props.canManage,
        creating: props.creating,
        keyId: 'task.tags',
        create: async (name) => {
          emit('create', name)
          return false
        },
        onToggle: (id) => emit('toggle', id),
        onManage: (q) => emit('manage', q),
      })
  },
})

export const TagPicker = vueIsland(Picker, {
  name: 'TagPicker',
  display: 'inline-block',
  events: { onToggle: 'toggle', onCreate: 'create', onManage: 'manage' },
})
