/**
 * Modal — Naive's NModal with the card preset: Tessera manages every entity
 * (task, project, board, members, settings) in a centred modal over a dimmed
 * backdrop. Controlled via `show` + `onClose`; uncontrolled via `defaultShow`.
 * @category Primitives
 */
import { NModal } from 'naive-ui'
import { vueIsland } from '../runtime/island'

export const Modal = vueIsland(NModal, {
  name: 'Modal',
  toProps: ({
    title,
    preset = 'card',
    bordered = false,
    closable = true,
    width = 560,
    maskClosable,
    segmented,
    size,
  }) => ({
    title,
    preset,
    bordered,
    closable,
    maskClosable,
    segmented,
    size,
    style: {
      width: typeof width === 'number' ? `${width}px` : width,
      maxWidth: 'calc(100vw - 32px)',
    },
  }),
  model: {
    vueProp: 'show',
    vueEvent: 'update:show',
    reactProp: 'show',
    defaultProp: 'defaultShow',
    changeProp: 'onClose',
  },
  slots: { children: 'default', footer: 'footer', headerExtra: 'header-extra' },
})
