---
title: Task relations
category: Tasks
order: 33
keywords: relations, blocks, blocked by, duplicates, relates, dependencies, gantt, arrows, unlink
updated: 2026-08-28
---

A relation is a statement about **how two tasks stand to each other**: one holds the other up, one repeats the other, or they are simply about the same thing. Relations live in the **Relations** tab of the task window, and blocking ones are additionally drawn as arrows on the [Gantt chart](/help/board-layouts).

![The Relations tab: relation kinds and the task picker](../assets/task-relations-light.png)

## The four kinds

| Kind | Reads as | When to use it |
|---|---|---|
| **relates to** | “these two are about the same thing” | Neither holds the other up, but working on one you should remember the other. |
| **blocks** | “until this one is done, that one can't start” | You are holding up somebody else's work. |
| **blocked by** | “waiting for that one” | Somebody else's work is holding you up. |
| **duplicates** | “the same thing as that one” | The task was filed twice. Usually the duplicate is then closed and the work goes on in the original. |

**“Blocks” and “blocked by” are one relation seen from two ends.** Which of the two you pick depends only on which task you are standing in. In the task that is waiting — “blocked by”; in the one being waited for — “blocks”.

## Creating one

At the bottom of the tab: the kind on the left, a search field on the right. You can **type a number** or **start typing a title** — the suggestion searches the whole workspace and groups what it finds by “project · board”, so tasks with the same name in different projects don't get confused.

You can link to a task on any board of the workspace, subtasks included: blocking dependencies between subtasks are an everyday thing.

A relation row behaves like a link: clicking it opens the related task. A completed task has its title struck through, so it's visible whether a block has been lifted. The cross on the right removes the relation.

The fastest way to create relations is [commands in a comment](/help/comments): `/blocks #2591`, `/blocked_by #2591`, `/relate #2591`, `/duplicates #2591`, `/unlink #2591`.

## A relation is one-directional — and it shows

An easy thing to trip on. A relation **is stored on the task it was created from**. If #10 says “blocks #20”, then:

- the **Relations** tab of task **#10** carries the row “blocks #20”;
- the **Relations** tab of task **#20** gets **no mirror row** — what it gets is an entry in the [history](/help/task-history), “added a relation to #10”.

So don't be surprised by an empty Relations tab on a blocked task. If a relation should be visible from both ends, create it from both ends; a repeated relation between the same tasks does not produce a duplicate.

**On the [Gantt chart](/help/board-layouts) this limitation doesn't apply**: the chart collects the blocking relations of the whole board and draws the arrow no matter which side owns the row.

## Relations from an integration

A relation created by an integration (GitLab, for instance) is marked with a grey badge naming its source. Your own relations carry no badge — a “Tessera” label on every row would be noise.

Such a relation can be deleted, but the confirmation warns you honestly: **“This relation will come back on the next sync.”** As long as it exists in GitLab, synchronisation will restore it. It has to be removed at the source.

## Blocking relations on the Gantt chart

The Gantt chart is the one place where dependencies are visible as a whole.

**Arrows.** Every blocking relation is drawn as a curve **from the end of the blocking task to the start of the blocked one** (finish-to-start). Arrows are built from the whole board's relations at once, and a pair linked from both ends produces one arrow, not two.

**Creating by dragging.** Each bar has an anchor point at its right edge. Drag from it to another bar — a live curve follows the cursor, and releasing creates the relation “this task **blocks** that one”. No duplicates are made: if the pair is already linked (either way), the drag does nothing.

**Deleting.** Hover an arrow and a cross appears at its midpoint.

**The “Auto” order.** The **Auto** button on the Gantt chart arranges rows **by the dependency graph**: a blocking task comes before the one it blocks, and a chain `A → B → C` reads top to bottom even if the original list had them scattered. Tasks caught in a cycle are not lost — they are appended at the end in their ordinary order.

“Auto” is a mode of its own, not one more sort level: pressing the button **clears the grouping, the sorting and the filters**. And the other way around — add any grouping or sorting and the mode quietly switches off (remove them and it comes back). Two orders cannot be in force at once, and Tessera doesn't pretend otherwise.

Two things the Gantt chart does not do, and should not:

- **Arrows don't move dates.** Tessera does not reschedule for you: a relation shows the dependency, and the decision to move something stays yours.
- **Arrows aren't drawn to subtasks.** Subtasks are shown as thin sub-bars inside their parent's row, and an arrow always attaches to the parent bar. The relation between subtasks can still be created — it just isn't drawn as a curve.

Tasks without a single date don't make it into the chart — they are collected in the “unscheduled” list below it, and their relations are still visible in the task window.
