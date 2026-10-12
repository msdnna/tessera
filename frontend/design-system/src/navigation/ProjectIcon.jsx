/**
 * ProjectIcon — the glyph of a project/board in the sidebar tree: one of 16
 * curated ionicons (`briefcase`, `code`, `rocket`…), a custom SVG/image, or the
 * project's initials. Only the glyph — the 22px coloured badge box around it
 * belongs to the call site (see the conventions header).
 * @category Navigation
 */
import ProjectIconVue from '@/components/ProjectIcon.vue'
import { vueIsland } from '../runtime/island'

export const ProjectIcon = vueIsland(ProjectIconVue, {
  name: 'ProjectIcon',
  display: 'inline-flex',
})
