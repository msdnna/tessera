---
title: The board archive
category: Boards
order: 26
keywords: archive, archived, restore, read-only, delete, subtasks, app
updated: 2026-08-28
---

The archive is where tasks go that are no longer wanted on the board but would be a shame to lose. It isn't a bin with delayed deletion: nothing disappears from the archive on its own.

You open it from the board menu — the three dots in the top right corner, the **Archive** entry. There is no separate button in the header as there is on a computer: tags and milestones live under the same three dots.

## The archive is the same board, read-only

Opening the archive doesn't take you to a separate screen. It is a **scope** of the same board: the same columns, the same grouping, the same filters — only archived tasks are shown instead of live ones. An amber **“Archive (read-only)”** chip appears in the [board bar](/help/board-composer); the cross on it returns you to the ordinary board.

![The board archive on a phone](../assets/board-archive-mobile-light.png)

The chip comes first in the bar, and while the bar is collapsed it pushes the search field past the single visible row — nothing is lost, tap the bar and it unfolds with search and filters intact.

The main rule follows from that: **nothing in the archive can be changed**. Cards don't drag, the “complete” and “add subtask” buttons are taken off them, and the priority, due-date and tag pills stop being buttons: tapping any of them simply opens the task for reading. You can't create a task in the archive either.

Search the archive with the same tools as the board: filter by tag, by assignee, search by title — all of it works here too.

Entering the archive drops the milestone scope if you had one. And the other way round: picking a milestone in the side menu first leaves the archive and only then narrows the board — the archive listing doesn't break down by milestone, and a chip promising one would be a lie. For the same reason, leaving the archive also drops a milestone filter you set while inside it.

## How a task gets into the archive

The **Archive** action lives in two places:

- in the card menu — the three dots in the card's top right corner;
- in the task itself — the archive icon in the row of actions at the bottom of the window.

Both ask for confirmation. Subtasks go into the archive **together with their parent**; the “detach subtasks” choice offered on a computer does not exist in the app. That is why archived cards are shown flat: their children are already lying next to them, inside the same archive.

## Restoring and deleting forever

An archived card has its own menu — the same three dots, a different set:

- **Open** — read the task with all of its history and comments.
- **Restore** — the task returns to the board, into its column, and leaves the archive listing at once. Subtasks that went in with their parent come back with it.
- **Delete forever** — the task is erased outright, bypassing the board. This can't be undone, so it sits behind a confirmation naming the task.

## Archiving and deleting are not the same

The difference matters and is easy to miss:

- **Archive** — the task is kept with all of its history, comments and attachments, it stays visible in the archive and can be brought back.
- **Delete** (in a live card's menu) and **Delete forever** (in the archive) — the task is gone for good.
- **Deleting a column** — the column disappears **along with its tasks, bypassing the archive**. Before deleting a column, move the tasks you need into another one or send them to the archive.

When in doubt, archive. It exists precisely so you don't have to choose between “it's in the way” and “I'd hate to lose it”.
