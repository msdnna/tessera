/**
 * EmptyState — the shared "nothing here" block (v2): an ionicons5 glyph on a
 * 56px tile in a soft tone, an optional title and a line of text (the app
 * puts up to two actions under it). `size="small"` for inline panels (sidebar, search),
 * `medium` for full views; `tone` is the tile's hue (accent by default).
 * @category Feedback
 */
import * as ionicons from '@vicons/ionicons5'
import EmptyStateVue from '@/components/EmptyState.vue'
import { vueIsland } from '../runtime/island'

export const EmptyState = vueIsland(EmptyStateVue, {
  name: 'EmptyState',
  toProps: ({ icon, title, text, size, tone }) => ({
    ...(icon && ionicons[icon] ? { icon: ionicons[icon] } : {}),
    title,
    text,
    size,
    tone,
  }),
})
