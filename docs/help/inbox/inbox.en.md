---
title: Inbox
category: Inbox
order: 50
platforms: web
keywords: inbox, notes, reminders, remind, snooze, capture, record, G I, N M, N R, private notes, done
updated: 2026-10-08
---

The Inbox holds your quick records: notes and reminders in one feed. You can record something from anywhere without leaving the board, and the feed opens as a panel on the right, so it doesn't take you off the screen either. There are no separate Notes and Reminders sections any more — their old addresses open this panel.

Notifications (mentions, assignments, GitLab events) stay under the bell: only your own records go to the Inbox.

![The Inbox panel next to a board](../assets/inbox-light.png)

## Record

The **“＋”** button in the bottom right corner → **Record**, or the keys **N, then M**. A field opens by the button and reads the line as you type:

- a date or a time makes it a **reminder**: “tomorrow 10:00 call the bank”, “fri 18:30”, “14.10”, “in 2 h”. A date without a time means 09:00; a time already past today means tomorrow;
- otherwise it is a **note**;
- `/2951` or `/a word of the title` ties it to a task. A task chip appears under the line; if there is no such task, the chip turns red and the record can't be saved.

Under the line you see what came out: the date and time chip, the task and the title. **Tab** switches the type by hand (a “manual ×” mark appears; a date in a note is struck through and not applied). **Shift+↵** opens the record's text — the same Markdown editor as a task description; **Ctrl+↵** saves right from the text. **↵** saves, **Esc** closes, **↑** in the empty field brings back the last record.

**N, then R** opens the same field with the type already set to Reminder.

![The record field by the “＋” button: the line read as a reminder tied to a task](../assets/inbox-capture-light.png)

The line already understands `#tag` and `@person`, but saving them with the record is not there yet — coming soon.

## The feed

The panel opens with the tray button in the header (next to the bell; the number on it counts reminders that went off), the **Inbox** row in the sidebar, or the keys **G, then I**. **G I** again from inside the feed hides it. Drag the panel's left edge to change its width — it is remembered.

On top are the filters: **All · Reminders · Notes · This board** (the last shows records tied to tasks of the open board). Records are grouped:

- **Due now** — reminders whose time has come; “complete all” closes them at once;
- **Today** — what is still ahead today; what you completed today stays here struck through until midnight;
- **Later**;
- **Notes** — newest first.

A snoozed reminder shows a “from 12:00” chip — the time it was first set for.

## Keys in the feed

| Key | Action |
|---|---|
| ↑ ↓ | select a record |
| 1–4 | filter |
| ↵ | open: a note or a reminder without a task is edited right in the feed, a reminder with a task opens the task |
| E | complete the reminder (or reopen it) |
| H | snooze: in an hour, this evening, tomorrow morning, on Monday, by the task's due, or your own date in words — “thu 15:30”, “in 3 days” |
| T | turn into a task — it lands in the first column of the open board, the record's text becomes the description, the reminder's time the due date |
| Shift+D | turn a note into a document |
| L | tie to a task |
| ⌫ | delete |

Complete, snooze and delete happen at once, and a message with **Undo** (or **Ctrl+Z**) appears at the bottom. The same actions are in the record's menu: the “⋯” button on hover, or a right click.

## Who sees a record

Reminders are always yours alone. Notes made through the record field are private too — they carry a lock. Notes written earlier in the old Notes section stay shared with the workspace. You can change who sees your note from its menu: “Shared with the workspace”.

Records belong to the workspace you made them in. Old reminders tied neither to a task nor to a workspace show up in the feed of every workspace.

## Records on a task

In the task window, between the subtasks and the activity, there is a **Reminders and notes** section — your records tied to this task.

- **Remind** (**Shift+R**) lays out the quick options: **1** in an hour, **2** tomorrow morning, **3** by the task's due, **D** — a date in words.
- **Note** (**Shift+M**) opens the record field right in the section, with the task already filled in.

A click on a reminder opens Snooze, a click on a note edits its text.

On a board card your nearest reminder on the task shows as a bell with its time right after the due date; one that went off is highlighted orange. A click on it opens Snooze. Colleagues see only their own reminders on a card.

## My tasks

The start screen has an Inbox block on the right: today's reminders and the latest notes. “All N →” and **G I** open the panel.

## Delivery

A reminder that goes off is delivered as before: a push in the Android app and to external channels if they are enabled in [notifications](/help/notifications). A snoozed one fires again at the new time; a completed one is not sent any more.
