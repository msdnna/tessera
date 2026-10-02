/**
 * Button — Naive UI's NButton under Tessera's theme (accent colour, 8px radius,
 * contrast text on primary). `type="primary"` is the accent action; `tertiary` /
 * `quaternary` are the quiet toolbar buttons used across boards and modals.
 * @category Primitives
 */
import { NButton } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Button = vueIsland(NButton, {
  name: 'Button',
  display: 'inline-flex',
  events: { onClick: 'click' },
  slots: { children: 'default', icon: 'icon' },
})
