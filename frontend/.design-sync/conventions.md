# Tessera design system — conventions

Tessera is a task tracker (tasks, notes, reminders). Its killer feature is a kanban
board whose columns can be **grouped by tag**. Layout: project tree sidebar on the
left, the board in the centre, every entity (task, project, board, members,
settings) edited in a centred **Modal**. Russian is the product language — write
UI copy in Russian unless asked otherwise.

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

Use the variables, never raw hex, for neutrals:

| Variable                                | Use                               |
| --------------------------------------- | --------------------------------- |
| `--t-bg`                                | page / board canvas               |
| `--t-surface`                           | cards, modals, panels             |
| `--t-surface-alt`                       | columns, inputs, secondary panels |
| `--t-hover`                             | hover fill                        |
| `--t-border`                            | hairlines (use sparingly)         |
| `--t-text1` / `--t-text2` / `--t-text3` | primary / body / muted text       |
| `--t-placeholder`                       | placeholders                      |
| `--t-primary` / `--t-on-primary`        | accent / text on accent           |
| `--t-error` / `--t-error-grad`          | error text / error fill gradient  |

**Accent gradient (the signature of the language).** Every non-neutral element —
primary buttons, chips, tags, avatars, priority marks, accent text/icons, active
tabs — carries a soft same-hue diagonal gradient: darker bottom-left → base colour
at the centre → lighter top-right. Neutral greys stay flat.

- Accent: `background: var(--t-accent-grad)` (fills), `var(--t-accent-grad-subtle)`
  (thin accents), `var(--t-accent-grad-vert)` (tall narrow bars).
- Accent text: `className="accent-grad-text"`; accent icons: `<Icon gradient />`.
- Any other hue: `hueGrad('#2f80ed')` / `hueGradVert(c)` / `softFill(c)` (exported).
- Tags carry the gradient on TEXT + border, never as a filled box — use `TagPill`.

Radii: 8px controls · 10–12px cards (TaskCard 12px) · 14px board columns.
Priority colours (`PRIORITY_COLORS`, 0..4): `#9aa0aa` none · `#5b9bd5` low ·
`#3aa675` normal · `#e0a418` high · `#e0533d` urgent. Icons: ionicons5 outline via
`<Icon name="CalendarClearOutline" />`; board/status glyphs via `TesseraIcon`.
Font: the system UI stack (no brand webfont in the app UI).

## Kanban board recipe (how KanbanBoard lays out the real components)

Row of columns, `display:flex; gap:12px; align-items:flex-start` on `--t-bg`.
Each column: 280px wide, 10px padding, 14px radius; background
`color-mix(in srgb, <col> 6%, var(--t-surface-alt))`; 1px border
`color-mix(in srgb, <col> 12%, var(--t-border))`; a 3px TOP bar painted with
`hueGrad(<col>)` (transparent `border-top` + gradient on `border-box`). Inside:
`<ColumnHeader column={…} count={n} editable first|isDone style={{marginBottom:8}} />`
then `<TaskCard>`s (each carries its own 8px bottom margin). Sidebar project badge:
22px box, 5px radius, `hueGrad(<project colour>)` fill, white `<ProjectIcon size={13}>`.

## Where the truth lives

- `styles.css` and its `@import`s (`_ds_bundle.css`) — every variable and helper class.
- `components/<group>/<Name>/<Name>.prompt.md` + `<Name>.d.ts` — per-component API.

## Example

```jsx
<TesseraProvider>
  <div style={{ display: 'flex', gap: 12, padding: 16, background: 'var(--t-bg)' }}>
    <div style={{ width: 280, padding: 10, borderRadius: 14, background: 'var(--t-surface-alt)' }}>
      <ColumnHeader
        column={{ id: 'doing', name: 'В процессе', color: '#2f80ed' }}
        count={1}
        editable
        style={{ marginBottom: 8 }}
      />
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
</TesseraProvider>
```
