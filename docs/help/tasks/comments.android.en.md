---
title: Comments
category: Tasks
order: 32
keywords: comments, discussion, replies, thread, mention, commands, task reference, edit, delete, app
updated: 2026-08-28
---

The **Comments** tab is the second in a task's tab row. Comments are written with the same [Markdown editor](/help/markdown-editor) as the description, and can do all the same things: reply threads, mentions with a notification, and commands.

## The composer is at the bottom, not pinned

The main difference from the browser: **the input field is not pinned**. It sits after all the comments and **scrolls with them**. To write a new comment in a long discussion you have to scroll the tab to the end.

You send with the **Send** button. **There is no Ctrl+Enter in the app** — a soft keyboard does not offer that combination.

To the right of Send there is an **eye** — a preview of the draft. It is not in the editor itself: on a comment it would scroll out of reach along with the text.

## A send does not survive a lost connection

Worth knowing up front. In the browser a comment **is not lost** when the network drops: it stays in the field and the editor retries three times on its own.

**The app does not do this.** The field is cleared the moment you tap Send, and if the server does not answer, an error appears at the top and the text you typed is gone. Writing a long comment where the connection is flaky — copy it before sending.

## Editing and deleting

**Your own** comments carry a **pencil and a cross** in the row header, to the right of the date: edit and delete. A phone has no hover, so the buttons are always visible instead of appearing under the cursor as in the browser.

The pencil expands the same editor right in the row, with Save and Cancel buttons and a preview toggle beside them. Deleting happens at once, with no confirmation.

## Reply threads

A reply starts a **thread**: a root message with replies under it, tied to it by a **grey vertical rail** on the left. The root's avatar is slightly larger than the replies' — that is how you see where a thread begins.

There is a Reply button on the root and on every reply inside the thread. The author of the message you are answering is **pre-filled as a mention**: a Tessera member by display name, a GitLab-only one by their `@username` (a mention of their display name would not resolve to anyone).

Threads are **exactly two levels deep**: a reply to a reply joins the same thread. The app keeps **one reply composer for the whole tab** — open a reply in another thread and the previous form closes, taking whatever you had typed in it.

A long thread collapses with «Hide replies», leaving «3 replies» in their place; a tap expands it again. Collapsing is visible only to you. **Replying to a collapsed thread expands it** — answering blind, without seeing what has already been written, is a bad idea.

## Mentions

`@` opens the member list: avatar, name, and a «GitLab» marker on those without a Tessera account. Keep typing to filter the list.

The rule is the same as in the browser: **only the person you picked from the list gets a notification**. An `@Ivan` typed by hand is highlighted, but no notification goes out — Tessera does not guess which Ivan you meant.

Comments that came from GitLab are marked **«· GitLab»** next to the author's name, and their avatar is muted.

## Task references with `#`

`#123` in a posted comment becomes a **link**. A tap opens that task on top of the current one.

- The link is resolved **at tap time**, not at send time: write `#123` freely, it will work once the task exists.
- A number that does not exist says so honestly.
- Inside code blocks `#123` **stays plain text**.

## Commands

A command is a line that starts with `/`. Type `/` **at the start of a line** and a list appears under the field: a monospace `/key` with its description beside it. Workspace commands carry a grey **«custom»** badge — the app recognises them but does not run them.

The command hint appears **only in the new-comment composer**. It is not in the reply form or when editing a posted comment — commands do not run there either.

The set is the same as in the browser: `/assign`, `/unassign`, `/due`, `/start`, `/estimate`, `/priority`, `/title`, `/tag`, `/untag`, `/milestone`, `/move`, `/close`, `/reopen`, `/archive`, [relations](/help/task-links) `/relate` · `/blocks` · `/blocked_by` · `/duplicates` · `/unlink`, hierarchy `/subtask` · `/parent` · `/unparent`.

The rules are the same too: a command takes a whole line, there can be several (one per line, run top to bottom), executed lines do not stay in the comment's text, and a command inside a code block does not run.

### The hint before you send

A **grey strip parsing your draft** appears under the field: `/key` on the left, what will happen on the right («assigned @msdnna»), and an error in **red** («member not found»). Below it, if the text contains a workspace command, a line reading «/approve — will stay as plain text».

The parsing is done by **the server itself** — the same code that will later run the command, so the hint cannot disagree with the result. It is computed **shortly after you stop typing**: the strip does not appear instantly, and that is normal.

After sending, the app shows what was applied and **re-reads the task** — assignees, column and dates in the tabs update right away.

## An empty tab

With no comments yet, the list is replaced by the line «No comments yet», with the usual input field below it.
