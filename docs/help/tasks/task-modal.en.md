---
title: The task window
category: Tasks
order: 30
keywords: task, window, modal, estimate, milestone, priority, due, recurrence, assignees, status, parent, tabs, link, side panel
updated: 2026-10-10
---

A card on the board shows a task in two lines. Everything else — the estimate, the milestone, the author, the relations, the files, the history — lives in the **task window**. It opens with a click on the card and closes with `Esc` or a click outside it.

![The task window: fields on the left, tabs on the right](../assets/task-modal-light.png)

The window is split in half: **fields on the left**, **tabs** with the task's content on the right. The divider between the halves can be dragged, and a double click on it hides the right half entirely (and brings it back).

## Three ways to open a task

The **How to open a task** button in the window header switches the presentation:

- **Modal window** — the familiar centred dialog. The default.
- **Full screen** — the window fills the screen, for tasks with a long description.
- **Side panel** — the task moves into a panel on the right and the board stays live on the left: you can see which column the task is in and what sits next to it.

The choice is remembered **on this device**, not in your profile: a 27" monitor suits the side panel, a laptop the modal, and forcing one on the other would be uncomfortable. On a narrow screen there is no choice — a task always opens as a full-screen sheet.

## The window header

- The **breadcrumbs** “project · board” at the top aren't just a caption. Clicking them opens the list of boards: that's how a task is **moved to another board**, with all of its history.
- The **arrow button** copies a link to the task, shaped `/board/<board>?task=<number>`. Send it to a colleague and it opens the board with that task already unfolded.
- The **title** is edited right in the header — it's an ordinary input, not a separate form.
- If the task is linked to GitLab, an **Open issue in GitLab** link appears next to it.

## The fields on the left

| Field | What it does |
|---|---|
| **Status** · <kbd>S</kbd> | The board column: digits <kbd>1</kbd>–<kbd>9</kbd> pick a column, <kbd>]</kbd> moves to the next one, <kbd>E</kbd> completes. |
| **Priority** · <kbd>P</kbd> | Five levels, digits <kbd>4</kbd>…<kbd>0</kbd> — the same as <kbd>P</kbd> + digit. |
| **Assignees** · <kbd>A</kbd> | Any number of people, including members from GitLab. Search, “Assign to me”, <kbd>Space</kbd> toggles, <kbd>⇧</kbd> <kbd>⌫</kbd> unassigns everyone. |
| **Author** | Who created the task. Read-only. |
| **Due** · <kbd>D</kbd> | Start date, due date, time, plus **recurrence** and **notifications**. |
| **Estimate** · <kbd>O</kbd> | How much work the task is, in the project's units. |
| **Milestone** · <kbd>M</kbd> | The milestone the task belongs to; <kbd>⌘</kbd> <kbd>↵</kbd> creates a new one — with its dates. |
| **Tags** · <kbd>L</kbd> | The same tags the board can lay itself out by. |
| **Parent** · <kbd>U</kbd> | Make the task a subtask of another one (search across the board and the project), or detach it — <kbd>⌫</kbd>. |

Every field opens a **picker** at its row: a search or a header with the field's key, the list with the current value ticked, a key hint at the bottom. A single choice applies on <kbd>↵</kbd> and closes the picker; a multiple one (assignees) stays open. <kbd>Esc</kbd> closes only the picker and puts the focus back on the row; pressing the field's key again with the picker open moves to the next field (<kbd>S</kbd> <kbd>A</kbd> <kbd>P</kbd> <kbd>D</kbd> <kbd>L</kbd>). The same picker opens from a card's pills, the cells of the List view and the fields of the <kbd>⌘</kbd> <kbd>K</kbd> palette; on a phone it slides up as a bottom sheet.

### The estimate

The estimate is entered in the units configured for the project: **time** (`3d 4h`, `90m`, `1w`), **story points** or **custom units**. Under the field you see right away how the input was read (“= 1 day 2 hours”), and anything unreadable is pointed out right there, under the field; <kbd>Tab</kbd> goes to the quick-value chips, <kbd>⌫</kbd> in an empty field clears the estimate. If the project works on a scale (Fibonacci, T-shirts, linear), the picker offers a ready list of values with digits instead of a text field. The “Project scale…” row opens the Project window on its Estimate tab.

Next to its own estimate a task shows **`Σ` — the sum of its subtasks' estimates**. These are two different numbers and they are deliberately not added together: `Σ` answers “how much will the children take”, the own estimate “how much the task itself will”.

On a board card the estimate and the milestone are shown as pills (unless you have hidden them in [Customising the board](/help/board-customize)); a click on a pill opens the same picker as here.

### Due date, recurrence and notifications

The **Due** row (key **D**) opens the due popover to the left of the properties panel:

1. **The “When?” field** reads words: “fri 18:00”, “tomorrow”, “in 3 days”, “14.10”, “next monday 9:00”, “end of week”, “no date”. Under the field you see how it was read (→ Fri 9 Oct, 18:00 · in 6 days); words it doesn’t understand are named and nothing changes. **↵** applies.
2. **Start and due** — two dates; **Tab** switches which one you are setting. The start date is what [the timeline and the Gantt chart](/help/board-layouts) need: without it a task is drawn as a point rather than a bar. The calendar shades the range between the two.
3. **Presets** — Today, Tomorrow, End of week, Next Monday, In a week, No due date. **↓** from the field moves into the list, where the digits **1–5** and **0** work; **→** moves into the calendar.
4. **The calendar** — arrows move by day, **PgUp / PgDn** by month, **T** is today, **↵** picks. Below it is the time: a field, the 09:00, 12:00, 18:00 chips and “No time”. A day without a time is stored as a date and reads the same in every time zone; a time is in your profile’s time zone.
5. **“Repeat”** — a door with the rule in plain words (“Every week on Mon and Thu · next — Mon 12 Oct”). Inside: the frequency (daily, weekly, monthly, yearly, or **on dates you tick in the calendar**), “every N”, the weekdays, when to create the next one (on completion, on entering a column, on schedule), where it goes, and three switches — **create a duplicate**, **repeat forever**, **skip weekends**. **Esc** goes back, **⇧⌫** removes the repeat. The repeat counts from the due date: with no due date no next task appears. The rule is also shown in the tooltip of the due pill on a card.
6. **“Due reminder”** — default, on or off; how early to remind (at the due time, 15 minutes, an hour, 3 hours, a day before) and whether to repeat. “Default” takes the setting from [Notifications](/help/notifications); pressing the chosen value again returns it to the default.

Everything is saved at once, so **↵ “done”** only closes the popover, **⌫** on an empty field clears the date, **Esc** closes. The same popover opens from the due pill on a board card. In the ⌘K palette the Due field takes words and presets, and **Tab** opens the calendar right under the field. On a phone the due date is a full-height sheet; the calendar, the repeat and the reminder are its next pages.

## The tabs on the right

Every tab label carries a counter, so a closed tab still tells you whether there is anything inside.

- **Description** — the [Markdown editor](/help/markdown-editor) with a toolbar, attachments and diagrams.
- **[Comments](/help/comments)** — the discussion, with reply threads, mentions and commands.
- **Subtasks** — this task's children; the same ones the card shows when [subtasks are expanded](/help/board-composer).
- **[Relations](/help/task-links)** — blocks, duplicates and plain “relates to”.
- **Files** — the task's attachments. Files dropped into the description land here too.
- **[Documents](/help/task-documents)** — documents that point at this task.
- **[History](/help/task-history)** — the journal of changes.

## The bottom of the window

At the bottom: **Archive** and **Delete**. The difference matters: [the archive](/help/archive) keeps a finished task for as long as you like, while deleting sends it to the [board's trash](/help/trash) for 30 days — right after the delete, Undo in the message at the bottom brings it back.

If the task has subtasks, archiving asks what to do with them: archive them along with the parent, or detach them and leave them on the board.

An archived task opens **read-only** — the window says so outright, and instead of the action buttons there is “Restore from the archive”.
