---
title: Visualizations: list, calendar, timeline, Gantt, matrix
category: Boards
order: 25
keywords: visualizations, view, list, calendar, timeline, gantt, chart, matrix, eisenhower, dependencies, zoom
updated: 2026-10-08
---

The same board tasks can be looked at in six ways: Kanban, List, Timeline, Gantt, Calendar and Matrix. The switcher sits on the board bar — six equal segments, from the keyboard `V`, then `1`–`6`; when room is short the segments stay icons and only the chosen one keeps its caption. The chosen view is remembered for that board.

It's important to understand that these are **not different boards and not copies of the tasks**. It's one set of cards shown differently: move a task in the calendar and it moves on the Kanban too.

Each visualization has its own memory: grouping, sorts and filters are kept separately for each. A filter set on the Gantt won't turn up on the Kanban. A saved view remembers the view kind too: apply it and the board switches to where it was saved. See [Board views](/help/board-saved-views).

## Board

The Kanban — columns and cards, the default layout. It's covered by [The board: basics](/help/boards-and-tasks) and [Board columns](/help/board-columns).

## List

![The board as a list](../assets/board-list-light.png)

Tasks as rows, laid out in **groups** — the same ones the Kanban shows as columns: statuses, tags or milestones, whatever the grouping is. A group has a header with the status glyph, a counter (“4 (7)” — tasks, and with their subtasks) and the sum of estimates, and a backdrop tinted like the column under it. **Clicking the header folds the group**; the list keeps its own folded groups, the Kanban isn't affected.

A row carries the number, the title, the priority, the due date, the assignees, the estimate and the tags. An overdue date is red, completed tasks are struck through. The circle on the left completes the task (or reopens it). The field columns can be resized — drag the edge of a column caption; a double click on the edge brings the default width back. The widths are remembered with the rest of the board's settings and saved in a view.

Clicking a row opens the task; right-clicking gives the same context menu the card has. A double click on the title renames it right in the row. “＋ Task” in a group header creates a task in that group (`↵` keeps the field open for the next one).

**From the keyboard** the list is triaged without the mouse:

| Key | Action |
|---|---|
| `↑` `↓` | through the rows and group headers |
| `⇧↑` `⇧↓` | select several rows (with the mouse — ⇧-click or ⌘-click) |
| `←` `→` | fold / unfold the group |
| `↵` | open the task (on a header — fold the group) |
| `F2` | rename in place |
| `E`, `P` `0`–`4`, `⌘←` `⌘→` | complete, priority, into the next group — for every selected row at once |
| `P` `D` `A` `L` `S` | priority, due date, assignees, tags, status — as on the board |
| `⌘↑` `⌘↓` | move up / down in the column |
| `C` | a new task in the group |

`⌘↑` `⌘↓` change the order only when grouped by status and with no sort — the same order the Kanban cards have. The selection frame and the hint above the row show only while you work from the keyboard.

On a phone the rows are taller: the title, the number and due date under it, the assignee on the right; the field columns aren't shown.

The list draws only the rows in view, so a board of several hundred tasks scrolls smoothly too.

## Calendar

![The board as a calendar](../assets/board-calendar-light.png)

A task sits in the cell of **its due date** as a pill in the colour of its status; a red dot means the due date has passed. Today is marked with a gradient. The first day of the week follows the settings (Localization), month and weekday names come from the interface language.

Two modes, switched above the grid on the right:

- **Month** — the overview. A cell holds up to three tasks, the rest are “+N more”; clicking it opens the whole day as a list.
- **Week** — planning: seven full-height days, every task of the day, the timed ones on top, in order.

The chosen mode is remembered for the board and saved in a view.

**Move a due date by dragging.** Drag a task onto another day: the cell lights up and shows the new date, which is saved on release. A date-only due stays date-only, a timed one keeps its time, and the start date (if there is one) moves along with it. Tasks **without a due date** are behind the “N undated” link above the grid: open the list and drag a task onto a day — that gives it a due date.

The “＋” in a cell (shown on hover) creates a task due that day.

**From the keyboard:** `T` — today, arrows — through the days, `⇧←` `⇧→` — a month (in Week mode — a week) back and forth, `C` — a task on the picked day. `↵` steps into the tasks of the picked day: `↑` `↓` between them, `↵` opens, `⇧←` `⇧→` move the due date by a day, `E` `P` `D` `A` `L` `S` — as on the board, `Esc` — back to the day.

On a phone the month is shown as dots in the status colours; tapping a day opens its tasks as a list.

## Timeline

![The board as a timeline](../assets/board-timeline-light.png)

A horizontal time axis, with a task as a bar from its start date to its due date. The bars are laid out in **swimlanes**, and the lanes are the same grouping as on the board bar: by status, tag, milestone, assignee, or no grouping at all.

The controls above the chart:

- **Today** — bring the axis back to the current date.
- **The scale** — Day, Week, Month or Quarter; `−` and `+` (or the wheel with ⌘/Ctrl) zoom smoothly, and the scale moves on by itself.
- **The arrow** — collapse the left column of task names, giving the whole width to the chart.
- On the right — counters: how many tasks are overdue and how many have no dates.

A bar can be **dragged along the axis** — that changes the task's dates — and stretched by either edge (the left one sets the start, the right one the due date). Dashed verticals mark the **milestone due dates** of the project: you see at once which tasks won't make a milestone.

Tasks with no dates aren't thrown away: they gather in an “Unscheduled” block beside the chart. Give a task a due or start date and it appears on the axis. Expanded subtasks are drawn as thin bars inside their parent's row.

## Gantt

![The board as a Gantt chart](../assets/board-gantt-light.png)

The Gantt is the timeline plus **dependencies between tasks**. The axis, the zoom, dragging bars and the milestone markers all work the same; blocking arrows are added.

To **create a dependency**, drag from the right edge of one bar to another — the hint above the chart says as much. A “blocks → blocked” arrow appears. The counters on the right gain a number of links.

Every task on the Gantt occupies **its own row** — otherwise an arrow would have nowhere to land. That's why the Gantt looks longer than the timeline on the same data.

The **Auto** button on the board bar (Gantt only) lays the rows out **by the dependency graph**: a blocking task always sits above the one it blocks. Bear in mind this is only meaningful with grouping and sorting empty, so turning “Auto” on clears them. Turn it off and you're back to the ordinary layout, but the conditions it cleared have to be set again.

The link types themselves are covered in the article on task links.

## The Eisenhower matrix

![The board as an Eisenhower matrix](../assets/board-matrix-light.png)

A 2×2 square: the columns are “Urgent” and “Not urgent”, the rows “Important” and “Not important”. Hence four quadrants:

| Quadrant | What it means |
|---|---|
| Do | urgent and important |
| Schedule | important, not urgent |
| Delegate | urgent, not important |
| Later | neither urgent nor important |

**Tasks sort themselves.** A task with high or urgent priority counts as important; one due within the next 3 days or already overdue counts as urgent. Both thresholds are set in the Board view panel, the Matrix section, and the axis captions name them (“Urgent · due ≤ 3 days”). The matrix needs no extra fields: it reads the priority and due date the tasks already have.

The automation can be overruled: **drag a card into another quadrant** and it stays there. Such a card gets a “by hand” mark; clicking it puts the task back in its automatic place.

Cards in the matrix are compact by default, two to a row; the size is changed in the Board view panel, the Cards section. The **“＋”** button in a quadrant header (on hover) creates a task: it goes into the board's first column and is pinned to that quadrant straight away.

**From the keyboard:** the arrows walk the cards and cross into the next quadrant, `↵` opens, `E` `P` `D` `A` `L` `S` — as on the board, `⌘` + an arrow pins the card into the quadrant that way, `⌫` gives a pinned card back to its automatic place, `C` — a new task in the quadrant.

**Completed tasks don't go into the matrix** — it's a tool for triaging what's ahead, not a report on what's done.
