/**
 * PopoverForm — tier 1 of the v2 interaction model: a small form (1–3 fields)
 * opened at its anchor instead of a centred modal. Title as a question,
 * fields (FormField), a footer with the key hint, «Отмена Esc» and the primary
 * button with ↵. In the app it floats at its anchor without a scrim; here it is
 * drawn in place. `danger` paints the primary red (a type-the-name delete).
 * @category Interaction
 */
import PopoverFormVue from '@/components/kit/PopoverForm.vue'
import { vueIsland } from '../runtime/island'

export const PopoverForm = vueIsland(PopoverFormVue, {
  name: 'PopoverForm',
  events: { onSubmit: 'submit', onCancel: 'cancel', onSuggest: 'suggest' },
  slots: { children: 'default', foot: 'foot' },
  toProps: (p) => ({
    show: true,
    inline: true,
    width: p.width ?? 340,
    title: p.title ?? '',
    subtitle: p.subtitle ?? '',
    primary: p.primary ?? '',
    busyLabel: p.busyLabel ?? '',
    busy: !!p.busy,
    disabled: !!p.disabled,
    danger: !!p.danger,
    hint: p.hint ?? '',
  }),
})
