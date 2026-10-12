/**
 * MarkdownEditor — Tessera's own Markdown editor for task descriptions and
 * comments: textarea + formatting toolbar + Write/Preview tabs, @-mentions and
 * /-commands. `variant="v2"` is the task-description edit block (bar with
 * Написать · Просмотр and the tools, ringed field, footer with Отмена Esc ·
 * Сохранить ⌘↵); `variant="composer"` the 46px comment row (avatar, field, «Aa»,
 * «Прикрепить», «Отправить ⌘↵»); `boxed` the framed note composer. Stores plain
 * Markdown.
 * @category Editor
 */
import MarkdownEditorVue from '@/components/MarkdownEditor.vue'
import { vueIsland } from '../runtime/island'

export const MarkdownEditor = vueIsland(MarkdownEditorVue, {
  name: 'MarkdownEditor',
  model: {
    vueProp: 'modelValue',
    vueEvent: 'update:modelValue',
    reactProp: 'value',
    defaultProp: 'defaultValue',
    changeProp: 'onChange',
  },
  events: { onSubmit: 'submit' },
  toProps: (p) => ({
    placeholder: p.placeholder ?? null,
    mentionItems: p.mentionItems || [],
    minRows: p.minRows ?? 3,
    initialMode: p.initialMode || 'write',
    variant: p.variant || 'default',
    send: !!p.send,
    sending: !!p.sending,
    toolbar: p.toolbar ?? true,
    expandable: false,
    split: !!p.split,
    avatar: p.avatar ?? null,
    saveLabel: p.saveLabel ?? null,
    sendLabel: p.sendLabel ?? null,
    footer: p.footer ?? true,
  }),
})
