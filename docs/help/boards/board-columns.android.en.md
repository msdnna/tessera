---
title: Board columns
category: Boards
order: 22
keywords: columns, statuses, drag, rename, colour, done column, collapse, delete, defaults, app
updated: 2026-08-28
---

A column is a task status. A new board starts with four columns, and on the phone you can change them exactly as you would in the browser: rename, colour, reorder, add and delete.

## The four default columns

Every new board gets **“To do” → “In progress” → “In review” → “Done”**, each with its own colour. The rightmost one (“Done”) is designated as the done column right away.

These four have a property yours will not have: their names **follow the interface language**. Switch the language and they translate themselves. As soon as you rename such a column by hand, that link is broken: from then on the column is called what you called it, in any language, and renaming it back does not restore the automatic translation.

## Columns are editable only when grouping by status

Everything below is available while the board is grouped **by status** — its normal state. Group the board by tag or by milestone and the headers stop being columns: the “⋯” menu and the “+ Column” tile disappear, and the names no longer open for editing. That is not a fault — a tag cannot be renamed “as a column”, it has its own [tag management](/help/tags).

Columns are not editable in the archive either: the [archive](/help/archive) is read-only.

## Renaming — one tap on the name

On the desktop this is a double click; on the phone a **single tap on the column name** is enough. The name turns into an input field right there in the header: edit it and confirm. A blank name is not saved — the column keeps its previous title.

Mind the mis-tap: a tap on the name lands in the editor rather than “on the column”, so the keyboard may appear unexpectedly. Cancelling puts everything back.

## Column menu: colour, done column, delete

The three dots on the right of the header open the column menu. It holds:

- **“Rename”** — the same input field as tapping the name;
- **a palette of eight colours** — purple, blue, teal, green, amber, red, pink and grey; the selected one has a ring around it. A tap colours the column immediately and closes the menu. The colour goes into the thin strip above the header, into the status icon, and as a barely visible wash into the column's fill and border;
- **the “Done column” switch** — see below;
- **“Delete”** — with confirmation.

There is no separate “remove colour” item: the grey swatch is the colour grey, not a reset.

## The done column

The done column is the one that means “the task is finished”: a card landing there is marked complete and the move is written into its history. It is a **property of the board**, not of the rightmost position: make a middle column the done one, reorder the columns, and the designation stays with it.

The switch works as a radio button with an undo: turning it on makes this column the done one (the previous one stops being it), and tapping the enabled switch again clears the explicit designation — the board falls back to its default rule and treats the **rightmost** column as done.

## Reordering columns

Column order is changed with a **long press on the header**: the column tears away, a copy follows your finger and the original stays in place, dimmed. Release it over the target and the neighbouring columns move aside into their new positions.

Press the header specifically: a long press on a card starts moving the card, not the column.

## Collapsing

The chevron on the left of the header collapses a column into a **narrow strip** about a finger wide, keeping the card count and the title turned on its side. The freed width is handed to the remaining columns — on a phone this is the main way to win space, so collapse anything you do not need right now.

A collapsed strip is still a full column: you can **drop a card onto it** and the task changes status without expanding it. Tapping the strip expands it back.

Empty columns can collapse by themselves — the auto-collapse switch lives in the [board appearance](/help/board-customize) sheet. Which columns are collapsed is remembered with the rest of the board's look and goes into a [saved view](/help/board-saved-views).

## Adding and deleting

At the end of the row of columns there is a **“+ Column”** tile with a dashed border. A tap turns it into an input field: type a name and the column appears at the end; an empty entry brings the tile back.

Deleting lives in the “⋯” menu and asks for confirmation with the column name — for good reason: **the column is deleted together with all of its tasks**, and they do not go to the archive. If you need those tasks, move them to another column first: on the phone the quickest way is to drop each card onto the collapsed strip of a neighbouring column.
