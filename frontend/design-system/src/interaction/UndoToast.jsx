/**
 * UndoToast — the undo toast of the v2 interaction model: a reversible action
 * happens at once and offers «Отменить ⌘Z» for 7 s; the 3px accent bar along
 * the bottom edge is the time left (grey while paused under the pointer).
 * status: 'live' · 'busy' («Отменяем…») · 'done' (green check, «…
 * восстановлена») · 'error' («Повторить ↵»). In the app they stack centred over
 * the work area, 24px above its bottom, the newest at the bottom.
 * @category Interaction
 */
import UndoToastVue from '@/components/kit/UndoToast.vue'
import { vueIsland } from '../runtime/island'

export const UndoToast = vueIsland(UndoToastVue, {
  name: 'UndoToast',
  display: 'inline-block',
  events: { onUndo: 'undo', onRetry: 'retry', onClose: 'close' },
  toProps: (p) => ({
    caption: p.caption ?? '',
    icon: p.icon ?? 'trash',
    status: p.status ?? 'live',
    progress: p.progress ?? 1,
    paused: !!p.paused,
    reduced: !!p.reduced,
    secondsLeft: p.secondsLeft ?? 7,
    older: !!p.older,
  }),
})
