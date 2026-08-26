---
title: Boards and tasks
category: Working with tasks
order: 20
keywords: board, kanban, columns, tags, grouping, filter, views, task, subtasks, priority, due date, app
updated: 2026-08-24
---

The board is the main working screen. Columns set the stages, cards are the tasks. On a phone the columns swipe sideways: one takes up almost the full width, and its neighbours peek in from the edge.

![A board with tasks laid out across columns](../assets/board-mobile-light.png)

## How to move a task

There are two ways, and on a phone they aren't equal:

- **Press and hold a card** and drag it into a neighbouring column. While the card is “in your hand”, the board scrolls towards the edges on its own. Handy for a short move to the next column.
- **Open the task and change its column** with the status chip. Three columns away from the current one, this is faster than dragging a card across half the board.

On the task screen, next to the status, there is a “›” button — it shifts the task one column to the right — and a “✓” button, which sends it straight to the completing column.

## Grouping by tags

A board's columns don't have to be stages. On the bar above the board there is a **grouping chip** (the stack icon): a tap opens a list — “By status”, “By tag”, “By milestone”. Pick tags and a column becomes a tag, not a stage. The very same set of tasks can be viewed either as a flow through stages or as a layout by direction — without rebuilding the board.

If the bar is collapsed to a single line, tap it — it expands and shows the sorting and filter chips. The chosen conditions hang as chips, and an unwanted one is removed with its cross.

![The same board grouped by tags](../assets/board-tags-mobile-light.png)

A single task with two tags lands in two columns at once — this isn't a duplicate but one and the same card shown in both directions. Tasks with no tags gather in an “Ungrouped” column at the end.

## Views

Besides Kanban, the board can show the same tasks differently — the view switcher on the board bar:

- **List** — a flat table, handy for bulk triage.
- **Calendar** — by due dates.
- **Timeline** and **Gantt** — duration and dependencies. Both pan and pinch-zoom; while you're on them, a swipe from the left edge doesn't open the menu — otherwise it would clash with the horizontal scroll.
- **Matrix** — a layout by importance and urgency.

The view settings are shared with the web version: change the grouping from your phone and the board opens the same way on your computer.

## Task screen

![The task screen: properties on top, tabs below](../assets/task-modal-mobile-light.png)

A tap on a card opens the task full-screen. On top are the title and properties (priority, due date, assignees, tags, milestone, status), below them the description in Markdown, and under that the tabs: **Comments**, **Subtasks**, **Relations**, **Files**, **History**.

Properties are applied the moment you change them. The title and description are saved with the **“Save”** button — going back without it won't record your edits.

Subtasks form a tree of any depth; on the board they can be expanded right inside the cards.

## When a task counts as done

A task is complete once it lands in the board's **completing column**. That's also when the completion date is stamped and an event is written to the log; move it back out and the mark is removed. If a task isn't counted as done even though it visually sits “at the end”, check which column is set as completing: by default it's the rightmost. Assigning a different one is done from the web version.

## Empty columns

To avoid swiping the board through emptiness, turn on **collapse empty columns** in the board settings — they fold into narrow strips. On a phone this is more noticeable than on a wide screen: every empty column is one extra swipe.
