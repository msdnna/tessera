---
title: GitLab synchronization in detail
category: Administration
order: 86
keywords: administration, gitlab, synchronization, write-back, labels, tags, conflicts, journal, issue
updated: 2026-08-26
platforms: web
---

Once the [project binding](/help/admin-gitlab-project) is set up, what remains is deciding what exactly moves between Tessera and GitLab, and in which direction. There are two directions, and they are configured separately:

- **from GitLab into Tessera** — synchronization reads issues and turns their labels into a column, a priority and tags. This is governed by the **label parsing rules**;
- **from Tessera into GitLab** — task changes are pushed back onto the issue. This is **write-back**; it is off by default and is turned on one action at a time.

When both sides change the same field, the change is not overwritten but held back as a **conflict**. What exactly each run did is visible in the **sync journal**.

All the settings below live in the **Integrations → GitLab** window and apply to the selected binding. Only the instance administrator can change them.

## Write-back to GitLab

The “Write-back to GitLab” section sits below the binding fields. The top toggle **“Enable write-back”** is the master switch: while it is off, nothing goes out to GitLab no matter how the other toggles are set.

**Create an issue from a task** adds a **“Create a GitLab issue”** button to the task modal (under the “Parent” property). The issue is opened from the task's title, description and properties, after which the task counts as synced. This is a separate capability: it works even if change write-back is not configured for any field.

**Fetch issue templates from the project** pulls in templates from `.gitlab/issue_templates/*.md` — when creating an issue, you can pick a template above the description editor.

**Push subtasks into the GitLab hierarchy** creates the subtasks of a grouped task as child items of the issue. The parent is marked by the label from the **“Grouped-task label”** field (empty — `M: Сгруппированная задача` is used). The label is set here rather than as a tag: it has to be read as grouping by the parsing rules, otherwise the server rejects the “Mark as grouped” button. **“Label the parent automatically”** is off by default — otherwise the very first subtask would quietly edit labels in GitLab; instead, a hint with a button appears in the subtask list.

**Upload attachments to GitLab** copies images from the description and comments, as well as the task's files, into the GitLab project's upload store. Without this, links inside the issue stay broken — GitLab resolves them against its own address. It is turned off when you would rather not duplicate binaries: links are then replaced with a note. Note: deleting a file in Tessera does not remove the GitLab copy — GitLab has no upload-deletion API.

### Write-back actions

What exactly happens to the issue is set by the actions table — the **“Configure actions”** button opens it on the right side of the window. The button is disabled until write-back is enabled.

![The “Write-back actions” pane on the right](../assets/gitlab-writeback-light.png)

Each row is an “event in Tessera → action in GitLab” pair, with its own toggle: a rule can be silenced without deleting it. Entries go out under the integration owner's token, which needs the `api` scope.

There are ten events in Tessera: move to a column, the “Completed” flag, a priority change, a due date, assignees, an estimate, a milestone, a title or description, tags, and a new comment. Some of them have a qualifier — a specific column, a specific priority level (“Any priority” = any change), the direction of the “Completed” flag (“Became ‘Completed’” / “‘Completed’ cleared” / any change).

There are nine actions in GitLab:

| Action | What it does |
|---|---|
| **Set a label** | puts the given label on the issue. The **“Clear labels with the same prefix”** toggle removes prefix neighbors — so `S: In progress` displaces `S: To do` rather than piling up alongside it |
| **Close / reopen the issue** | closes or reopens; the “From the ‘Completed’ flag” variant takes the direction from the task itself |
| **Set the due date** | writes `due_date`. An issue has no start date, so this variant is ignored for them |
| **Set the assignees** | carries over the task's assignees. Only those who signed in to Tessera through GitLab are matched |
| **Set the estimate** | writes `timeEstimate`; it only makes sense when the board counts the estimate in time |
| **Set the milestone** | sets the milestone — if the Tessera milestone is itself linked to GitLab |
| **Update the title/description** | carries the task's text over to the issue |
| **Reconcile the tags** | brings the issue's labels into line with the task's tags |
| **Post a comment** | posts a comment; **“Add a Tessera marker”** appends a signature at the end, by which a reader in GitLab can tell where the entry came from |

Moving to a column deliberately **sets a status label and does not close the issue**: closing is a separate “Completed” event, and mixing them would mean closing the issue every time a card reached the right edge of the board.

An empty table does not mean “do nothing”: if the actions were never configured, the set is restored from the previous write-back flags when the window opens, and it is shown ready for editing.

A subtlety about tags: the “Reconcile the tags” action works only when a tag name can be turned back into the full label name — that is, with **“Keep the tag prefix”** enabled in the parsing rules. If the prefix is dropped, there is no way to restore `T: bug` from the `bug` tag — and such entries are not queued at all.

### How changes are sent

Write-back is asynchronous: the task handler only puts the change into a queue, and a background worker processes it in batches roughly every ten seconds. That is why an unreachable GitLab does not slow down work in Tessera — the entry simply waits. A failed attempt is retried with a growing pause, up to five times, after which the entry is marked failed and stays in the journal. A series of edits to the same field collapses into a single entry (the last one wins) — comments never collapse.

## GitLab label parsing rules

The way back: how issue labels turn into a task's state. The **“Configure rules”** button opens the editor — it is available regardless of whether write-back is on.

![The “GitLab label parsing rules” pane on the right](../assets/gitlab-tags-light.png)

At the top are three shared parameters:

- **Default column** — where an issue lands if no status rule fired;
- **Other labels** — what to do with labels that matched no rule: “Create a tag” (the default) or “Ignore”;
- **Keep the tag prefix** — whether to keep the label prefix in the tag name (`T: bug`) or drop it (`bug`). Tag write-back depends on this too, see above.

Below is the list of rules. Each consists of a pattern, a match type and an action:

- **Prefix** — a label matches if it starts with the pattern (`S: `); the rest of the name is taken as the value;
- **Regex** — a label matches by a regular expression; the whole name is taken as the value.

There are six actions:

- **Status → column**, **Priority** and **Board (routing)** — with a mapping table of “label value → column / priority level / board”. Board routing is needed when one project's issues are spread across different boards;
- **Tag** — the label becomes a task tag (with its own prefix toggle);
- **Grouping (subtasks)** — the label marks the task as a grouped parent;
- **Ignore** — the label is dropped.

A rule of the “Prefix” type has a **friendly name** — the label under which the prefix is shown in the Tessera interface (for example, “Status” for `S: `). It is stored on the project, not on the rule, so it is visible on other tag screens too.

Parsing goes top to bottom: each label goes to the first rule it matches, and the status, priority and board are taken from the first match that fired. Values in the tables are compared case-insensitively — the label `S: In Progress` finds the row `In progress`.

Out of the box the rules are filled with the layout common to the project: `S: ` — status, `P: ` — priority, `M: ` — grouping, everything else becomes tags with the prefix kept. If a team uses a different label system, it is more sensible to rewrite the mapping tables than to rename labels in GitLab.

## Conflicts

Synchronization does not decide for you who is right. If **both you and GitLab** changed the same field since the last exchange, the entry is not sent but held back as a conflict. This is how the title and description, due date, estimate, the “Completed” state and priority behave — that is, the fields where losing someone else's edit would be noticeable.

An unresolved conflict is visible in three places at once: an orange **“Conflict”** badge on the task card, a counter on the “Sync” button and on the “Integrations” icon in the sidebar header, and the **“Conflicts”** item in the menu next to the sync button (it is active only when there is something to resolve).

The conflicts pane shows a list of tasks on the left and, on the right, a per-field breakdown in three columns: **“Before (base)”** — the value as of the last sync, **“GitLab”** and **“Tessera”** — what each side became. For the title and description, differences are highlighted line by line.

A decision is made with buttons: keep mine, keep the GitLab value, or **merge by hand** — the last is available for text and numeric fields (for status and priority the choice is binary anyway). After resolving, the chosen value is sent to GitLab by the usual write-back, and the base snapshot is updated so the next edit is not treated as a discrepancy again.

## Sync journal

The **“Sync journal”** item in the “Sync” button's menu opens the run history. On the left is a list: the type (**Pull** — reading from GitLab, **Push** — delivering write-back), the start time, what triggered it (manual or auto), the outcome and the duration. The dot on the right shows the state: running, success, partial, error. A run happening right now is visible live — the time counter ticks, and when it ends the row changes its status by itself, with no need to reopen the pane.

A pull run's counters read like this: `+N` — tasks created, `~N` — updated, “no changes” — the pass completed and found no discrepancies. A push run counts deliveries.

Clicking a run expands its list of actions; clicking an action shows the details on the right: which fields changed and to what, which tags, comments and assignees were touched. A long run is paged — **“Show more”** appears at the bottom of the list. A failed delivery (push only) has a **“Retry”** button — it puts the entry back into the queue.

## If something goes wrong

- **Changes do not go out to GitLab.** Check the master “Enable write-back” toggle, then whether the table has an enabled action for the event in question. An event with no action is not queued at all, and leaves no trace of itself in the journal.
- **Tasks arrive in the wrong column.** Look at the status rule: most likely the label value is missing from the mapping table, and the default column fired.
- **Labels arrive as tags instead of a status.** No rule matched, and the label was picked up by “Other labels → Create a tag”. Most often the culprit is the prefix: the rule has `S: ` with a space, while the GitLab label is `S:In progress`.
- **Tags do not go back to GitLab.** Enable “Keep the tag prefix” — without the prefix a tag name is not turned back into a label name.
- **Assignees are not carried over either way.** People are matched by GitLab identity: until a person has signed in to Tessera through GitLab at least once, there is nothing to match them against.
- **An entry is marked failed.** Open it in the journal — the GitLab error text is there. After fixing the cause (token permissions, a deleted issue, an unreachable server) click “Retry”.
- **The “Mark as grouped” button does not work.** The grouped-task label has to fall under a rule with the “Grouping” action; otherwise Tessera would set a label it would then read back as an ordinary tag.
