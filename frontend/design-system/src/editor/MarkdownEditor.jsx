/**
 * MarkdownEditor — Tessera's own Markdown editor for task descriptions and
 * comments: textarea + formatting toolbar + Write/Preview tabs, @-mentions and
 * /-commands. `variant="boxed"` is the framed comment composer with a bottom
 * toolbar and send button. Stores plain Markdown.
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
  }),
})
