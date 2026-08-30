---
title: The board bar: grouping, sorting, filters
category: Boards
order: 21
keywords: bar, composer, grouping, sorting, order, filter, chips, search, subtasks, milestone, backlog, reset
updated: 2026-08-28
---

The board bar is the narrow strip under the header that collects everything which changes how tasks are shown without changing the tasks themselves: grouping, sorting, filters and search. Every condition is a **chip**; read left to right, the chips fully describe what you are looking at.

![The board bar: grouping, sorting and filter chips](../assets/board-composer-light.png)

The order of the elements is always the same:

1. **The scope chip** — the archive or a milestone, if you entered one (see below).
2. **The branch icon** — expanding subtasks.
3. **The grouping chip** — what the columns are made of.
4. **Sort chips** — one per level.
5. **Filter chips** — one per condition.
6. **The `+` button** — add a grouping, a sort or a filter.
7. **Search by title** and, when there is something to reset, the **Clear all** cross.

Once there are many chips, the bar takes the whole row and the buttons on the right (customize, saved views) slide away for a while. That isn't a glitch — they come back as soon as you drop the extra conditions.

## Grouping

Grouping decides what a column means on this board. The `+` menu → **Grouping** offers:

- **By status** — stage columns, the ordinary Kanban. The default.
- **By tags (all)** — a column per project tag. This is Tessera's signature capability: the very same set of tasks is seen either as a flow through stages or as a layout by direction, team or client, without rebuilding the board.
- **By tags · “Prefix”** — only the tags of one prefix. If your tags read `Difficulty::high`, `Difficulty::low`, grouping by the “Difficulty” prefix gives you a board of exactly those columns, and the other tags stay out of the layout.
- **By milestone** — a column per project milestone plus a “No milestone” column. The header of such a column also shows the sum of its tasks' estimates (`Σ`).

On the Timeline and the Gantt the list also carries **By assignee** and **No grouping** — there the columns become swimlanes, and both layouts make sense.

The grouping chip isn't only an indicator: **clicking it flips status ⇄ tags**. That's the fastest way to compare two readings of one board.

A task with two tags lands in two columns at once. It isn't a duplicate but one and the same card shown in two directions: change it in one column and it changes in the other. Tasks without tags gather in a **No tags** column.

## Sorting — both direction and order

Sorting in Tessera is **multi-level**. The `+` menu → **Sorting** adds a level; there are six fields: **Priority**, **Due date**, **Milestone**, **Title**, **Number** and — on the Timeline and the Gantt only — **Status**.

There are two things you can do to a level, and they are easy to confuse:

- **Clicking a chip changes the direction** — the `↑` arrow (ascending) flips to `↓` (descending).
- **Dragging a chip changes the order of the levels.** The leftmost chip is the primary sort, the next one breaks ties in the first, and so on.

The hint on the chip itself says exactly that: “Click — direction · drag to reorder”. The cross on a chip removes the level.

An example: the chips “Priority ↓” and “Due ↑” in that order mean “the most important first, and within one priority the most urgent”. Swap them by dragging and you get a fundamentally different list: nearest due dates first, with priority only breaking ties within a day.

Until a single sort level is added, the cards sit in **manual order** — the one you dragged them into. Once a level appears, the manual order hides behind it (it isn't lost: remove the sort chip and the order returns). Subtasks expanded on cards follow the same rule their parents do.

Due dates are handled apart from the direction: **tasks without a due date always come last**, whether the sort is `↑` or `↓`. Otherwise a descending sort would float everything undated to the top.

## Filters

The `+` menu has a section per filter: **Priority**, **Assignee**, **Author**, **Tag**, **Milestone**, **Due date**, and — on the Timeline and the Gantt — **Status** as well (there are no status columns there, and nothing else would hide completed tasks).

Several values of one filter combine with “or”: two chosen assignees show the tasks of both. Different filters combine with “and”: assignee **and** tag.

Worth calling out:

- **Due date** is not a date but a state: “Overdue”, “Today”, “This coming week”, “With a due date”, “No due date”.
- **Milestone** has an explicit “No milestone” entry besides the milestones themselves.
- **Tag** — the menu is grouped by prefix so a long tag list stays readable.
- **Assignee** and **Author** — on boards linked to GitLab, a separate “GitLab” heading lists people from GitLab who have no Tessera account. The author filter additionally picks up authors met right in the board's tasks: an issue can be opened by someone outside the project's member roster.

Every chosen filter becomes a chip, and the cross on the chip removes that one. The cross at the very end of the bar (**Clear all**) drops every sort and filter at once, but **leaves the grouping alone** — it stays as chosen.

## Search by title

The field on the right of the bar searches task titles and works *together* with the filters, not instead of them. The search narrows an already filtered set, so an empty result while chips are active more often means “nothing matched all conditions at once” than “no such task”. When in doubt, clear the filters with the cross and search again.

## Expanding subtasks

The branch icon on the left of the bar expands subtasks right inside the cards: every card lists its children without opening the task window. The tooltip on the icon states the current state — “Expand subtasks” or “Subtasks expanded”.

Expansion respects the filters: if a filter hid some of the children, the card honestly reports how many are hidden.

## Scope: milestone and archive

Besides filters a board has a **scope** — what it loaded from the server at all. The scope shows as a chip at the very start of the bar and is removed by the cross on it.

- **Milestone.** The project tree on the left can show a board's milestones; clicking one opens the board narrowed to a single milestone. An accent chip with the milestone name appears in the bar. A separate **Backlog** entry shows the tasks attached to no milestone. This is a scope, not a filter: a large project never loads all its cards at once.
- **Archive.** The “Archive” button in the board header opens its archive — see [The board archive](/help/archive).

A scope and a milestone *filter* are different things, and they don't stack. If, while scoped to one milestone, you add a **filter** by milestone, the scope drops itself: filtering across several milestones needs the whole board loaded.

## What the bar remembers

The state of the bar is kept **separately for each visualization** of one board. A status filter set on the Gantt never leaks into the Kanban — that filter isn't even offered there. Switch to the list and back, and each visualization returns to its own conditions.

The current (unnamed) state of the bar is remembered **on this device**. To carry a set of conditions between devices and open it in one click, save it as a view — see [Saved views](/help/board-saved-views).
