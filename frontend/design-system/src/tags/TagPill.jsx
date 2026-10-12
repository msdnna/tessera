/**
 * Tag chip — the unit of Tessera's killer feature (kanban columns can be grouped
 * by tag). Paints the tag's own hue as a soft same-hue gradient on the TEXT and
 * the border; the box itself is never filled with the gradient. A `scope::name`
 * tag shows its scope as a muted leading segment.
 * @category Tags
 */
import TagPillVue from '@/components/TagPill.vue'
import { vueIsland } from '../runtime/island'

export const TagPill = vueIsland(TagPillVue, { name: 'TagPill', display: 'inline-flex' })
