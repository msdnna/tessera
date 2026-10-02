/**
 * Select — Naive's NSelect under Tessera's theme: single or multiple choice from
 * `options` ({ label, value }). Controlled via `value` + `onChange`, or
 * uncontrolled via `defaultValue`.
 * @category Primitives
 */
import { NSelect } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Select = vueIsland(NSelect, {
  name: 'Select',
  model: {
    vueProp: 'value',
    vueEvent: 'update:value',
    reactProp: 'value',
    defaultProp: 'defaultValue',
    changeProp: 'onChange',
  },
})
