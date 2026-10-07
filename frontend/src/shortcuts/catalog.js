// The shortcut catalog (v2, #2956): every key the app answers to, in one list.
//
// The cheat sheet, the key hints in tooltips and menus, and the command rows of
// the ⌘K palette are all rendered from here, and the dispatcher reads the keys
// from here too — so a hint can never advertise a key the handler doesn't use.
// Handlers are attached at runtime by whichever component owns the action
// (`useShortcuts`), because most of them need that component's state.
//
// Fields:
//   id       stable name; the label is `shell.keys.<id>`
//   keys     chord sequence (see keymap.js); omitted for display-only rows
//   group    cheat-sheet section; omitted for hidden helper bindings
//   display  hand-written caps for rows that stand for several bindings
//   digit    caption of the digit range of a `digit` chord
//   command  shown in the palette's «Команды» section; the value is its icon
//   to       route a key-less command opens (palette only)
//   form     palette form a key-less command opens (palette only, #2982)
//   input    also fires while the cursor is in a text field
//   modal    also fires while a modal dialog is open
//   repeat   fires on auto-repeat (held arrow keys)
//   eager    fires on its own key even though longer sequences start with it
//   prio     wins over other entries completed by the same key (default 0)

export const GROUPS = [
  'nav',
  'sidebar',
  'board',
  'card',
  'task',
  'chart',
  'create',
  'palette',
  'view',
  'views',
  'project',
  'stage',
  'inbox',
]

export const CATALOG = [
  // ── navigation ──
  { id: 'nav.palette', keys: 'mod+k', group: 'nav', input: true, modal: true },
  { id: 'nav.goBoard', keys: 'g b', group: 'nav', command: 'GridOutline' },
  // «Входящие» (#2997): opens the panel with the focus in the list, and back.
  { id: 'nav.goInbox', keys: 'g i', group: 'nav', command: 'FileTrayOutline' },
  // The bell's feed (mentions, assignments, GitLab) — G I went to «Входящие».
  { id: 'nav.goNotifications', keys: 'g n', group: 'nav', command: 'NotificationsOutline' },
  // The page of the stage the open board is scoped to (#2993).
  { id: 'nav.goStage', keys: 'g e', group: 'nav' },
  { id: 'nav.goMine', keys: 'g m', group: 'nav', command: 'HomeOutline' },
  { id: 'nav.sidebar', keys: '[', group: 'nav', command: 'EyeOutline' },
  { id: 'nav.shortcuts', keys: '?', group: 'nav', command: 'KeypadOutline' },
  // Undo toasts (#2969): ⌘Z takes back the newest one from anywhere but a text
  // field; F6 moves focus onto it.
  // `modal`: a toast raised inside a large window (the «Проект» window's tag
  // table, #2990) is undone without leaving it.
  { id: 'undo.last', keys: 'mod+z', group: 'nav', modal: true },
  { id: 'toast.focus', keys: 'f6', group: 'nav' },

  // Pages: palette commands without a key of their own.
  { id: 'go.documents', command: 'DocumentsOutline', to: '/documents' },
  { id: 'go.milestones', command: 'TrailSignOutline', to: '/milestones' },
  { id: 'go.settings', command: 'SettingsOutline', to: '/settings' },

  // ── sidebar and spaces (#2969): the tree row under the keyboard cursor, and
  // the rail. They work on the selected row without opening its menu, and the
  // menu lists the same keys.
  { id: 'nav.goSidebar', keys: 'g s', group: 'sidebar' },
  { id: 'nav.goRail', keys: 'g w', group: 'sidebar' },
  // R works too (row.renameR); the hint shows the one key the mockup names.
  { id: 'row.rename', keys: 'f2', group: 'sidebar' },
  { id: 'row.renameR', keys: 'r' },
  { id: 'row.new', keys: 'shift+n', group: 'sidebar' },
  // ⇧F10 works too (row.menuF10).
  { id: 'row.menu', keys: '.', group: 'sidebar' },
  { id: 'row.menuF10', keys: 'shift+f10' },
  { id: 'row.open', keys: 'enter' },
  { id: 'row.icon', keys: 'i', group: 'sidebar' },
  { id: 'row.slug', keys: 'u', group: 'sidebar' },
  { id: 'row.move', keys: 'm', group: 'sidebar' },
  { id: 'row.copyLink', keys: 'mod+shift+c', group: 'sidebar' },
  { id: 'row.settings', keys: 'mod+,', group: 'sidebar' },
  { id: 'row.archive', keys: 'e', group: 'sidebar' },
  { id: 'row.delete', keys: 'mod+backspace', group: 'sidebar' },

  // ── board ──
  { id: 'board.addTask', keys: 'c', group: 'board' },
  { id: 'board.quick', keys: 'n', group: 'board', eager: true },
  { id: 'board.filter', keys: 'f', group: 'board' },
  { id: 'board.group', keys: 'shift+g', group: 'board' },
  // Stage selector in the sub-toolbar and the stage strip over the board (#2994).
  { id: 'board.stage', keys: 'shift+e', group: 'board', command: 'TrailSignOutline' },
  { id: 'board.stageStrip', keys: 'shift+h', group: 'board' },
  // Six equal views (#2970): Канбан · Список · Таймлайн · Гант · Календарь · Матрица.
  { id: 'board.layout', keys: 'v digit', group: 'board', digit: '1–6' },
  // The column under the pointer, or the selected card's (#2969): F2 renames in
  // the header, ⌫ deletes at once with an undo toast, ⌘⇧←/→ moves it.
  { id: 'col.rename', keys: 'f2', group: 'board' },
  { id: 'col.delete', keys: 'backspace', group: 'board' },
  { id: 'col.move', group: 'board', display: [['mod', 'shift', '←/→']] },
  { id: 'col.moveLeft', keys: 'mod+shift+arrowleft' },
  { id: 'col.moveRight', keys: 'mod+shift+arrowright' },
  {
    id: 'board.select',
    group: 'board',
    display: [['arrowup', 'arrowdown', 'arrowleft', 'arrowright']],
  },
  { id: 'card.up', keys: 'arrowup', repeat: true },
  { id: 'card.down', keys: 'arrowdown', repeat: true },
  { id: 'card.left', keys: 'arrowleft', repeat: true },
  { id: 'card.right', keys: 'arrowright', repeat: true },
  { id: 'card.clear', keys: 'escape' },

  // ── selected card ──
  { id: 'card.open', keys: 'enter', group: 'card' },
  { id: 'card.complete', keys: 'e', group: 'card' },
  { id: 'card.status', keys: 's', group: 'card' },
  { id: 'card.assignees', keys: 'a', group: 'card' },
  { id: 'card.priority', keys: 'p digit', group: 'card', digit: '0–4' },
  { id: 'card.priorityPick', keys: 'p' },
  { id: 'card.due', keys: 'd', group: 'card' },
  { id: 'card.tags', keys: 'l', group: 'card' },
  { id: 'card.subtask', keys: 'shift+n', group: 'card' },
  { id: 'card.move', group: 'card', display: [['mod', '←/→']] },
  { id: 'card.moveLeft', keys: 'mod+arrowleft' },
  { id: 'card.moveRight', keys: 'mod+arrowright' },

  // ── the open task (#2958): the modal's own keys. `modal` because the task
  // modal is a Naive dialog; the field keys mirror the selected card's, so the
  // cheat sheet lists only what is new here.
  { id: 'task.next', keys: 'j', group: 'task', modal: true },
  { id: 'task.prev', keys: 'k', group: 'task', modal: true },
  { id: 'task.close', keys: 'escape', group: 'task', modal: true },
  { id: 'task.complete', keys: 'e', modal: true },
  { id: 'task.status', keys: 's', modal: true },
  { id: 'task.assignees', keys: 'a', modal: true },
  { id: 'task.priority', keys: 'p digit', modal: true },
  { id: 'task.priorityPick', keys: 'p', modal: true },
  { id: 'task.due', keys: 'd', modal: true },
  { id: 'task.tags', keys: 'l', modal: true },
  { id: 'task.subtask', keys: 'shift+n', modal: true },
  // «Напоминания и заметки» of the open task (#2998): ⇧R offers the quick
  // moments (then 1–3 or D), ⇧M opens the capture with the task filled in.
  { id: 'task.remind', keys: 'shift+r', group: 'task', modal: true },
  { id: 'task.note', keys: 'shift+m', group: 'task', modal: true },

  // ── the Gantt / timeline views (#2959): the selected bar and the time axis ──
  { id: 'chart.today', keys: 't', group: 'chart' },
  { id: 'chart.scroll', group: 'chart', display: [['arrowleft', 'arrowright']] },
  { id: 'chart.zoom', group: 'chart', display: [['+', '−']] },
  { id: 'chart.select', group: 'chart', display: [['arrowup', 'arrowdown']] },
  { id: 'chart.shift', group: 'chart', display: [['shift', '←/→']] },
  { id: 'chart.open', keys: 'enter', group: 'chart' },
  { id: 'chart.left', keys: 'arrowleft', repeat: true },
  { id: 'chart.right', keys: 'arrowright', repeat: true },
  { id: 'chart.up', keys: 'arrowup', repeat: true },
  { id: 'chart.down', keys: 'arrowdown', repeat: true },
  { id: 'chart.zoomIn', keys: '=' },
  { id: 'chart.zoomInShift', keys: 'shift+=' },
  { id: 'chart.zoomOut', keys: '-' },
  { id: 'chart.shiftLeft', keys: 'shift+arrowleft', repeat: true },
  { id: 'chart.shiftRight', keys: 'shift+arrowright', repeat: true },
  { id: 'chart.clear', keys: 'escape' },

  // ── palette forms (#2969 / #2982): create commands walked through field by
  // field inside the palette — palette-only, no key of their own. `form` names
  // the form usePaletteForm opens.
  { id: 'form.project', command: 'FolderOpenOutline', form: 'project', group: 'create' },
  { id: 'form.board', command: 'GridOutline', form: 'board', group: 'create' },
  { id: 'form.group', command: 'FolderOutline', form: 'group', group: 'create' },
  { id: 'form.workspace', command: 'AppsOutline', form: 'workspace', group: 'create' },

  // ── quick create ──
  { id: 'create.task', keys: 'n t', group: 'create', command: 'AddOutline' },
  { id: 'create.reminder', keys: 'n r', group: 'create', command: 'AlarmOutline' },
  { id: 'create.note', keys: 'n m', group: 'create', command: 'DocumentTextOutline' },
  // While the quick-actions menu is open its items answer to the bare second key.
  { id: 'fab.task', keys: 't', prio: 1 },
  { id: 'fab.reminder', keys: 'r', prio: 1 },
  { id: 'fab.note', keys: 'm', prio: 1 },
  { id: 'fab.close', keys: 'escape', prio: 1 },

  // ── palette (handled inside the palette; listed for the cheat sheet) ──
  { id: 'palette.select', group: 'palette', display: [['arrowup', 'arrowdown']] },
  { id: 'palette.actions', group: 'palette', display: [['tab']] },
  { id: 'palette.open', group: 'palette', display: [['enter']] },
  { id: 'palette.prefix', group: 'palette', display: [['#', '@', '>']] },
  { id: 'palette.back', display: [['shift', 'tab']] },
  { id: 'palette.close', display: [['escape']] },
  { id: 'palette.newTab', display: [['mod', 'enter']] },
  { id: 'palette.copyLink', display: [['mod', 'shift', 'C']] },

  // ── view ──
  { id: 'view.theme', keys: 'shift+t', group: 'view', command: 'ContrastOutline' },
  { id: 'view.cardSize', keys: 'shift+c', group: 'view' },
  { id: 'view.collapse', keys: 'h', group: 'view' },
  // The board's view panel (#2986): one entry for every setting of the board.
  { id: 'view.panel', keys: 'shift+v', group: 'view', command: 'OptionsOutline' },

  // ── saved views (#2987): ⌘S updates the loaded view, ⌘⇧S saves the board as a
  // new one, ⌘⌫ drops the changes. Digits pick a view while the panel is open
  // (the chip's menu answers to them too). F2 / ⌫ / ⌥↑↓ work on the panel's row
  // under the keyboard cursor — the row catches them, these entries only list them.
  { id: 'views.update', keys: 'mod+s', group: 'views', command: 'SaveOutline' },
  { id: 'views.saveAs', keys: 'mod+shift+s', group: 'views', command: 'BookmarkOutline' },
  { id: 'views.reset', keys: 'mod+backspace', group: 'views', command: 'ArrowUndoOutline' },
  { id: 'views.pick', keys: 'digit', group: 'views', digit: '0–9' },
  { id: 'views.rename', group: 'views', display: [['f2']] },
  { id: 'views.delete', group: 'views', display: [['backspace']] },
  { id: 'views.move', group: 'views', display: [['alt', '↑/↓']] },

  // ── the «Проект» window (#2971): ⌘, opens it on the current project, digits
  // switch its tabs. The tag table's keys are caught by the table itself (they
  // only mean something on its rows); these entries list them.
  { id: 'project.window', keys: 'mod+,', group: 'project', command: 'SettingsOutline' },
  { id: 'project.tab', keys: 'digit', group: 'project', digit: '1–4', modal: true },
  { id: 'project.close', keys: 'escape', modal: true, prio: 2 },
  { id: 'tags.select', group: 'project', display: [['space'], ['shift', '↑/↓']] },
  { id: 'tags.rename', group: 'project', display: [['enter'], ['f2']] },
  { id: 'tags.new', group: 'project', display: [['n'], ['/']] },
  { id: 'tags.bulk', group: 'project', display: [['m'], ['c'], ['backspace']] },

  // ── «Входящие» (#2997): caught by the panel's list itself while it has the
  // focus (they mean something only on its records); listed for the sheet.
  { id: 'inbox.select', group: 'inbox', display: [['arrowup', 'arrowdown']] },
  { id: 'inbox.filter', group: 'inbox', display: [['1–4']] },
  { id: 'inbox.open', group: 'inbox', display: [['enter']] },
  { id: 'inbox.done', group: 'inbox', display: [['e']] },
  { id: 'inbox.snooze', group: 'inbox', display: [['h']] },
  { id: 'inbox.toTask', group: 'inbox', display: [['t']] },
  { id: 'inbox.toDoc', group: 'inbox', display: [['shift', 'D']] },
  { id: 'inbox.link', group: 'inbox', display: [['l']] },
  { id: 'inbox.delete', group: 'inbox', display: [['backspace']] },

  // ── the stage page (#2993) ──
  { id: 'stage.edit', keys: 'e', group: 'stage' },
  { id: 'stage.board', keys: 'b', group: 'stage' },
  { id: 'stage.close', keys: 'shift+x', group: 'stage' },
  { id: 'stage.back', keys: 'escape', group: 'stage' },
]

export const byId = new Map(CATALOG.map((e) => [e.id, e]))
