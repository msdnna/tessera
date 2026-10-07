/**
 * ColorIconPicker — «Иконка и цвет» of the v2 popover forms: seven colours and
 * eight icons in one well, the first icon being the initials of `name`.
 * `set="project"` or `"space"` picks the icon row. Keys while focused: ←/→
 * colour, ↑/↓ icon. Controlled: pass `color`/`icon` and update them from
 * `onColorChange`/`onIconChange`.
 * @category Interaction
 */
import ColorIconPickerVue from '@/components/kit/ColorIconPicker.vue'
import { vueIsland } from '../runtime/island'

export const ColorIconPicker = vueIsland(ColorIconPickerVue, {
  name: 'ColorIconPicker',
  events: { onColorChange: 'update:color', onIconChange: 'update:icon' },
  toProps: (p) => ({
    color: p.color ?? '#7c5cff',
    icon: p.icon ?? '',
    name: p.name ?? '',
    set: p.set ?? 'project',
  }),
})
