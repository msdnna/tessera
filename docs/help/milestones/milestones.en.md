---
title: Milestones
category: Milestones
order: 60
keywords: milestones, milestones, sprint, release, progress, burn-up, burn-down, metrics, goals, dates, closed, backlog, milestone strip, project tree, gitlab
updated: 2026-10-07
---

A milestone (stage, sprint) is a point by which a set of tasks must be done: a release, a demo, a sprint, a hand-over. Each milestone has goals, dates, its own page with metrics, and on the board — a strip with its progress.

![The milestone page: goals, burn-up and metrics](../assets/stage-page-light.png)

## Where to find milestones

- **The «Milestones» section** in the sidebar — the milestones of every project of the workspace at once, with progress. The **«Active» / «All»** switch adds the closed ones. A click on a row opens the milestone page; the gear by a project opens its milestones in the «Project» window.
- **The project tree.** Project menu → «Show in the tree» → «Milestones» or «Boards and milestones»: the project's milestones appear under it with their percentage, plus a **«Backlog»** row (tasks without a milestone). A milestone opens its page, «Backlog» the project's board with that filter. Closed ones stay hidden until «Show closed milestones» is on in the same menu.
- **The «Project» window → «Milestones»** — managing the project's milestones (below).
- **The milestone selector on the board** — filtering the board by milestone (below).

## States

A milestone stores two states, as GitLab does: **active** and **closed**. **«Planned»** is an active milestone whose start is still ahead; Tessera shows it so by itself, there is nothing to set. A closed milestone remembers when it was closed.

## Managing a project's milestones

The **«Milestones»** tab of the [«Project» window](/help/project-window): a milestone row shows the name, «N days left», dates, state, progress «done / total» and the GitLab milestone number. Closed milestones sit in a folding group at the bottom.

![A project's milestones in the «Project» window](../assets/project-stages-light.png)

- **Create** — the **«+ Milestone»** button or **N**: an empty row, the name, **Tab** — dates, **Enter** — create (and type the next one right away). Dates are optional.
- **Open** — a click on the row or **Enter**: the milestone page.
- **Row menu** («⋯» or **.**): rename (**F2**), dates, close or reopen, create the milestone in GitLab, delete. Deleting takes two steps; the tasks stay but lose the milestone.

## The milestone page

It opens from the milestone list, from the tree, with a click on the name in the milestone strip, and with **G**, then **E** from a board filtered by a milestone.

- **Header**: name, state, dates, days left, a link to the GitLab milestone. **«Open the milestone board»** (**B**) — the board holding most of the milestone's tasks, already filtered — and **«Close the milestone»** (**Shift+X**, two steps: the first press asks «Close it for sure?»).
- **Goals** — the milestone description in Markdown. **«Edit»** or **E** opens the editor with **@** mentions; **Ctrl+Enter** saves, **Esc** cancels (with a second press if the text changed).
- **Burn-up** — the «done» line against the scope line over the milestone's days, and dashed — the forecast to the end of the milestone at the pace of the last five working days. Switches: **tasks / estimate**, **burn-up / burn-down**. Weekends sit on a grey band, today is a vertical mark. Before the milestone starts there is no chart.
- **Progress** — two rings: by task count and by estimate.
- **Signals** — «overdue», «no estimate», «no assignee»: a click filters the task list below.
- **By status** — a bar in the column colours.
- **Scope change** — how many tasks were added and removed after the milestone's first day and how the scope moved.
- **By assignee** — done / total and the estimate per person.
- **Pace** — what this milestone has done against the three previous closed ones, with the average and the forecast.
- **Milestone tasks** — by status, unfinished first; a click opens the task.

Scope and burn-up are counted from history: every time a task joins or leaves a milestone it is written to the task's history (by hand, by the GitLab sync or by a repeating task). Tasks that were in a milestone before Tessera started recording this count as in it from the day they were created.

## On the board: the selector and the milestone strip

In the board's sub-toolbar, next to the grouping, is the **milestone selector** (**Shift+E**): «Milestone: Sprint 42 ▬ 58 %». Its menu: active milestones with progress, planned ones, **«Backlog»** (**B**) — tasks without a milestone, **«All tasks»** (**A**) — no filter; closed ones under «Show closed». A search on top, «Manage milestones…» at the bottom. The pick is remembered per board: come back and you see the same milestone.

With one milestone picked, a **milestone strip** appears between the sub-toolbar and the columns. Folded, it is one row: the name, dates, progress «18 of 31», the first line of the goals and the days left. **Shift+H** or the chevron unfolds it: the goals in full and three figures — progress by estimate, scope change since the start, overdue. The strip remembers its state. Its «⋯» menu: close the milestone, open it in GitLab, **hide the strip on this board** (bring it back with the same **Shift+H**). Without goals the strip offers «Add goals →».

![The milestone strip over the board](../assets/stage-strip-light.png)

A task created on a board filtered by a milestone gets that milestone right away.

## How a task gets into a milestone

The milestone is picked in the task window, in the **«Milestone»** field; you can also create one there on the fly. A task belongs to one milestone. The board can also be **grouped** by milestone (columns become milestones, plus «No milestone») — see [Board toolbar](/help/board-composer).

## GitLab

If the project is linked to GitLab, a milestone is synced with the repository's milestone: name, dates, state and description (the goals) come from GitLab. Such a milestone is **read-only in Tessera for now** — editing the goals and dates and closing it with the change written back to GitLab are still in the works, the buttons say «Soon». A Tessera-native milestone can be created in GitLab with the «To GitLab» menu item.

Deleting a linked milestone warns separately: **the milestone stays in GitLab**, only the link goes. How the link itself is set up is in [GitLab: sync](/help/admin-gitlab-sync).
