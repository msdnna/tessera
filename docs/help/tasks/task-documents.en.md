---
title: A task's documents
category: Tasks
order: 34
keywords: documents, document link, block, fragment, quote, anchor, approval
updated: 2026-08-28
---

The **Documents** tab of the task window answers one question: **which documents talk about this task**. A specification, the minutes of a meeting, a policy — anything the task grew out of or is governed by.

![The Documents tab of the task window](../assets/task-documents-light.png)

## The link is made from the document's side

That's the main thing to know: **there is no “add” button in the Documents tab**. The link is created in the document itself — in the **Links and approvals** panel (the chain icon in the document header), with the **Link a task** button.

That's deliberate. A link means something where the context is: you are reading the paragraph about data import and you attach the import task next to it. From the task window there is no way to see which part of the document it refers to — and that is precisely what is most valuable about such a link.

The task's tab is the other end of the same link. Here you can **open the document** and **remove the link**; both work from either side, there is one row for one link.

## The whole document, or one block

In a document a link can be made two ways:

- **To the whole document** — when nothing is selected. The common case: “this task is about this policy”.
- **To a specific block** — when a paragraph, a list item or a heading is selected in the document. Such a link answers the question “which clause exactly is the task about”.

A link anchored to a block shows a **grey quote** next to the document title in the task's tab — a short fragment of that very paragraph.

The quote is taken **at the moment the link is created** and doesn't change afterwards. That isn't an oversight: the paragraph gets edited later, and the text the link was made for disappears. The stored quote is the only thing that will explain, half a year on, what this was about. If the fragment has been rewritten entirely, it's worth re-creating the link.

## What the tab shows

Each row is the document's icon or emoji, its title and — for a block link — the quote. Clicking the row **closes the task and opens the document**. It opens from the top: the quote in the row is what tells you which paragraph to look for.

Already-linked tasks **are not offered in the picker** on the document's side — linking the same pair again would change nothing, and a button that appears to do nothing would only be confusing.

## Links and approvals share one panel

In a document, task links live in the same panel as the **approval routes**, and that isn't an accident: a route is raised against the document, and the tasks are the reason the document exists at all. For more about documents themselves — import and export, version history, approvals, the table of contents and discussions — see [Documents](/help/documents).
