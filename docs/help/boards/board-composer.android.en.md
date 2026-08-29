---
title: "Board bar: grouping, sorting, filters"
category: Boards
order: 21
keywords: bar, composer, grouping, sorting, order, filter, chips, search, subtasks, milestone, backlog, reset, app
updated: 2026-08-28
---

The board bar is the row under the header that holds everything which changes how tasks look without changing the tasks themselves: grouping, sorting, filters and search. Every condition is shown as a **chip**; the chips read left to right and together they fully describe what you are looking at.

![The board bar expanded](../assets/board-composer-mobile-light.png)

## Collapsed and expanded are two different modes

On a phone the bar is almost always longer than one row, so it has two states — and that is where the main difference from the desktop bar lies.

**Collapsed**, the bar is exactly one row; the rest of the chips are clipped and the content is dimmed. In this state **any tap on the bar only expands it**: tapping a chip, the `+` button or the reset cross does the same thing as tapping empty space. That is deliberate — hunting for a free spot in a chip-packed row was fiddly.

**Expanded**, the bar shows every chip, wraps them onto several rows, and only then do the chips respond to taps themselves. At the same time the buttons on the right — saved views and board view — slide away: there is no room for both chips and buttons at phone width. They come back once the bar collapses.

To collapse it, tap **outside the bar**, on the board. There is no separate “collapse” button.

Hence the practical rule: **on a phone almost every action in the bar takes two taps.** The first expands it, the second does what you meant.

## Grouping

Grouping decides what a column means on this board. The grouping chip sits right after the subtasks icon and is always visible — even when nothing is filtered.

Tapping the chip opens a menu; a tick marks the current option:

- **By status** — stage columns, the ordinary kanban. The default.
- **By tags (all)** — a column per project tag. This is Tessera's signature feature: the same set of tasks read either as a flow through stages or as a split by direction, team or client.
- **By tags · “Prefix”** — only the tags of one prefix. If your tags are named `Effort::high`, `Effort::low`, grouping by the “Effort” prefix gives a board of exactly those columns. One entry appears per prefix found in the board's tags.
- **By milestone** — a column per project milestone plus “No milestone”. The entry only appears if the project has milestones.

On the timeline and the Gantt chart, **“By assignee”** and **“No grouping”** join the list — there columns become lanes and both layouts make sense.

> On a computer the grouping chip flips status ⇄ tags in a single click. In the app it always opens the menu, so you can reach a prefix or milestone layout without stepping through the intermediate ones.

A task with two tags lands in two columns at once. That is not a duplicate but one and the same card shown in two directions: change it in one column and it changes in the other. Tasks without tags gather in the “No tags” column.

## Sorting — direction and order

Sorting is multi-level. The `+` button → **“Sorting”** adds a level; there are six fields: **Priority**, **Due date**, **Milestone**, **Title**, **Number** and — only on the timeline and Gantt — **Status**. A field that is already added is not offered again.

There are three things you can do to a level:

- **Tapping the chip flips the direction** — the `↑` arrow (ascending) switches to `↓`.
- **A long press and drag reorders the levels.** The leftmost chip is the primary sort, the next one breaks ties in the first, and so on.
- **The cross on the chip** removes the level.

Dragging works differently from the desktop, which is worth knowing in advance. The long press answers with a vibration, the chip lifts, and from then on it travels **over the others and they do not step aside**. The place it will land is shown by a ring around the target chip. The move happens once — the moment you lift your finger.

Example: the chips “Priority ↓” and “Due ↑” in that order mean “the most important first, and within one priority the most urgent”. Swap them and you get a fundamentally different list: nearest due dates first, with priority only breaking ties within a day.

Until a level is added, cards lie in **manual order** — the one you dragged them into. Once a level appears, the manual order hides behind it, but is not lost: remove the sort chip and the order comes back.

Tasks **without a due date are always last**, whichever way the due-date sort points. Otherwise a descending sort would float everything undated to the top.

## Filters

The `+` button — a dashed square with a plus — opens a two-level menu: first the section, then the value. Inside a section the first row is **“Back”**, which returns to the list of sections without closing the menu.

The sections are **Priority**, **Assignee**, **Author**, **Tag**, **Milestone**, **Due date**, and on the timeline and Gantt also **Status** (there are no status columns there, and nothing else to hide finished tasks with). A section is not shown when there is nothing to filter by: no members — no “Assignee”, no tags — no “Tag”.

Several values of one filter combine with “or”: two chosen assignees show the tasks of both. Different filters combine with “and”: assignee **and** tag.

Worth calling out:

- **Due date** is not a date but a state: “Overdue”, “Today”, “This week”, “Has due date”, “No due date”.
- **Milestone** offers an explicit “No milestone” entry alongside the milestones themselves.
- **Tag** — the menu is grouped by prefix; the headers appear only when there is more than one group.
- **Assignee** and **Author** — on GitLab-linked boards, a “GitLab” header lists the people from GitLab who have no Tessera account. The author filter additionally offers the authors met in the board's own tasks: an issue can be opened by someone outside the member list.

Every chosen value becomes a chip, and the cross on the chip drops exactly that one.

## Reset

The cross at the right edge of the bar is **“Reset all”**. It drops sorting, filters, the search text and the milestone scope in one go, but **leaves grouping alone**: the board keeps the layout you chose.

The cross appears only when there is something to reset. In the archive it is absent — there the scope is dropped by its own chip.

## Search by title

The search field is at the end of the bar, and **you can only type in it while the bar is expanded**. While the bar is collapsed, its place is taken by a plain label: either your current query or “Search…”. It is not a broken field — expand the bar and it becomes one.

Search works together with the filters, not instead of them: it narrows the already filtered set. So an empty result with active chips more often means “nothing matched all the conditions at once” than “no such task”.

## Expanding subtasks

The branch icon — a square chip with no caption, on the left, right after the scope chip — expands subtasks inside the cards: every card lists its children without opening the task. While subtasks are expanded the icon is tinted with the accent colour; there is no caption like on the desktop, the state reads from the highlight.

Expansion respects the filters: if a filter hid some of the subtasks, the card honestly says how many children are hidden.

## Scope: milestone and archive

Besides filters a board has a **scope** — what it loaded from the server at all. The scope is shown by a chip at the very start of the bar and dropped by the cross on it.

- **Milestone.** The side menu can show a board's milestones underneath it; picking one opens the board narrowed to that milestone, and an accent chip with its name appears in the bar. A separate **“Backlog”** entry shows the tasks not attached to any milestone. This is a scope, not a filter: a large project does not load every card at once.
- **Archive.** Opened from the board menu; an amber “Archive” chip takes its place in the bar — see [Archive](/help/archive).

The two chips never show together: the archive is a read-only scope for the whole board, and narrowing by milestone does not apply inside it.

A scope and a milestone filter are different things and they do not stack. If you add a milestone **filter** while inside a milestone scope, the scope drops itself: filtering by several milestones requires the whole board to be loaded.

## What the bar remembers

The state of the bar is kept **separately for each view** of the same board. A status filter set on the Gantt chart will not leak onto the kanban — which does not even offer that filter.

The current (unnamed) state of the bar is remembered on this device. To have a set of conditions open with a single tap on other devices too, save it as a view — see [Saved views](/help/board-saved-views).
