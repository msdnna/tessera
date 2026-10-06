/**
 * Switch — Naive's NSwitch; the active rail is the accent colour. Controlled via
 * `value` + `onChange`, or uncontrolled via `defaultValue`.
 * @category Primitives
 */
import { NSwitch } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Switch = vueIsland(NSwitch, {
  name: 'Switch',
  display: 'inline-flex',
  model: {
    vueProp: 'value',
    vueEvent: 'update:value',
    reactProp: 'value',
    defaultProp: 'defaultValue',
    changeProp: 'onChange',
  },
})
