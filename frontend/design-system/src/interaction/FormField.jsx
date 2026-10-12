/**
 * FormField — one field of a PopoverForm: label (+ an aside on the right), the
 * control (children; an input with class `kf-input` gets the v2 well), then an
 * optional path preview («…/project/<preview>»), an error with the «Свободно: x
 * или ⌥↵» suggestion, and a quiet message.
 * @category Interaction
 */
import FormFieldVue from '@/components/kit/FormField.vue'
import { vueIsland } from '../runtime/island'

export const FormField = vueIsland(FormFieldVue, {
  name: 'FormField',
  events: { onSuggest: 'suggest' },
  slots: { children: 'default' },
  toProps: (p) => ({
    label: p.label ?? '',
    aside: p.aside ?? '',
    preview: p.preview ?? null,
    error: p.error ?? '',
    suggestion: p.suggestion ?? '',
    message: p.message ?? '',
  }),
})
