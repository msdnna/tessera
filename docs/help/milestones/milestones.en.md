---
title: Milestones
category: Milestones
order: 60
keywords: milestones, milestone, release, progress, due dates, gitlab
updated: 2026-08-21
---

A milestone is a point by which a set of tasks should be ready: a release, a demo, a hand-off to the client. The section opens from the **Milestones** item in the sidebar and shows the milestones of every project in the workspace at once.

![The Milestones section: project milestones with progress](../assets/milestones-light.png)

## What the list shows

Milestones are grouped by project. A milestone row carries the name, the start and end dates, a progress bar and a counter of completed tasks (for example, `3/6`). Until a milestone has a single task attached, “no tasks” shows in place of the progress.

The toggle at the top right chooses what to show: **Active** — only milestones that aren't closed, **All** — closed ones included. Clicking a row opens the project board filtered by that milestone, so you can see at a glance what's left.

The gear next to a project name opens the management of its milestones: creating, renaming, dates, closing.

## How a task joins a milestone

A milestone is picked in the task window, in the **Milestone** field. A task belongs to a single milestone; clear the choice and the task drops out of the milestone, and the progress is recalculated.

On a board you can group and filter by milestones just as by tags and assignees — see [Boards and tasks](/help/boards-and-tasks).

## The link with GitLab

If a project is linked to GitLab, a milestone is synced with the repository milestone of the same name: a GitLab icon appears next to the name, leading to the milestone in GitLab itself. The sync is optional — milestones work fully without it.
