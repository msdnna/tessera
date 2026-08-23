import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { slashItems, filterSlashItems } from '../docSlash'

export const slashKey = new PluginKey('slashMenu')

// "/" only opens the menu at the start of a block or after whitespace, and the
// query stops at the next space or slash — so a URL typed inline ("a/b") and a
// date ("12/08") never turn into a command palette.
const TRIGGER = /(?:^|\s)\/([^\s/]*)$/

const CLOSED = { active: false, query: '', from: 0, to: 0, index: 0, items: [], closedFrom: null }

/**
 * The "/…" range the cursor currently sits at the end of, if any.
 *
 * @param {object} state editor state
 * @returns {{from:number,to:number,query:string}|null}
 */
export function slashRangeAt(state) {
  const { selection } = state
  if (!selection.empty) return null
  const { $from } = selection
  if (!$from.parent.isTextblock) return null
  // Inside a code block a slash is code, not a command.
  if ($from.parent.type.spec.code) return null

  const before = $from.parent.textBetween(0, $from.parentOffset, undefined, '￼')
  const match = TRIGGER.exec(before)
  if (!match) return null

  const query = match[1]
  const to = selection.from
  return { from: to - query.length - 1, to, query }
}

/**
 * Menu state for the given editor state — what DocEditor renders.
 * @param {object} state editor state
 */
export function slashState(state) {
  return slashKey.getState(state) || CLOSED
}

/**
 * SlashMenu is the "/"-triggered insert palette (item 5 of #2718).
 *
 * Written by hand rather than on @tiptap/suggestion, which is not a dependency
 * of this project and cannot become one from this machine (the npm registry is
 * unreachable here). It is also less machinery than it looks: the plugin owns
 * the whole state — open/closed, the query, the filtered items and the
 * highlighted index — so the Vue component is a pure renderer and the keyboard
 * behaviour stays testable without mounting anything.
 */
export const SlashMenu = Extension.create({
  name: 'slashMenu',

  addOptions() {
    // Fired for items whose work happens outside the editor (the image picker).
    return { onExternal: null }
  },

  addCommands() {
    // Every command below writes to the transaction it was handed rather than
    // dispatching its own: a command that dispatches mid-chain leaves the
    // manager holding a transaction built on a state that no longer exists,
    // and ProseMirror rejects it ("Applying a mismatched transaction").
    const step = (delta) => () => (params) => {
      const { state, tr, dispatch } = params
      const s = slashState(state)
      if (!s.active || !s.items.length) return false
      if (dispatch)
        tr.setMeta(slashKey, { index: (s.index + delta + s.items.length) % s.items.length })
      return true
    }

    return {
      slashNext: step(1),
      slashPrev: step(-1),

      slashClose:
        () =>
        ({ state, tr, dispatch }) => {
          if (!slashState(state).active) return false
          if (dispatch) tr.setMeta(slashKey, { close: true })
          return true
        },

      slashRun:
        (item) =>
        ({ state, chain, dispatch }) => {
          const s = slashState(state)
          if (!s.active) return false
          const chosen = item || s.items[s.index]
          if (!chosen) return false
          if (!dispatch) return true

          const range = { from: s.from, to: s.to }
          if (chosen.external) {
            chain().deleteRange(range).run()
            this.options.onExternal?.(chosen)
            return true
          }
          // One chain, so the typed "/table" and the table it produced are a
          // single undo step.
          return !!chosen.apply(chain().deleteRange(range))
        },
    }
  },

  addKeyboardShortcuts() {
    const open = () => slashState(this.editor.state).active
    return {
      ArrowDown: () => open() && this.editor.commands.slashNext(),
      ArrowUp: () => open() && this.editor.commands.slashPrev(),
      Enter: () => open() && this.editor.commands.slashRun(),
      Tab: () => open() && this.editor.commands.slashRun(),
      Escape: () => open() && this.editor.commands.slashClose(),
    }
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: slashKey,
        state: {
          init: () => CLOSED,
          apply(tr, prev, _oldState, state) {
            const meta = tr.getMeta(slashKey)
            if (meta?.close) return { ...CLOSED, closedFrom: prev.from }
            if (meta?.index != null && prev.active) return { ...prev, index: meta.index }

            const range = slashRangeAt(state)
            if (!range) return prev.active || prev.closedFrom !== null ? { ...CLOSED } : prev
            // Escape dismissed this very slash: stay shut until the user moves
            // on, otherwise the next keystroke pops the menu straight back up.
            if (prev.closedFrom === range.from) return { ...CLOSED, closedFrom: prev.closedFrom }
            // Only typing opens the menu. Clicking the caret next to a "/" that
            // is already part of the text must not.
            if (!prev.active && !tr.docChanged) return prev

            const items = filterSlashItems(slashItems(), range.query)
            if (!items.length) return { ...CLOSED }
            const same = prev.active && prev.from === range.from
            return {
              ...range,
              active: true,
              items,
              index: same ? Math.min(prev.index, items.length - 1) : 0,
              closedFrom: null,
            }
          },
        },
      }),
    ]
  },
})
