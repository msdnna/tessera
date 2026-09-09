---
title: Documents
category: Documents
order: 40
keywords: documents, editor, docx, word, import, export, pdf, versions, history, comments, discussions, approval, templates, app
updated: 2026-09-09
---

The **Documents** section is a collaborative editor for what doesn't fit into a task description: requirements, meeting notes, guides. The section opens from the “Documents” item in the side menu (the “☰” on the left of the top bar).

**Documents can be read and edited right from your phone** — with the same editor as in the browser. This is not a cut-down set of paragraphs: blocks, tables, page layout, PDF blocks and a slash menu are all available. For an overview of what the section can do, see [Documents](/help/documents); here is what differs on a phone.

![The Documents section in the app](../assets/documents-mobile-light.png)

## Structure

Documents are filed into a tree — a document can have nested ones, so the section behaves like a small wiki. On a phone a grid with breadcrumbs is shown first, and a tap on a document slides a **reader out over it**. An open document takes over the app's header: its title replaces “Documents” and opens a menu with everything else — discussions, links and approval, history, table of contents, export, rename and delete, jumping to the parent and nested documents, and back to the list. Creating a document (nested ones included), renaming and deleting all live in this menu; deleting a container warns about its nested documents.

## Editing

The reader has a pencil that opens the editor. Editing works block by block: the handle to the left of a block lets you insert a block, discuss it, or drag it. While several people have the document open, a block a colleague is holding right now won't let itself be interrupted and shows whose name it is; if the document was changed elsewhere, the app offers to refresh rather than overwriting your edits. Leaving the editor first flushes whatever was queued for autosave.

## Discussions, history, links

The panels that sit beside the text on a wide screen open as **bottom sheets** on a phone — one at a time, dismissing one another:

- **Discussions.** A button with a count of open threads — on both the reader and the editor. A comment is anchored to a block; the block's quote stays next to it as it was at the moment you tapped. Threads are filed by address: to a live block, to the whole document, and separately those whose block has since been deleted.
- **Version history.** A list of edits top to bottom; named snapshots stand out among the autosaved ones. Any entry opens as a comparison against the current state — one document where added, removed, changed and moved blocks are marked with a word and a stripe on the left. Restoring is confirmed and saves the current state into history.
- **Links and approval.** A document is linked to a task whole or by a single block; approval shows the status, the number of signatures and a step-by-step route, and the “Sign” button appears only once it is your turn. The other end of the link is the “Documents” tab in the task itself.

## PDF, import and export

A PDF reads in place — page by page, with its name and size. **“Upload a file”** by the list: `.md` and `.json` the phone parses itself, office formats go to the server converter (if it is deployed), and a PDF is stored as an attached file and opens even without the converter. Next to it is a gallery of **templates**: your workspace's saved templates plus three built-in ones (meeting minutes, a specification, a retrospective). **Export** hands the document to the system “Share”; HTML is always offered, the rest when the converter is available.

## A document or a note

If a record is only for you and only for a couple of days, it's a [note](/help/notes). A document is for long material that others read and edit.
