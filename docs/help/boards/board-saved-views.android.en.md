---
title: Saved views
category: Boards
order: 24
keywords: views, save, load, preset, overwrite, delete, app
updated: 2026-08-28
---

A combination of grouping, sorting, filters and card settings is a **view**. Name it and save it, and you can come back to it with a single tap instead of assembling the chips again.

In the app both operations — loading and saving — live in **one** sheet behind the folder icon, to the right of the [board bar](/help/board-composer). There are no two separate buttons like on the desktop.

## Where the folder is

The folder icon is visible **only while the board bar is collapsed**. Start typing in search or expand the bar, and the icon leaves together with the gear to free up width for the chips. Collapse the bar by tapping an empty spot and the icons come back.

While a named view is loaded, the folder is **highlighted with the accent colour**. A flat grey icon means you are looking at an unnamed set of conditions you assembled yourself.

## What a view stores

A view remembers what you see:

- the visualization (board, list, calendar, timeline, Gantt, matrix);
- the grouping, and the tag prefix if you group by prefix;
- every sort level — with its order and direction;
- every filter, **including the text in the search field**, and subtask expansion;
- which columns are collapsed and whether empty ones auto-collapse;
- card size, field stacking, showing empty fields and the visibility of each field.

It does not store the **scope** — the selected milestone or the archive: that is where you are, not how you look at it.

## Loading

Tapping a name in the list applies the view **immediately** and closes the sheet. The current one is marked with a check and the accent colour.

Note that a view carries its visualization. Load a view that was saved on the Gantt while you are on the kanban, and the board switches to the Gantt. That is not a glitch: the visualization is part of the saved set.

Here the app behaves differently from the browser. **The list shows every view of the board**, no matter which visualization you are on; on the desktop the list is narrowed to the current visualization. So on the phone you will also see names that the browser does not show you on the same board.

## Saving and overwriting

At the bottom of the sheet there is a “Name” field and a **“Save”** button; while the field is empty, the button is disabled.

The field is **pre-filled with the name of the current view** if one is loaded. That is how you overwrite: leave the text alone and press “Save” — the current conditions are written back into the same view. There is no separate “Overwrite:” row with name buttons like on the desktop.

Saving under a name you already have always overwrites it rather than creating a second view with the same name.

**There is no autosave on the phone.** The “Autosave” toggle exists only in the browser; in the app, even with a view loaded, changes to the bar are not written into it by themselves — until you press “Save”, you are editing an unnamed state. Which also means a filter you drop by accident cannot spoil your “reference” view here.

## Deleting

The bin next to a name asks for confirmation and deletes the view. Deleting the one currently loaded changes nothing on the board — you simply stop being in a named view, and the folder icon goes dim.

When a board has no saved views, the sheet says the list is empty.

## Views are personal and travel with you

Views are stored **on the server, separately for each user**. Two consequences follow:

- Your views are **available on all your devices** — in the app and in the browser, under the same account.
- **Nobody else sees them.** Two people on one board can each have their own “My tasks for the week” without getting in each other's way.

Separate from all this is the **unnamed** current state of the bar — what you just tapped together and never saved. It is remembered per board, but **only on this phone**: there is no server record behind it. If a set of conditions has to survive a change of device, save it as a view.
