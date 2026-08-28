---
title: Task relations
category: Tasks
order: 33
keywords: relations, blocks, blocked by, duplicates, relates, dependencies, gantt, arrows, app
updated: 2026-08-28
---

The **Relations** tab is the fourth in a task's tab row. The relation types, what they mean and the rules around them are the same as in the browser; what follows is what works differently on a phone.

## Four types

| Type | Reads as |
|---|---|
| **relates to** | «these two are about the same thing» |
| **blocks** | «that one can't start until this one is done» |
| **blocked by** | «waiting for that one» |
| **duplicates** | «the same thing as that one» |

«Blocks» and «blocked by» are **one relation seen from two sides**. Which one you pick depends only on which task you are setting it from.

## A relation row

On the left, the type label in a **fixed-width column**, so task numbers line up under each other and the list reads as a column. Then the source badge (on relations from an integration), the `#N` number in the accent gradient, the title, and a cross on the right.

A completed task has its title **struck through** — you can see whether a block has been lifted. Tapping the row opens the related task on top of the current one.

## Adding one

Under the list, a row of two controls: a **type dropdown** on the left and a **search field** on the right, hinting «Find a task: #number or title».

Search covers **the number and the title at once**: `2727` finds task #2727 as well as any task with those digits inside its number. Matches are listed, and each row has a second line under the title reading **«project / board»**. The browser does the same thing with grouping; on a phone the label goes on every row, because a group header would eat the little width there is.

**Tapping a row creates the relation right away** — there is no separate Add button, and the search field clears. The type defaults to «relates to», so if you want a block, change the type first and search after.

The suggestion list is **capped at thirty rows**. If the task you want is not there, narrow the query — by number is the most reliable.

The fastest way to set relations is [commands in a comment](/help/comments): `/blocks #2591`, `/blocked_by #2591`, `/relate #2591`, `/duplicates #2591`, `/unlink #2591`.

## Removing goes through a confirmation

The cross **does not delete immediately**: a small «Remove the relation?» popover opens next to it — as in the browser.

A relation from an integration gets different, honest text: **«This relation will come back on the next GitLab sync. Delete it?»**. As long as the relation exists in the source, syncing will restore it — remove it there.

## A relation is one-directional, and it shows

The same limitation as in the browser, and on a phone people trip over it more often, because an empty tab is easy to read as «no relations».

A relation **is stored on the task it was set from**. Set «blocks #20» from #10 and the **#10** Relations tab has the row, while the **#20** tab does **not** — there you get an «added a relation» entry in the [history](/help/task-history) instead. If it has to be visible from both sides, set it from both; a repeated relation between the same tasks does not create a duplicate.

## On the Gantt chart

This is where the limitation does not apply. **The [Gantt chart](/help/board-layouts) collects blocks from the whole board** and draws an arrow from the blocker's finish to the blocked task's start, whichever side the relation was set from. The counters above the chart show how many there are.

Two things about the phone:

- **You cannot drag a relation from bar to bar** — that gesture is not implemented in the app. Set relations here on the tab, or with a command in a comment.
- **You cannot remove an arrow on the chart either** — there is no cross on the arc; you have to go into the task.

The **Auto** button on the Gantt chart lays rows out along the dependency graph — a blocker always above the task it blocks.

Arrows, as in the browser, **do not move dates** and **are not drawn to subtasks**: an arc always attaches to the parent's bar.

## An empty tab

With no relations, the list is replaced by «No relations yet», with the usual «type + search» row below it.
