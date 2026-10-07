# Tessera design system — conventions

Tessera is a task tracker (tasks, notes, reminders). Its killer feature is a kanban
board whose columns can be **grouped by tag**. Layout: project tree sidebar on the
left, the board in the centre. Russian is the product language — write UI copy in
Russian unless asked otherwise.

**Fewer modals (v2 interaction model, #2969).** Four tiers, smallest first:
0 · **InlineEdit** — rename or name a new row in place (F2 / double click, ↵ saves,
Esc cancels); 1 · **PopoverForm** at its anchor, no scrim, 1–3 `FormField`s (on a
phone it becomes a bottom sheet) — create a project, change an address, a space's
appearance; 1′ · the same form walked through in the ⌘K palette; 2 · a big window
(`Modal`) only for big entities — the task, GitLab, members, background jobs,
commands, conflicts, help, the cheat sheet, the project window. Row menus are
**ActionMenu** (a key on every row). Reversible deletes happen at once and offer an
**UndoToast** (7 s, ⌘Z); irreversible ones take a second press (the armed red row)
or the typed name (a danger PopoverForm).

## Setup — always wrap in `TesseraProvider`

```jsx
<TesseraProvider mode="light" accent="purple" language="ru">
  {/* the whole screen */}
</TesseraProvider>
```

It publishes the `--t-*` colour variables and mounts the SVG gradients that
`<Icon gradient>` and priority flags paint with. Without it: light/purple
fallbacks only, and gradient icons render empty. Use ONE at the root; a nested
provider (e.g. `mode="dark"` on a panel) re-themes only its own subtree
(gradient SVG icons always follow the ROOT provider's accent).
`accent`: purple (default `#7c5cff`) · blue · teal · green · orange · red · pink.

Components are the real Vue components of the web app mounted inside React. Style
their **host** only (`className` / `style` for margin, width, flex) — never target
their internals. Interactions that would hit the Tessera API (inline edits, card
menus) are inert in a design.

## Styling idiom — CSS variables + the accent gradient

**v2 rule: no separator lines.** Blocks are told apart by surface level, spacing,
radius and soft shadows — never by 1px borders. A 1px line is allowed only as a
focus ring and as a tag's own outline.

Use the variables, never raw hex, for neutrals. Surface levels, lowest first:

| Variable                                | Use                                           |
| --------------------------------------- | --------------------------------------------- |
| `--t-rail` / `--t-sidebar`              | far-left rail / sidebar (a step below canvas) |
| `--t-bg`                                | canvas: board, topbar, page body              |
| `--t-col`                               | board column base (tinted per column)         |
| `--t-card` / `--t-card-hover`           | cards (= `--t-surface`)                       |
| `--t-pop`                               | popovers, menus, tooltips, modals             |
| `--t-ctl` (= `--t-input-bg`)            | control wells: fields, composer, segments     |
| `--t-pill`                              | neutral pill / chip fill                      |
| `--t-sub`                               | tray inside a card (subtasks)                 |
| `--t-foot`                              | sidebar footer capsule                        |
| `--t-hover` / `--t-hover-side`          | hover fill (content / sidebar)                |
| `--t-active`                            | active item (accent gradient over sidebar)    |
| `--t-drop`                              | drag-and-drop landing slot                    |
| `--t-kbd`                               | keyboard key cap                              |
| `--t-scrim`                             | dimmer under a modal / palette                |
| `--t-text1` / `--t-text2` / `--t-text3` | primary / body / muted text                   |
| `--t-placeholder`                       | placeholders                                  |
| `--t-primary` / `--t-on-primary`        | accent / text on accent                       |
| `--t-error` / `--t-error-grad`          | error text / error fill gradient              |
| `--t-border`                            | legacy hairline — avoid in new designs        |

Shadows by level: `--t-sh-card` (card) · `--t-sh-hover` (hovered card) ·
`--t-sh-pop` (popover, menu) · `--t-sh-drag` (dragged card, modal) · `--t-sh-drop`
(inset, landing slot).

**Accent gradient (the signature of the language).** Every non-neutral element —
primary buttons, chips, tags, avatars, priority marks, accent text/icons, active
tabs — carries a soft same-hue diagonal gradient: darker bottom-left → base colour
at the centre → lighter top-right. Neutral greys stay flat; never mix two hues in
one gradient.

- Accent: `background: var(--t-accent-grad)` (fills), `var(--t-accent-grad-subtle)`
  (thin accents), `var(--t-accent-grad-vert)` (tall narrow bars).
- Accent text: `className="accent-grad-text"`; accent icons: `<Icon gradient />`.
- Any other hue: `hueGrad('#2f80ed')` / `hueGradVert(c)` / `softFill(c)` (exported).
- Soft tinted pill of a hue (priority, overdue, status chip):
  `linear-gradient(to top right, color-mix(in srgb, <c> 11%, var(--t-card)), color-mix(in srgb, <c> 19%, var(--t-card)))`, text in `<c>`.
- Tags carry the gradient on TEXT + border, never as a filled box — use `TagPill`.

Radii (`--t-r-*`): capsule `--t-r-pill` for tags, chips, pills, avatars-with-count ·
`--t-r-ctl` 10px controls (fields, buttons, composer, segmented control; a segment
inside one is 7px) · 7px rows inside menus · `--t-r-tip` 10px tooltips/menus ·
`--t-r-card` 14px cards · `--t-r-pop` 16px popovers · `--t-r-col` 18px board columns.
Priority colours (`PRIORITY_COLORS`, 0..4): `#9aa0aa` none · `#5b9bd5` low ·
`#3aa675` normal · `#e0a418` high · `#e0533d` urgent. Icons: ionicons5 outline via
`<Icon name="CalendarClearOutline" />`; board/status glyphs via `TesseraIcon`.
Font: the system UI stack (no brand webfont in the app UI).

## Kanban board recipe (how KanbanBoard lays out the real components)

v2 «coloured lanes». Row of columns, `display:flex; gap:12px; align-items:flex-start`
on `--t-bg`. Each column: 256px wide (244px when grouped by tag), 8px padding, 18px
radius, `display:flex; flex-direction:column; gap:8px`; background
`color-mix(in srgb, <col> var(--t-col-tone), var(--t-col))` — no border, no top bar.
Status palette: К работе `#0eb0a9` · В процессе `#2f80ed` · На рассмотрении
`#f0a020` · Готово `#18a058`; a column with no colour uses neutral `#9aa0aa`.
Inside: `<ColumnHeader column={…} count={n} editable first|isDone />`, then the
`<TaskCard>`s wrapped in one `<div>` (each card carries its own 8px bottom margin).
Grouped by tag: `column={{ …, tag: { name, color } }}` + `prefixNames`, no `editable`.
A card picked from the keyboard (arrow keys) is `<TaskCard selected />` — draw it only
for keyboard-driven states, never as the result of a click.
Sidebar project badge: 20px box, 6px radius, `hueGrad(<project colour>)` fill, white
`<ProjectIcon size={12}>`; the active board row uses `--t-active` and shows the board's
task count right-aligned (12px, `--t-text3`).

## Where the truth lives

- `styles.css` and its `@import`s (`_ds_bundle.css`) — every variable and helper class.
- `components/<group>/<Name>/<Name>.prompt.md` + `<Name>.d.ts` — per-component API.

## Example

```jsx
<TesseraProvider mode="dark">
  <div style={{ display: 'flex', gap: 12, padding: 16, background: 'var(--t-bg)' }}>
    <div
      style={{
        width: 256,
        padding: 8,
        borderRadius: 18,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
        background: 'color-mix(in srgb, #2f80ed var(--t-col-tone), var(--t-col))',
      }}
    >
      <ColumnHeader
        column={{ id: 'doing', name: 'В процессе', color: '#2f80ed' }}
        count={1}
        editable
      />
      <div>
        <TaskCard
          task={{
            id: 'a1',
            number: 2950,
            title: 'Редизайн доски',
            priority: 3,
            tag_ids: ['t1'],
            assignee_ids: ['u1'],
          }}
          tags={[{ id: 't1', name: 'design', color: '#eb2f96' }]}
          members={[{ user_id: 'u1', name: 'Мария Соколова' }]}
        />
      </div>
    </div>
  </div>
</TesseraProvider>
```
