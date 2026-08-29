---
title: Customizing the board view
category: Boards
order: 23
keywords: customize, view, card size, fields, stack, board icon, board colour, autosave, collapse
updated: 2026-08-28
---

The gear button on the board bar opens the **Customize the view** panel on the right. It holds everything about how the board and its cards look: the board itself, how large the cards are and which fields to show on them.

![The “Customize the view” panel: sections from the board name down to views](../assets/board-customize-light.png)

The panel is available on the Kanban, the list, the calendar and the matrix. It isn't there on the Timeline and the Gantt: there are no cards in the usual sense there, so there is nothing whose size and fields to set.

The sections run top to bottom in the same order as below.

## Board

The board-name field edits it in place: Enter, or a click away, saves.

Below it is the picker for the **board's icon and colour**, the same one projects and groups use. The icon can be taken from the built-in set or uploaded; the mode switch decides whether to draw the icon on a badge plate or without one. With no icon chosen, the board shows the first two letters of its name.

The icon and the colour are visible in the project tree on the left — that's how you tell a dozen boards of one project apart by look rather than by reading names.

## Card size

Three options: **Compact**, **Medium** (the default) and **Expanded**. The size changes the density of a card — how much of it the title takes and how much is left for the fields.

Compact makes sense when coverage matters (a big board fitting on screen at once); expanded when the cards carry many fields and you need to read them without opening the task.

## Fields

This is where you decide what to show on a card at all. First two general switches:

- **Stack (in a column)** — lay the fields out vertically one under another instead of a wrapping row. Helps when there are many fields and the inline layout breaks in awkward places.
- **Show empty fields** — whether to draw a placeholder pill for a field that isn't filled in. On, and you can see a task has no due date; off, and the card is cleaner but the absence goes unnoticed.

Then a switch per field: **Priority**, **Due date**, **Assignee**, **Tags**, **Estimate**, **Milestone**, **Description**, **Number (#)** and **GitLab**. A field switched off only disappears from the cards: inside the task it stays, and it keeps taking part in filters and sorting.

A practical trick: on a board grouped by tags, switch the “Tags” field off. The tag is already written in the column header, and on the cards it is only noise.

## Columns

One switch — **Collapse empty columns**. Columns without tasks fold into narrow strips automatically as tasks leave them. Collapsing individual columns by hand doesn't go away; the two work together — see [Board columns](/help/board-columns).

## Grouping · sorting · filter

This section isn't a second set of settings but **the very same chips as on the board bar**, shown as a list. Sorting through a long set of conditions is easier here: the chips are spelled out in words (“Priority: high”) rather than as an icon plus a value. The `+` button opens the same add menu, the cross on a chip removes a condition, and clicking the grouping chip flips status ⇄ tags just the same.

The conditions themselves are covered in [The board bar](/help/board-composer).

The **Expand subtasks** switch lives here too — a duplicate of the branch icon on the bar.

## Views

**Autosave** writes changes made on the bar into the currently loaded saved view, so you don't have to press Save each time.

The switch is disabled until a view is loaded — and that's a meaningful detail, not an oversight: autosave needs something to save into. Load a view with the folder button and the switch's caption changes to “Autosave: <name>”.

The save and load buttons themselves live on the board bar, not here; the hint at the bottom of the section says as much. Details are in [Saved views](/help/board-saved-views).

## What of this goes into a view

Everything set in this panel — card size, stacking, empty fields, the visibility of each field, collapsed columns — is stored in a board view along with the grouping and the filters. The board's name, icon and colour are not: those are properties of the board itself, the same for every member, and independent of any view.
