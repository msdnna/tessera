---
title: A task's history
category: Tasks
order: 35
keywords: history, journal, audit, who changed, events, log
updated: 2026-08-28
---

The **History** tab is the task's journal: who did what to it, and when. It is kept automatically — nothing has to be switched on.

![The History tab: the task's journal](../assets/task-history-light.png)

Each row is an avatar and a name, a short sentence about what happened, and a time. Entries run **top to bottom in time**: the first line is the task being created, the last is whatever just happened.

## What goes into the journal

| Event | How it reads |
|---|---|
| Creation | created the task |
| Rename | renamed → “a new title” |
| Description edit | changed the description |
| Priority | changed the priority → high |
| Due date | set the due date · cleared the due date |
| Completion | marked it done · reopened it |
| Recurrence | rescheduled the repeat |
| Column | moved it → “In progress” |
| Assignees | assigned someone · unassigned someone |
| Archive | archived it · restored it from the archive |
| Comment | left a comment |
| [Relation](/help/task-links) | added a relation to #2591 |
| Attachment | attached the file “report.pdf” |

## What the journal does not keep

Understanding this matters as much as knowing what it does keep:

- **It doesn't keep former values.** A row says “changed the description”, but not what it looked like before. A task's history is not a version history; versions belong to [documents](/help/documents), not to tasks.
- **It doesn't keep the text of a comment.** The journal holds “left a comment”; the text itself lives in the [Comments](/help/comments) tab — and if the comment was deleted, the journal will not bring it back.
- **It is neither deleted nor edited.** Entries are only ever appended.

## Relations leave a trace on both sides

[Relations](/help/task-links) have a quirk that explains some of the entries. The relation itself is stored on the task it was created from, but **it is written into both journals**: #10 gets “added a relation to #20”, #20 gets “added a relation to #10”.

So if the Relations tab is empty while the history mentions a relation, everything is fine. The relation exists — it was simply created from the other end.

## Commands count as one entry

A comment with [commands](/help/comments) doesn't file a row per command: five commands in one comment produce **one entry** about what was applied. Otherwise a single comment could push the rest of a task's history out of sight.
