/**
 * InlineEdit — tier 0 of the v2 interaction model: a row's or header's text
 * turns into a field in place (rename, or the name of a new row). ↵ saves,
 * Esc cancels, a click away saves; `error` keeps it open with a red ring and the
 * reason, `busy` swaps the ↵ cap for a spinner. `variant="row"` is a sidebar
 * tree row (28px field), `"header"` a column header (26px, semibold).
 * @category Interaction
 */
import InlineEditVue from '@/components/kit/InlineEdit.vue'
import { vueIsland } from '../runtime/island'

export const InlineEdit = vueIsland(InlineEditVue, {
  name: 'InlineEdit',
  display: 'flex',
  events: { onCommit: 'commit', onCancel: 'cancel' },
  slots: { trailing: 'trailing' },
  toProps: (p) => ({
    variant: p.variant ?? 'row',
    placeholder: p.placeholder ?? '',
    error: p.error ?? '',
    busy: !!p.busy,
    hint: p.hint ?? 'save',
    hintLeft: p.hintLeft ?? 0,
    showHint: p.showHint ?? true,
    autofocus: !!p.autofocus,
  }),
  model: {
    vueProp: 'modelValue',
    vueEvent: 'update:modelValue',
    reactProp: 'value',
    defaultProp: 'defaultValue',
    changeProp: 'onChange',
  },
})
