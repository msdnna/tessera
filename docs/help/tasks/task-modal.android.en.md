---
title: The task window
category: Tasks
order: 30
keywords: task, window, modal, estimate, milestone, priority, due, recurrence, assignees, status, parent, tabs, save, cancel, back, app
updated: 2026-08-28
---

A card on the board shows a task in two lines. Everything else — estimate, milestone, author, relations, files, history — lives in the **task window**, which opens on a tap.

In the browser the window is split in half: fields on the left, tabs on the right. There is no such split on a phone — the window is **one column you scroll top to bottom**: header, title, fields, tabs. So reaching history or relations means scrolling, not glancing to the right.

![The task window on a phone](../assets/task-modal-mobile-light.png)

## How it opens

There is no «modal / full screen / side panel» choice in the app: the window always takes up almost the whole screen (leaving a thin margin) and is always the same. The "How to open a task" setting is about monitors, and a phone has nothing to choose from.

The keyboard does **not** cover the window: while it is up, the window shrinks to the space above it. That matters in comments — the input and the command suggestions stay visible.

## Back works in two steps

The system Back button on any tab other than the first **returns to «Description»** instead of closing the task. A second press then closes the window. That way you don't have to drop out to the board to get from «History» back to the description.

## The header

At the top are the **breadcrumbs** «project / board» and the task number (`#123`) on the right. As in the browser, the breadcrumbs are tappable: a tap opens the list of boards, and the task **moves to another board** with all of its history.

There is **no copy-link button** in the app — a link of the form `/board/<board>?task=<number>` is assembled in the web version.

The **title** is edited right here: an ordinary field, captioned «Task title» when empty.

## What saves immediately and what saves on a button

This is the main difference from the browser, and it's worth remembering.

At the bottom of the window sit **«Cancel»** and **«Save»**. But that button saves only the **title and the description**. Every other field — priority, due date, estimate, assignees, tags, milestone, status, parent — **applies the moment you change it** and has already gone to the server.

Hence: **«Cancel» does not roll back** a removed assignee or a changed priority. It only discards edits to the title and the description. If a field was changed by mistake, set the old value back the same way you changed it.

## The fields

| Field | What it does |
|---|---|
| **Priority** | Five levels; the coloured priority dot shows on the card too. |
| **Due** | Opens the calendar: start, due, time, recurrence and notifications. |
| **Estimate** | Effort in the project's units, with `Σ` over subtasks next to it. |
| **Author** | Who created the task (for GitLab tasks, the issue's author). Read-only. |
| **Assignees** | Any number of people, GitLab members included. |
| **Tags** | The same tags a board can lay itself out by. |
| **Milestone** | The milestone link; a new one can be created here. |
| **Status** | The board column, a "next column" arrow and a "done" circle. |
| **Parent** | «Make a subtask…» or «Detach». |

### Status: three controls in one row

The «Status» row is a chip with the current column (tap it for the column list), a **«›» arrow** and a **circle**:

- the arrow shifts the task **one column to the right** — the most common move, and not one you want to open a list for;
- the circle marks the task done. If the board has a done column, the task **moves into it** (so the server records it in history); if there is no such column, it is simply flagged as done.

While a move is in flight, both the arrow and the circle are disabled.

### Estimate and `Σ`

The estimate is given in the project's units: **time** (`3d 4h`, `90m`), **story points** or custom units; if the project works on a scale, you get a ready list of values instead of a field. An empty one reads «Not set».

Next to your own estimate sits **`Σ` — the sum of the subtasks' estimates**. They are different numbers and deliberately don't add up: `Σ` answers "how much will the children take", the estimate itself "how much for this task".

The estimate and the milestone **cannot be edited from the card** — only from here. That is the main reason to open the window on a task that already looks filled in on the board.

### Due date, recurrence and notifications

The due row shows «start → due» when a start date is set, and a **recurrence glyph** next to it when the task repeats. A tap opens the calendar with its three parts: the dates with times, the recurrence (including hand-picked dates and a trigger on moving to a column) and the notifications. Details are in [Reminders](/help/reminders) and [Notifications](/help/notifications).

The start date is what [the timeline and the Gantt chart](/help/board-layouts) need: without it a task is drawn as a point rather than a bar.

### Tags and milestones: created from the keyboard

At the bottom of the tag picker there is a «New tag, Enter» field; the milestone picker has «New milestone, Enter». There is **no button** beside them: both are created with the confirm key on the on-screen keyboard. A name typed but not confirmed is lost. More about tags in [Tags, prefixes and how to manage them](/help/tags).

## The tabs

There are **six** tabs: «Description», «Comments», «Subtasks», «Relations», «Files», «History». Their labels carry counters, so a closed tab still tells you whether there is anything in it.

**There is no «Documents» tab in the app.** The list of documents referencing a task is available in the browser only — see [Task documents](/help/task-documents).

- **Description** — the [editor](/help/markdown-editor) with its toolbar and attachments.
- **[Comments](/help/comments)** — reply threads, mentions and commands.
- **Subtasks** — this task's children; a new one comes from the «+ subtask (Enter)» field — again, from the keyboard.
- **[Relations](/help/task-links)** — blocks, duplicates and "relates to"; tasks are found with the «Find a task: #number or title» field.
- **Files** — attachments; «Attach a file» opens the system picker, and tapping a file offers «Open "name"» in a suitable app.

## GitLab

The **«GitLab»** row in the fields shows one of two things. If the task is already linked to an issue — a number like `!42` with a link glyph; tapping it opens the issue **in the phone's browser**. If there is no issue yet and the board's binding allows creating one — a **«Create an issue»** button with a **«Template»** picker next to it (an empty list honestly reads «No templates»).

The chosen template is filled into the **task's description**, and that description becomes the issue's body. In the browser the template picker lives above the description editor; here it sits in the field rows — on a phone the description is a tab of its own, and a button on it would be invisible whenever any other tab is open.

For a GitLab task the «Author» row shows the issue's author with an `@login · GitLab` caption, not whoever created the card in Tessera.

## The bottom of the window

To the left of «Cancel» and «Save» are two icons: **archive** and **bin**. Both ask for confirmation on the spot ("Archive this task?" / "Delete this task? This cannot be undone."). The difference is fundamental: [the archive](/help/archive) keeps the task whole and lets you bring it back, deletion is irreversible.

An archived task is quicker to restore from the card itself — its «⋮» menu has «Restore from archive» and «Delete forever».
