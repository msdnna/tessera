---
title: Board views
category: Boards
order: 24
keywords: saved views, save, update, reset, changed, default, star, shared, personal, rename, order, delete
updated: 2026-10-08
---

A combination of view, grouping, sorting, filters and card settings you have put together is a **saved view**. You can name and save it to come back to it in one click instead of assembling the chips again.

![The saved view chip on the board bar and its menu](../assets/board-views-light.png)

Saved views live in the first section of the [Board view](/help/board-customize) panel (`⇧V`). A loaded view shows a **chip** on the board bar, right after the view switch; with no view loaded there is no chip.

## What is saved

A saved view remembers how you look at the board:

- the view — kanban, list, timeline, Gantt, calendar or matrix; applying the saved view switches to it;
- the grouping and the tag prefix;
- every sort level with its order and direction;
- every filter, and whether subtasks are expanded;
- which columns are collapsed and whether empty ones collapse automatically;
- the card size, “Stacked”, “Show empty” and the visibility of each field;
- the matrix thresholds;
- the list's column widths, the calendar mode (month or week), the scale and the switches of the timeline and the Gantt.

Not saved are **the text in the search field** (that is your query of the moment), **the milestone** and **the archive** — that is where you are, not how you look.

## Mine and shared

The section shows one list in two parts:

- **Mine** — your personal views. Only you see them, on all your devices.
- **Shared on the board** — every member of the board sees them. The space owner and admins create and change them; everybody else sees a lock next to a shared view, and instead of “Update” they are offered “Save as mine” — a personal copy under the same name.

A row's place in the list is its key: with the panel open and in the chip's menu, `1`–`9` apply a view by its place, `0` means “No saved view”.

## Save current as…

At the bottom of the list is the **Save current as…** row (`⌘⇧S` from anywhere on the board). It turns into a field in place: type a name and press Enter. The owner and admins see a **Mine / Shared** toggle next to the field. A name has to be unique within its list: two personal “Sprint” views are not allowed, a personal one and a shared one are.

## “Changed”, “Update” and “Reset”

As soon as the board differs from the loaded view — you changed the grouping, dropped a filter, switched the card size — an orange dot appears on the chip and on the sliders button, and on top of the section a note says what exactly changed, with two buttons:

- **Update** (`⌘S`) — write the current state into the view;
- **Reset** (`⌘⌫`) — bring the board back to what the view stores.

The view never changes by itself: changes go into it only on “Update”. To keep them separately, press `⌘⇧S` — you get a new view and the old one stays as it was.

## Default

The **star** on a row makes the view open by default. On a personal view it means “by default for me”, on a shared one “for everybody on the board” (set by the owner or an admin). Your own star wins over the shared one.

The default view applies when you open the board in a new browser session. Within a session the board remembers where you left off: moving between boards and reloading the page keep your current settings.

## The row menu

Each row has a “⋯” menu: **Rename** (`F2` or a double click — the name is edited right in the row), **Update from current**, the **Open by default** star, **Make shared** / **Make personal** (for the owner and admins), **Move up** / **Move down** (`⌥↑` / `⌥↓`) and **Delete** (`⌫`). Deleting happens at once, and a toast with an undo appears at the bottom of the screen — `⌘Z` or the toast's button puts the view back where it was.

## No saved view

**No saved view** in the chip's menu (`0`) drops the view: the board stays as it is, it just no longer counts as being in a named view. That unnamed state is remembered for every board and every view kind, but **only on this device**. If a set of conditions has to survive a change of device, save it as a view.
