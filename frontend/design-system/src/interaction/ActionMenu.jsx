/**
 * ActionMenu — the v2 row menu: rows with their key on the right, a colour row,
 * a switch row and a two-step danger item (the first press arms it — red,
 * «Точно удалить?» with the cost — the second fires). Item icons are ionicons5
 * export names ('CreateOutline'). In the app it floats at a row; here it is
 * drawn in place. `armed` shows a two-step item in its armed state.
 * @category Interaction
 */
import { defineComponent, h } from 'vue'
import * as ionicons from '@vicons/ionicons5'
import ActionMenuVue from '@/components/kit/ActionMenu.vue'
import { vueIsland } from '../runtime/island'

// Icon names → the ionicon components the Vue menu draws.
const withIcons = (items = []) =>
  items.map((it) => (typeof it.icon === 'string' ? { ...it, icon: ionicons[it.icon] } : it))

const Menu = defineComponent({
  name: 'TdsActionMenu',
  props: {
    items: { type: Array, default: () => [] },
    header: { type: String, default: '' },
    headerAside: { type: String, default: '' },
    width: { type: Number, default: 300 },
    armed: { type: String, default: '' },
  },
  emits: ['select'],
  setup(props, { emit }) {
    return () =>
      h(ActionMenuVue, {
        show: true,
        inline: true,
        items: withIcons(props.items),
        header: props.header,
        headerAside: props.headerAside,
        width: props.width,
        arm: props.armed,
        onSelect: (key, value) => emit('select', key, value),
      })
  },
})

export const ActionMenu = vueIsland(Menu, {
  name: 'ActionMenu',
  events: { onSelect: 'select' },
})
