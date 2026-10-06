/**
 * Card — Naive's NCard under Tessera's theme: a surface panel with an optional
 * title row (`title`, `headerExtra` on the right), body (children) and `footer`.
 * @category Primitives
 */
import { NCard } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Card = vueIsland(NCard, {
  name: 'Card',
  slots: { children: 'default', headerExtra: 'header-extra', footer: 'footer', action: 'action' },
})
