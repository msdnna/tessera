---
title: Board columns
category: Boards
order: 22
keywords: columns, statuses, drag, rename, colour, completing column, collapse, delete, defaults
updated: 2026-08-28
---

A column is a task's status. A new board starts with four columns, and the set can be changed however you like: renamed, coloured, reordered, added to and deleted.

![A board column: the header with a counter and the settings menu](../assets/board-columns-light.png)

## The four default columns

Every new board gets **“To do” → “In progress” → “In review” → “Done”**, each with its own colour. The rightmost one (“Done”) is set as the completing column right away.

These four have a property the ones you create don't: their names **follow the interface language**. Switch the language and they translate themselves. The moment you rename such a column by hand, that link is cut: from then on the column is called what you called it, in every language. This is deliberate — your name matters more than an automatic translation — but renaming it back will not restore the behaviour.

## Editing columns works in status grouping only

Everything below applies while the board is grouped **by status**. Under grouping by tags, milestones or assignee the columns aren't board columns but a way of laying tasks out: they can't be renamed or deleted, because what stands behind them are tags and milestones, not statuses. To get editing back, flip the grouping with the chip in the [board bar](/help/board-composer).

## Dragging

A column is moved **by its header**: take it by the name or by the status glyph to its left (the cursor turns into a hand) and drag it to a new place. The column order is shared by everyone who opens the board.

## Renaming with a double click

**A double click on the name** turns the header into an input. Enter saves; clicking away saves too (the field commits on blur). An empty name, or one you didn't actually change, just closes the field.

The same entry lives in the context menu: **right-click the header** → “Rename”.

## Colour

The “…” button in the header opens the column settings. The top row is nine circles: “Default” and eight colours. The colour paints the column's top edge as a gradient strip, faintly tints its background and border, and colours the status glyph in the header. It doesn't reach the cards — a card carries its own colours (priority, tags).

## The completing column

**A task counts as done exactly when it lands in the completing column.** Not “the rightmost”, not “the one called Done”, but the one actually set as completing.

It's set in the same “…” menu with **Make it completing** (or by right-clicking the header). Pressing it again gives “Stop completing”. The completing column is marked by a solid filled status glyph in its header.

When a task enters the completing column, a completion date is stamped and a “completed” event is written to its history; move it back out and the mark is removed, with a “reopened” event written.

If no column is set explicitly, the rightmost one counts as completing. Hence the most common confusion: **a column was moved, and tasks in a different column started counting as done**. If tasks aren't being marked complete even though they visually sit “at the end”, check in the “…” menu which column offers “Stop completing” — that one is it.

Deleting the completing column doesn't break the board, but it doesn't hand the role to a neighbour either: the explicit assignment is simply cleared, and the board goes back to treating the rightmost column as completing.

## Collapsing

The arrow button in the header (**Collapse the column**) folds the column into a narrow vertical strip — its tasks don't go anywhere, they just stop taking width. Clicking the strip (“Expand the column”) brings it back.

The collapsed state is remembered along with the rest of the view settings and is stored in a saved view.

There's an automatic counterpart next to it: the **Collapse empty columns** toggle in the [customize panel](/help/board-customize) folds every column without tasks by itself. Handy on boards with many columns where only two or three are busy at any moment.

## Creating and deleting

The **＋ New column** button to the right of the last column asks for a name and appends a column at the end. Inside a column there's its own **＋ New task** button: type a title and press Enter, and the task is created right in that column without opening the task window.

Deletion lives in the “…” menu, under **Delete the column**. It asks for confirmation, and for good reason: **a column is deleted together with all of its tasks**, and they do *not* go to the archive. If the tasks are worth keeping, move them to another column first.

## What else the header shows

Next to the name there's always a **task counter** for the column — it counts what's visible after the filters, not the column's whole contents. Under grouping by milestone it's joined by the **sum of estimates** of the column's tasks (`Σ 12h`), in the unit chosen for the project.
