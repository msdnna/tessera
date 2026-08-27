---
title: Tags, prefixes and how to manage them
category: Tags
order: 36
keywords: tags, labels, prefixes, colon, rename, colour, delete, grouping, popover, Enter, GitLab
updated: 2026-08-28
---

A tag in Tessera is more than a coloured label. Board columns can be laid out **by tag**, and then a tag becomes a direction of work: a team, a component, a client, a stage of refinement. That is why tags are worth creating deliberately and naming systematically — the second view of your board rests on them.

![Project tag management: the list, renaming and colour](../assets/tags-manager-light.png)

## Tags belong to the project, not to the task

The set of tags belongs to the **project**: every board of one project sees the same list, and a neighbouring project has its own. A tag removed from every task does not disappear from the list — it stays as a blank, and its column on a tag-grouped board is simply empty.

## Where to manage them

The **Tags** button in the board header opens a popover with the project's whole tag list. On a narrow screen the same thing lives in the board menu (the three-dot icon). The popover shows everything at once: which tags exist, what colour they are and how they group by prefix.

### Create

At the bottom of the popover there is a **New tag** field. Type a name and press **Add** or **Enter**. A new tag takes the first colour of the palette (purple) — the colour is changed separately, while renaming.

### Rename — with a double click

A single click on a tag in the popover does nothing: it is not a button but a sample of how the tag looks on a card. **Renaming opens on a double click** — the hover tooltip says as much (“*name* · double-click to rename”). The tag turns into an input; **Enter** or a click elsewhere saves it, and an unchanged name simply closes the field.

### Colour — only while editing

The palette of eight colours appears **under the tag you are editing**, and nowhere else. That is the answer to the frequent “where do I change a tag's colour”: double-click the tag → the row of circles under it → click the one you want. The colour applies immediately and the name field stays open.

The colour is not just decoration: the chip gradient on a card, the column colour under tag grouping and the fill in filters are all derived from it.

### Delete

The bin next to a tag removes it **from every task in the project at once** — the confirmation says so plainly. There is no undo: restoring a tag means creating it again and re-applying it by hand. If a tag is simply no longer needed but you would rather not break the history, it is safer to rename it (adding “archive”, say) and stop using it.

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

A prefix inside a tag name is often short — `T: bug`, `S: doing`. That is awkward to read on a card, which is what **Prefix names** at the bottom of the popover is for: next to every prefix found there is a field for a friendly name. Give `T:` the name “Type” and every chip, group header and filter shows “Type” instead of `T`. It is saved with **Enter** or by clicking outside the field.

Prefix names are a **project** setting: everyone sees them.

The section appears only once the project has at least one prefixed tag — otherwise there is nothing to name.

## Short prefixes — a switch for yourself

Next to it sits the **Short prefixes** switch. It does the opposite: it shows the raw prefix (`T`) instead of the friendly name (“Type”). This is a personal setting of **this device** — it does not change for your colleagues and does not travel to your phone. Handy when a card carries many chips and space is worth more than words.

## Tags on a task

In the task window and in a card's quick menu, tags are picked from the same list grouped by prefix: click a tag to attach it, click again to remove it.

**The main stumbling block when moving from ClickUp is creating a tag right there.** There is a field at the bottom of the picker labelled “New tag, Enter”, and the label does not overstate it: **there is no button next to it, a tag is created only with Enter**. A name typed and not confirmed is not saved, and the popover closes as if nothing happened. A tag created this way is attached to the task straight away and gets a random colour from the palette — if the colour matters, fix it afterwards in the board's tag popover.

One task can carry any number of tags. When the board is grouped by tags it appears **in every matching column** — the same card, not copies.

## Tags and GitLab

On a board linked to GitLab, repository labels arrive as tags. Some of them are parsed by rules into task fields: the label `S: done` may mean a column, `P: high` a priority. Such tags are **not offered in a task's picker**: attaching one by hand would mean disagreeing with the field it drives. In the board popover they are visible and editable like any other — there you are looking at the project's label list, not assigning labels.

How the parsing rules themselves are configured is covered in [GitLab: synchronisation](/help/admin-gitlab-sync).
