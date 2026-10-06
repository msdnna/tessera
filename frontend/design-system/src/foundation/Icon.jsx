/**
 * Icon — an ionicons5 glyph (the app's only icon set, outline style) inside Naive's
 * NIcon, exactly how the Tessera UI draws icons. `gradient` paints it with the
 * accent gradient (the `.grad-icon` treatment of active/accent icons).
 * @category Foundation
 */
import { defineComponent, h } from 'vue'
import { NIcon } from 'naive-ui'
import * as ionicons from '@vicons/ionicons5'
import { vueIsland } from '../runtime/island'

const IonIcon = defineComponent({
  name: 'TdsIonIcon',
  props: {
    name: { type: String, required: true },
    size: { type: [Number, String], default: 16 },
    color: { type: String, default: undefined },
    gradient: { type: Boolean, default: false },
  },
  setup(props) {
    return () => {
      const glyph = ionicons[props.name] || ionicons.HelpCircleOutline
      return h(
        NIcon,
        { size: props.size, color: props.color, class: props.gradient ? 'grad-icon' : undefined },
        () => h(glyph),
      )
    }
  },
})

export const Icon = vueIsland(IonIcon, { name: 'Icon', display: 'inline-flex' })
