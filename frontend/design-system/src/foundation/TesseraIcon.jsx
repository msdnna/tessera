/**
 * TesseraIcon — Tessera's own icon pack (ionicons-5 style, currentColor): board
 * view layouts (`layout-*`), kanban column statuses (`status-*`), task open modes
 * and text alignment. Sized in em like Icon.
 * @category Foundation
 */
import TesseraIconVue from '@/components/TesseraIcon.vue'
import { vueIsland } from '../runtime/island'

export const TesseraIcon = vueIsland(TesseraIconVue, {
  name: 'TesseraIcon',
  display: 'inline-flex',
})
