---
title: Tags, prefixes and how to manage them
category: Tags
order: 36
keywords: tags, labels, prefixes, colon, rename, colour, delete, merge, duplicates, grouping, project window, Enter, GitLab
updated: 2026-10-07
---

A tag in Tessera is more than a coloured label. Board columns can be laid out **by tag**, and then a tag becomes a direction of work: a team, a component, a client, a stage of refinement. That is why tags are worth creating deliberately and naming systematically — the second view of your board rests on them.

![The tag table in the «Project» window: prefix groups, selection and merging](../assets/project-tags-light.png)

## Tags belong to the project, not to the task

The set of tags belongs to the **project**: every board of one project sees the same list, and a neighbouring project has its own. A tag removed from every task does not disappear from the list — it stays as a blank, and its column on a tag-grouped board is simply empty.

## Where to manage them

A project's tags are the **«Tags»** tab of the [«Project» window](/help/project-window). Open it with the **«Tags»** button in the board header, with **«Manage tags…»** in a task's tag picker, or with **Ctrl+,** and the digit **2**. It is a table: tags grouped by prefix, each with its colour, how many tasks carry it and where it came from (**Tessera** — created here, **GitLab label** — brought in by the sync).

### Create

The **«New tag»** field above the table (key **N**). Type a name and press **Enter**. `prefix::name` puts the tag into that prefix's group, and creates the group when there is none. A new tag gets a random palette colour. A name the project already has (case aside) is not created — the field says so.

Next to it is the **filter** (key **/**): it matches the tag name and its group's friendly name.

### Rename — with a click, no double click

A click on a tag in the table, **Enter** or **F2** on its row turns it into a field. **Enter** or a click elsewhere saves, **Esc** cancels.

### Colour

The dot in the «Colour» column opens a palette of seven colours — a click applies it at once. Colour is not just decoration: it builds the chip gradient on a card, the column colour when grouping by tags and the fill in filters.

### Select several: merge, recolour, delete

The square to the left of a tag (or **Space** on its row) selects it, **Shift+↑/↓** — a range. An action bar appears at the bottom:

- **«Merge into…»** (**M**) — for duplicates like `design` / `дизайн` / `ui`. It offers the tag to merge into (the most used by default), and once confirmed every task of the others gets it and the others disappear. Task history shows the merge as an ordinary tag edit.
- **«Recolour»** (**C**) — one colour for all the selected.
- **«Delete»** (**⌫**) — at once, from every task of the project. An **«Undo»** toast appears at the bottom — 7 seconds to bring the tags back together with the tasks they were on (or **Ctrl+Z**).

**Esc** clears the selection. The same actions for one tag are in its row's «⋯» menu.

## Prefixes: how tags become a system

Tessera reads a tag whose name contains a colon as a **prefix and a value**:

- `Difficulty::high` — the `::` separator;
- `Status: in review` — the `: ` separator (a colon and a space).

Both spellings are equal. Everything before the separator is the **prefix** (the namespace), everything after it is the value.

What that buys you:

1. **A two-segment chip.** Such a tag is drawn the way GitLab draws it: a filled segment with the prefix name on the left, the value on the right. One glance at the card and it is clear that “high” is about difficulty, not about priority.
2. **Grouped lists.** In the tag popover, in a task's tag picker and in the board filter, tags are collected into groups by prefix, each with its own header. A list of forty tags stays readable.
3. **Columns for a single prefix.** A board can be grouped not “by all tags” but **by one prefix** — then only its values become columns, and the rest of the tags stay out of the layout. See [The board bar](/help/board-composer).

Case and spaces around the prefix do not matter: `Difficulty::` and `difficulty ::` are the same prefix.

## Prefix names

A prefix in a tag name is often short — `T: bug`, `S: doing`. That reads poorly on a card, so a group in the table has a **friendly name**: a click on the group header (or **F2** on it) opens a field right in the header. Name `T:` «Type» and every chip, group header and filter shows «Type» instead of `T`. It saves on **Enter** or a click elsewhere; an empty field removes the name and the prefix shows as is again. The raw prefix is always visible next to it on a grey plate.

Prefix names are a **project** setting: every member sees them. Editing one group leaves the names of the others alone.

## Short prefixes — a switch for yourself

On the right of the «Tags» tab header sits the **Short prefixes** switch. It does the opposite: it shows the raw prefix (`T`) instead of the friendly name (“Type”). This is a personal setting of **this device** — it does not change for your colleagues and does not travel to your phone. Handy when a card carries many chips and space is worth more than words.

## Tags on a task

In the task window and in a card's quick menu, tags are picked from the same list grouped by prefix: click a tag to attach it, click again to remove it.

**The main stumbling block when moving from ClickUp is creating a tag right there.** There is a field at the bottom of the picker labelled “New tag, Enter”, and the label does not overstate it: **there is no button next to it, a tag is created only with Enter**. A name typed and not confirmed is not saved, and the popover closes as if nothing happened. A tag created this way is attached to the task straight away and gets a random colour from the palette — if the colour matters, fix it in the tag table: **«Manage tags…»** under the picker opens it filtered by what you typed.

One task can carry any number of tags. When the board is grouped by tags it appears **in every matching column** — the same card, not copies.

## Tags and GitLab

On a board linked to GitLab, labels come in from the repository as tags — their source in the table is «GitLab label». Some of them are parsed by rules into task fields: an `S: done` label can mean a column, `P: high` a priority. Such tags **are not offered in a task's picker**: attaching one by hand would split it from the field it drives.

Tags whose prefix falls under a GitLab rule carry a **lock** in the table: they cannot be selected, renamed or recoloured — their name and colour come from GitLab, and the next sync would put its own back. The hint on such a tag says which repository the rule comes from.

How the parsing rules are set up is in [GitLab: sync](/help/admin-gitlab-sync).
