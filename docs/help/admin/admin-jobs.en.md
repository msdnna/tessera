---
title: Background jobs
category: Administration
order: 88
keywords: administration, background jobs, workers, worker, synchronization, queue, log, error, run
updated: 2026-10-08
platforms: web
---

Some of Tessera's work happens on its own, out of the user's sight: it polls GitLab, pushes changes there, sends notifications, watches reminder due dates and creates recurring tasks. The **Background jobs** window shows what of this is running right now, when it will happen next, and what ended with an error.

It's opened with the keys <kbd>G</kbd> <kbd>J</kbd>, the **Background jobs** item in the “⋯” menu of the sidebar footer or in the column of sections on the [Administration](/help/admin-users) screen — all of them only for an instance admin.

![The Background jobs window: the filters on the left, the list, the selected job's card](../assets/background-jobs-light.png)

## A worker and a sync are different things

There are two kinds of rows in the list, and they shouldn't be confused.

A **worker** is a permanent loop that lives for as long as the server runs and wakes up on its own interval. It's almost always “running”: that's the state of the loop itself, not a sign that heavy work is happening right now. For a worker the **Next run** line is what matters — how long until it wakes up.

A **sync** is a one-off pass with a beginning and an end: a specific run of the exchange with GitLab for a single integration. It has a duration, a mode, and counters of what was created and updated.

## The permanent workers

There are five of them, each with its own role:

| Worker                       | Interval | What it does                                                        |
| ---------------------------- | -------- | ------------------------------------------------------------------- |
| GitLab auto-sync             | 30 s     | checks which integrations are due for a sync and starts them        |
| GitLab write-back            | 10 s     | pushes the accumulated task changes to GitLab (write-back)          |
| Notification delivery        | 10 s     | works through the notification queue and sends them over channels   |
| Due dates and reminders scan | 60 s     | looks for task due dates that have arrived and reminders that fired |
| Recurring tasks              | 60 s     | creates the next occurrence on schedule                             |

The intervals are fixed; you can't configure them from the interface.

## How to read the window

On the left are the filters: **All · Running · Queued · Failed · Finished** and, by type, **Workers** and **GitLab**; each has a count, errors in red. The digits <kbd>1</kbd>–<kbd>7</kbd> switch them from the keyboard.

In the middle is the list of jobs with a search (<kbd>/</kbd>); <kbd>↑</kbd> <kbd>↓</kbd> walk the rows. Under the name a worker shows what it does and when it runs next, a sync — its mode and what started it. On the right of a row: “Queued” or “Error” as a pill, the running time of a sync in progress, a tick with the finish time for a finished one, a green dot for a live worker. The list refreshes itself every few seconds; the arrows button in the window's header refreshes it immediately, next to the “running · queued” summary.

On the right is the selected job's card: status, for a worker — the last activity and the next run, for a sync — the mode (**incremental** or **full**), what started it (**manual** or **scheduled**), the start, the finish, the duration and the **Created / updated** counters. If the job failed, the error text sits below on a red backing. A job's steps, progress and live log, and the 30-day history, are marked “soon” for now.

## The “journal” badge

The **journal** badge by a name means the record was taken from the saved synchronization log, not from the process's memory: such rows survive a server restart. Everything else is live state, and after a restart the list of workers starts from a clean slate.

The window shows completed runs from the last hour. Older ones drop out of the window — this is a display of the current state, not the full history; you look up the history for a specific integration in its synchronization log in its settings.

## Run and stop

**Run now** (<kbd>R</kbd>) does the worker's work immediately, without waiting for the next tick. It's what you need when a change has already been made and you don't want to wait a minute — for example, to check whether a notification goes out. The button is only on workers: a one-off sync of a specific integration is started from its own settings, not from here.

**Stop** (<kbd>⇧</kbd> <kbd>⌫</kbd>) appears on a running job that supports cancellation — right now that's the GitLab syncs. Cancellation isn't instant: the job stops at the nearest safe step, so the row stays in the “running” state for a while. Data already moved isn't rolled back on cancellation — the next pass simply continues from where the previous one got to.

## If a job ended with an error

The red dot and the error text in the card are the first thing to read: most often it names outright the unreachable GitLab address, an expired token or missing permissions. The usual order of action is:

1. read the error text in the job's card;
2. check the integration settings — the address, the service token, the token's permissions;
3. run the work again (“Run now” for a worker, a repeat sync — from the integration settings) and see whether the error recurs.

One failed attempt doesn't stop a worker: it keeps waking up on its interval and tries again.

## What's next

Who has access to this window and the other admin screens is covered in the [Instance users](/help/admin-users) article.
