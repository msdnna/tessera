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
  'list',
  'calendar',
  'matrix',
  'task',
  'editor',
  'chart',
  'create',
  'palette',
  'view',
  'views',
  'project',
  'window',
  'stage',
  'inbox',
  'notifications',
  'meet',
  'settings',
  'admin',
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
  // Background jobs (#2975, JobsWin) — administrators only; the window itself
  // registers nothing for anyone else.
  { id: 'nav.goJobs', keys: 'g j', group: 'nav' },
  { id: 'nav.sidebar', keys: '[', group: 'nav', command: 'EyeOutline' },
  { id: 'nav.shortcuts', keys: '?', group: 'nav', command: 'KeypadOutline' },
  // The help centre (#3107, PackageE «HelpWin»); `modal` so F1 closes it too.
  { id: 'nav.help', keys: 'f1', group: 'nav', command: 'HelpCircleOutline', modal: true },
  // Undo toasts (#2969): ⌘Z takes back the newest one from anywhere but a text
  // field; F6 moves focus onto it.
  // `modal`: a toast raised inside a large window (the «Проект» window's tag
  // table, #2990) is undone without leaving it.
  { id: 'undo.last', keys: 'mod+z', group: 'nav', modal: true },
  { id: 'toast.focus', keys: 'f6', group: 'nav' },

  // Pages: palette commands. Documents and settings got a key of their own in
  // v2 (#3106, PackageE): G D, G «,».
  { id: 'go.documents', keys: 'g d', group: 'nav', command: 'DocumentsOutline', to: '/documents' },
  { id: 'go.milestones', command: 'TrailSignOutline', to: '/milestones' },
  { id: 'go.settings', keys: 'g ,', group: 'nav', command: 'SettingsOutline', to: '/settings' },

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

  // ── the «Список» view (#3020): rows under the keyboard cursor. ↵ E P D A L S
  // and ⌘←/→ are the selected card's keys (above), C the board's; these are the
  // list's own.
  { id: 'list.select', group: 'list', display: [['arrowup', 'arrowdown']] },
  { id: 'list.extend', group: 'list', display: [['shift', '↑/↓']] },
  { id: 'list.fold', group: 'list', display: [['arrowleft', 'arrowright']] },
  { id: 'list.rename', keys: 'f2', group: 'list' },
  { id: 'list.reorder', group: 'list', display: [['mod', '↑/↓']] },
  { id: 'list.up', keys: 'arrowup', repeat: true },
  { id: 'list.down', keys: 'arrowdown', repeat: true },
  { id: 'list.extendUp', keys: 'shift+arrowup', repeat: true },
  { id: 'list.extendDown', keys: 'shift+arrowdown', repeat: true },
  { id: 'list.collapse', keys: 'arrowleft' },
  { id: 'list.expand', keys: 'arrowright' },
  { id: 'list.moveUp', keys: 'mod+arrowup' },
  { id: 'list.moveDown', keys: 'mod+arrowdown' },

  // ── the «Календарь» view (#3019): a day cursor, and a task picked inside it
  // (↵). E P D A L S on the picked task are the selected card's keys.
  { id: 'cal.today', keys: 't', group: 'calendar' },
  {
    id: 'cal.days',
    group: 'calendar',
    display: [['arrowleft', 'arrowright', 'arrowup', 'arrowdown']],
  },
  { id: 'cal.period', group: 'calendar', display: [['shift', '←/→']] },
  { id: 'cal.pick', group: 'calendar', display: [['enter']] },
  { id: 'cal.shift', group: 'calendar', display: [['shift', '←/→']] },
  { id: 'cal.add', group: 'calendar', display: [['C']] },
  { id: 'cal.left', keys: 'arrowleft', repeat: true },
  { id: 'cal.right', keys: 'arrowright', repeat: true },
  { id: 'cal.up', keys: 'arrowup', repeat: true },
  { id: 'cal.down', keys: 'arrowdown', repeat: true },
  { id: 'cal.prev', keys: 'shift+arrowleft', repeat: true },
  { id: 'cal.next', keys: 'shift+arrowright', repeat: true },
  { id: 'cal.enter', keys: 'enter' },
  { id: 'cal.clear', keys: 'escape' },

  // ── the Eisenhower matrix (#2973): the arrows, ↵ and E P D A L S work on the
  // selected card as on the kanban; these pin it into a quadrant and back.
  { id: 'matrix.pin', group: 'matrix', display: [['mod', '←/→/↑/↓']] },
  { id: 'matrix.auto', keys: 'backspace', group: 'matrix' },
  { id: 'matrix.pinUp', keys: 'mod+arrowup' },
  { id: 'matrix.pinDown', keys: 'mod+arrowdown' },

  // ── the «Начало работы» tour and the spotlight hints (#3000). Shown on the
  // tour's popover and the hint card, not in the cheat sheet. `prio` above the
  // board's own ↵/←/Esc: while a step or a hint is on screen they answer to
  // it. The tour's ↵/← also work inside the task window (`modal`); its Esc
  // doesn't — there Esc closes the window, which is a step of «Основы».
  { id: 'tour.next', keys: 'enter', modal: true, prio: 3 },
  { id: 'tour.back', keys: 'arrowleft', modal: true, prio: 3 },
  { id: 'tour.abort', keys: 'escape', prio: 3 },
  { id: 'hint.ok', keys: 'enter', prio: 4 },
  { id: 'hint.close', keys: 'escape', prio: 4 },
  // «Что нового» (WhatsNewV2): ↵ is its main button; Esc is the window's own.
  { id: 'whatsnew.go', keys: 'enter', modal: true, prio: 5 },
  { id: 'whatsnew.later', display: [['escape']] },
  // «Обучение» from ⌘K: the address starts the tour (AppLayout reads `?tour=`).
  { id: 'help.tour', command: 'SchoolOutline', to: '/?tour=start' },

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
  // The description's editor: I (#3118, PackageJ «Описание»). ⇧E — its key of
  // #3113 — stays as a hidden alias, so the hand that learnt it still works.
  { id: 'task.editDesc', keys: 'i', group: 'task', modal: true },
  { id: 'task.editDescAlt', keys: 'shift+e', modal: true },
  // «Напоминания и заметки» of the open task (#2998): ⇧R offers the quick
  // moments (then 1–3 or D), ⇧M opens the capture with the task filled in.
  { id: 'task.remind', keys: 'shift+r', group: 'task', modal: true },
  { id: 'task.note', keys: 'shift+m', group: 'task', modal: true },

  // ── the Markdown editor (#3118, PackageJ «Композер», «Пузырь», «Полноэкранный»).
  // The formatting keys are caught by the field itself — utils/edKeys reads them
  // from here, so the bubble's tooltips and the cheat sheet name the key that
  // works. ⌘K belongs to the palette, hence ⌘⇧K for a link. Q quotes a selection
  // of read-only text into the comment composer while its bubble is up; ⌘1–3
  // switch the fullscreen editor's modes.
  { id: 'editor.bold', keys: 'mod+b', group: 'editor' },
  { id: 'editor.italic', keys: 'mod+i', group: 'editor' },
  { id: 'editor.strike', keys: 'mod+shift+x', group: 'editor' },
  { id: 'editor.code', keys: 'mod+e', group: 'editor' },
  { id: 'editor.link', keys: 'mod+shift+k', group: 'editor' },
  { id: 'editor.orderedList', keys: 'mod+shift+7', group: 'editor' },
  { id: 'editor.bulletList', keys: 'mod+shift+8', group: 'editor' },
  { id: 'editor.send', keys: 'mod+enter', group: 'editor' },
  { id: 'editor.quote', keys: 'q', group: 'editor', modal: true },
  { id: 'editor.write', keys: 'mod+1', group: 'editor' },
  { id: 'editor.split', keys: 'mod+2', group: 'editor' },
  { id: 'editor.preview', keys: 'mod+3', group: 'editor' },

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
  { id: 'tags.select', group: 'project', display: [['space'], ['shift', '↑/↓']] },
  { id: 'tags.rename', group: 'project', display: [['enter'], ['f2']] },
  { id: 'tags.new', group: 'project', display: [['n'], ['/']] },
  { id: 'tags.bulk', group: 'project', display: [['m'], ['c'], ['backspace']] },

  // ── large windows (#2975, kit/LargeWindow): the frame's own keys — digits
  // switch the sections of its navigation column, Esc steps out a layer at a
  // time — and the keys of the windows of package F. Each answers only while
  // its window is the top layer (`when`), so one letter can mean different
  // things in two windows (R: retry a push in the journal, run a worker in the
  // jobs). Keys caught by a list itself (↑↓, ↵, ⌥↑↓) are display rows.
  { id: 'win.section', keys: 'digit', group: 'window', digit: '1–9', modal: true },
  { id: 'win.close', keys: 'escape', group: 'window', modal: true, prio: 2 },
  { id: 'win.filter', keys: '/', group: 'window', modal: true },
  { id: 'win.new', keys: 'n', group: 'window', modal: true },
  { id: 'gitlab.sync', keys: 'shift+s', group: 'window', modal: true },
  // The binding form is one PUT of the whole binding: ⌘S saves it (also from a
  // field — the save bar names the key).
  { id: 'gitlab.save', keys: 'mod+s', modal: true, input: true },
  { id: 'journal.retry', keys: 'r', group: 'window', modal: true },
  { id: 'journal.resolve', keys: 'c', group: 'window', modal: true },
  { id: 'conflict.pick', group: 'window', display: [['B'], ['T'], ['G']] },
  { id: 'conflict.base', keys: 'b', modal: true },
  { id: 'conflict.ours', keys: 't', modal: true },
  { id: 'conflict.theirs', keys: 'g', modal: true },
  { id: 'conflict.merge', keys: 'm', group: 'window', modal: true },
  {
    id: 'conflict.task',
    group: 'window',
    display: [
      ['mod', '['],
      ['mod', ']'],
    ],
  },
  { id: 'conflict.prev', keys: 'mod+[', modal: true },
  { id: 'conflict.next', keys: 'mod+]', modal: true },
  { id: 'conflict.apply', keys: 'mod+enter', group: 'window', modal: true, input: true },
  { id: 'jobs.stop', keys: 'shift+backspace', group: 'window', modal: true },
  { id: 'jobs.run', keys: 'r', modal: true },
  { id: 'members.invite', keys: 'i', group: 'window', modal: true },
  { id: 'commands.edit', group: 'window', display: [['enter']] },
  { id: 'commands.move', group: 'window', display: [['alt', '↑/↓']] },

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

  // ── settings and administration (#3106) ──
  // Conferences (#3107, PackageE «Встречи»): the section answers N V / N J / ⇧J,
  // the call ⌘D / ⌘E / ⌘⇧H, and ⌘⇧M folds the call into the mini-window and back
  // from anywhere while it runs.
  { id: 'meet.schedule', keys: 'n v', group: 'meet' },
  { id: 'meet.instant', keys: 'n j', group: 'meet' },
  { id: 'meet.join', keys: 'shift+j', group: 'meet' },
  { id: 'meet.mic', keys: 'mod+d', group: 'meet' },
  { id: 'meet.cam', keys: 'mod+e', group: 'meet' },
  { id: 'meet.leave', keys: 'mod+shift+h', group: 'meet' },
  { id: 'meet.minimize', keys: 'mod+shift+m', group: 'meet' },
  // The bell's feed (#3107, PackageE «NotifPop»): the popover answers these
  // itself while it is open — display rows for the cheat sheet and its footer.
  { id: 'notifications.select', group: 'notifications', display: [['arrowup', 'arrowdown']] },
  { id: 'notifications.open', group: 'notifications', display: [['enter']] },
  { id: 'notifications.read', group: 'notifications', display: [['E']] },
  { id: 'notifications.readAll', group: 'notifications', display: [['shift', 'E']] },
  { id: 'settings.sections', group: 'settings', display: [['arrowup', 'arrowdown']] },
  { id: 'admin.search', keys: '/', group: 'admin' },
  { id: 'admin.rows', group: 'admin', display: [['arrowup', 'arrowdown']] },
  { id: 'admin.menu', group: 'admin', display: [['enter']] },
]

export const byId = new Map(CATALOG.map((e) => [e.id, e]))
