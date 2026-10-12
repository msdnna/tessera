/**
 * Input — Naive's NInput under Tessera's theme: text, password or auto-growing
 * textarea. Controlled via `value` + `onChange`, or uncontrolled via
 * `defaultValue`. `prefix`/`suffix` take icons.
 * @category Primitives
 */
import { NInput } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Input = vueIsland(NInput, {
  name: 'Input',
  model: {
    vueProp: 'value',
    vueEvent: 'update:value',
    reactProp: 'value',
    defaultProp: 'defaultValue',
    changeProp: 'onChange',
  },
  events: { onFocus: 'focus', onBlur: 'blur', onKeyup: 'keyup' },
  slots: { prefix: 'prefix', suffix: 'suffix' },
})
