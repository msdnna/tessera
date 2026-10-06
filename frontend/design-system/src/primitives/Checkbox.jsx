/**
 * Checkbox — Naive's NCheckbox with an optional label (children). Controlled via
 * `checked` + `onChange`, or uncontrolled via `defaultChecked`.
 * @category Primitives
 */
import { NCheckbox } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Checkbox = vueIsland(NCheckbox, {
  name: 'Checkbox',
  display: 'inline-flex',
  model: {
    vueProp: 'checked',
    vueEvent: 'update:checked',
    reactProp: 'checked',
    defaultProp: 'defaultChecked',
    changeProp: 'onChange',
  },
  slots: { children: 'default' },
})
