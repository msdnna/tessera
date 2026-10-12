/**
 * Tabs — Naive's NTabs as a tab bar: `tabs` lists { name, label }; the active tab
 * is underlined in the accent (`line`) or raised (`segment`). Render the active
 * tab's content yourself below it. Controlled via `value` + `onChange`, or
 * uncontrolled via `defaultValue`.
 * @category Primitives
 */
import { defineComponent, h } from 'vue'
import { NTabPane, NTabs } from 'naive-ui'
import { vueIsland } from '../runtime/island'

const TabBar = defineComponent({
  name: 'TdsTabBar',
  props: {
    tabs: { type: Array, default: () => [] },
    value: { type: [String, Number], default: undefined },
    type: { type: String, default: 'line' },
    size: { type: String, default: 'medium' },
    justifyContent: { type: String, default: undefined },
  },
  emits: ['update:value'],
  setup(props, { emit }) {
    return () =>
      h(
        NTabs,
        {
          value: props.value ?? props.tabs[0]?.name,
          type: props.type,
          size: props.size,
          justifyContent: props.justifyContent,
          'onUpdate:value': (v) => emit('update:value', v),
        },
        () =>
          props.tabs.map((t) =>
            h(NTabPane, { key: t.name, name: t.name, tab: t.label, disabled: t.disabled }),
          ),
      )
  },
})

export const Tabs = vueIsland(TabBar, {
  name: 'Tabs',
  model: {
    vueProp: 'value',
    vueEvent: 'update:value',
    reactProp: 'value',
    defaultProp: 'defaultValue',
    changeProp: 'onChange',
  },
})
