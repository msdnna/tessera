/**
 * Tooltip — Naive's NTooltip (the app's hover hint, on the raised surface colour):
 * wraps its children as the trigger and shows `content` on hover. Pass `show` to
 * pin it open.
 * @category Primitives
 */
import { NTooltip } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Tooltip = vueIsland(NTooltip, {
  name: 'Tooltip',
  display: 'inline-flex',
  slots: { children: { name: 'trigger', box: true }, content: 'default' },
})
