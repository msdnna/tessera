---
title: Tags, prefixes and how to manage them
category: Tags
order: 36
keywords: tags, labels, prefixes, colon, rename, colour, delete, grouping, modal, Enter, GitLab, app
updated: 2026-08-28
---

A tag in Tessera is more than a coloured label. A board's columns can be laid out **by tag**, and then a tag becomes a direction of work: a team, a component, a client, a stage of refinement. In the app tags work just as they do in the browser — only the gestures differ, plus one setting the phone doesn't have.

## Tags belong to the project, not to the task

The set of tags belongs to the **project**: every board in it sees the same list, and a neighbouring project has its own. A tag removed from every task doesn't leave the list: it stays as a blank, and a column for it on the board is simply empty.

## Where to manage them

There is no «Tags» button in the phone's header — management lives in the **board menu**: «⋮» in the top right corner → **«Manage tags»**. A modal opens with the project's full tag list.

If the project has prefixed tags, the list is split into groups with an upper-case header above each. When there is only one group, no headers are drawn at all.

### Create

At the bottom of the modal there is a **«Tag name»** field with a **«Create»** button next to it. Unlike the browser, Enter isn't required here: the button exists and stays disabled while the field is empty. A new tag gets the first palette colour (purple) — the colour is changed separately, while renaming.

### Renaming — one tap

On a computer the editor opens on a double click. A double tap is awkward on a phone, so **one tap on the tag is enough**: the chip is replaced by a name field with a **«✓»** button to its right that saves and closes. Tapping empty space in the modal closes the editor without saving.

### Colour — only while editing

The palette of eight colours appears **under the tag you are editing**, and nowhere else. So the answer to "where do I change a tag's colour" is: tap the tag in this modal and pick a circle underneath. The colour applies immediately.

The colour isn't only decoration: it feeds the chip's gradient on a card, the column colour when grouping by tags, and the fill in filters.

### Delete

The bin on the right of a tag asks for confirmation — "Delete the tag? It will be removed from every task." And it will: the tag comes off **every task in the project at once**, with no undo. If a tag is simply no longer needed, it's safer to rename it (adding "archive", say) and stop using it.

## Prefixes: how tags become a system

Tessera reads a tag with a colon in its name as a **prefix and a value**:

- `Complexity::high` — the `::` separator;
- `Status: in review` — the `: ` separator (colon plus space).

Both spellings are equal. Everything before the separator is the **prefix** (a namespace), everything after is the value. Case and spaces around the prefix don't matter.

What that buys you on a phone:

1. **A two-segment chip.** A filled segment with the prefix name on the left, the value on the right. On a narrow card that saves more room than a long name.
2. **Grouped lists.** Both the tag-management modal and a task's tag picker collect tags into groups by prefix, each with a header — otherwise a list of forty tags is unscrollable on a phone.
3. **Columns from a single prefix.** In the board bar, the «By tags» grouping can take one **specific prefix** instead of all tags — then only its values become columns. More in [The board bar](/help/board-composer).

## Prefix names are set in the browser

A friendly name for a short prefix (`T:` → "Type") is a **project** setting, and the app honours it: the names are substituted into chips, group headers and filters. But there is **no editor for prefix names in the app** — the «Prefix names» section exists only in the web version. If the phone shows a bare `T` instead of a friendly name, that prefix has no name set in the browser yet (or the switch below is on).

## Short prefixes — a switch for yourself

Below the tag list sits the **«Short prefixes»** switch, captioned "Raw prefix (“T”) instead of the friendly name". It does the opposite of prefix names: it shows the prefix as it is.

This is a personal setting of **this device**: it doesn't change for your colleagues and doesn't travel between phone and browser. The switch only appears if the project actually has prefixed tags.

## Tags on a task

In the task window and in a card's quick menu, tags are picked from the same list grouped by prefix: tapping a tag adds it, tapping again removes it. An attached tag is filled with its colour; a detached one is pale.

**Creating a tag from here only works from the keyboard.** At the bottom of the picker there is a field captioned "New tag, Enter", and the caption isn't exaggerating: **there is no button next to it** — the tag is created by the confirm key on the on-screen keyboard. A name typed but not confirmed is lost, and the picker closes as if nothing happened. A tag created this way is attached to the task immediately and gets a **random** palette colour — if the colour matters, fix it afterwards in the tag-management modal.

One task can carry any number of tags. With the board grouped by tags it shows up **in every matching column** — it is the same card, not copies.

## Tags and GitLab

On a board linked to GitLab, repository labels arrive as tags. Some of them are parsed by rules into task fields: the label `S: done` may mean a column, `P: high` a priority. Such tags are **not offered in a task's picker**: attaching one by hand would contradict the field it drives. In the tag-management modal they are visible and editable like any other — there you are looking at the project's label list, not assigning them.

How the parsing rules themselves are configured is covered in [GitLab: synchronisation](/help/admin-gitlab-sync).
