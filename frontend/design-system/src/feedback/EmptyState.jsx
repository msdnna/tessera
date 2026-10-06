/**
 * EmptyState — the shared "nothing here" block: a muted ionicons5 glyph over a
 * line of secondary text. `size="small"` for inline panels (sidebar, search),
 * `medium` for full views.
 * @category Feedback
 */
import * as ionicons from '@vicons/ionicons5'
import EmptyStateVue from '@/components/EmptyState.vue'
import { vueIsland } from '../runtime/island'

export const EmptyState = vueIsland(EmptyStateVue, {
  name: 'EmptyState',
  toProps: ({ icon, text, size }) => ({
    ...(icon && ionicons[icon] ? { icon: ionicons[icon] } : {}),
    text,
    size,
  }),
})
