---
title: The description and comment editor
category: Tasks
order: 31
keywords: markdown, editor, toolbar, formatting, image, mermaid, diagram, fullscreen, preview, spoiler, checkbox, code, app
updated: 2026-08-28
---

The same editor as in the browser: it stores **Markdown**, not some house format. In the app, though, it lives in exactly two places — the **task description** and **comments** (including editing a sent one and replying inside a thread).

**Notes on a phone are edited in a plain text field** — no toolbar, no preview, no mentions. Markdown you type there is kept and renders fine in the browser, but you have to write the syntax by hand.

## Two rows above the field

A row of round buttons on the top right, a formatting strip below it.

**The top row** (in edit mode):

| Button | What it does |
|---|---|
| Image | picks an image from the gallery or files |
| Graph nodes | inserts a Mermaid diagram stub |
| Eye / pencil | switches edit ⇄ preview |
| Outward arrows | opens the fullscreen editor |

**Comments have no eye in that row**: the toggle sits at the bottom instead, next to the Send / Cancel buttons. That way it does not scroll out of reach once the comment grows.

**The formatting strip** scrolls sideways — there are more buttons than fit a phone's width: **B**, *I*, ~~S~~, `</>`, **H** (`## `), **•** (`- `), numbered list, `[ ]`, **❝** (`> `), spoiler, link, and at the end **two chevrons, left and right**.

The chevrons are **outdent and indent**. They exist only in the app: a soft keyboard has no Tab, and a hardware one is not something we can count on. Indentation steps by **two spaces** — exactly what Markdown needs for a nested list item.

Buttons act on the selection: with a selection they wrap it, without one they insert a stub and put the caret where you should type.

**There is no paperclip in the app.** A file is attached on the **Files** tab with the «Attach file» button; no link is inserted into the text — you look at attachments on that tab.

## What the editor does for you as you type

This is not the toolbar but the behaviour of the field itself — something the web editor on a phone cannot give you.

**Lists continue themselves.** Enter at the end of an item starts the next one with the same marker: `- ` gives `- `, `3. ` gives `4. `, `- [ ] ` gives an empty checkbox. Indentation is kept. **Enter on an empty item leaves the list** — the marker is removed and no stray blank line is left behind.

**Brackets and quotes close as a pair.** `(`, `[`, `{`, `"`, `'` and `` ` `` insert their closing character right away and put the caret between them. Typing the closing character does not duplicate it — the caret simply steps over the one already there. Backspace between a pair deletes **both**.

**A character typed over a selection wraps it instead of erasing it.** Select a word, type `*`, and you get `*word*` with the selection intact. The same goes for `_`, `~`, `` ` ``, `(`, `[`, `{`, `<`, `"` and `'`.

`*`, `_` and `~` **deliberately do not auto-close** while you type: otherwise «a * b» would turn into «a ** b». `<` is left out too — it would fight with `<details>`.

There is one rule that matters: **while predictive input is live (the word is underlined), the editor keeps its hands off the text**. Rewriting letters under a keyboard that considers them its own is a reliable way to get garbage.

## The input field

Monospace: nested list indentation lines up in a column, so you can see which level you are on. The field **grows to fit the text** — it has no scrollbar of its own, and a long description scrolls with the tab.

When the keyboard comes up, the app brings the **bottom** of the block into view — the field, plus the suggestion list under it if you are typing `@` or `/`. The scroll keeps up while the keyboard animates in: a single jump would fall short and leave the suggestions hidden behind it.

## Preview and fullscreen

The eye shows what everybody else will see. **Checkboxes are tappable in the preview** — `- [ ]` becomes `- [x]` in the Markdown itself, and that is saved. In a posted comment checkboxes are tappable **only in your own**: someone else's text is edited by its author.

The arrows button opens the **fullscreen editor** with two panes:

- **phone in portrait** — text on top, live preview below;
- **landscape and tablet** — panes side by side, as in the browser.

The panes **scroll in sync**: drag one and the other follows, proportionally to its own height. There is no eye inside the fullscreen window — the preview is already in front of you. Closing the window saves the text; it is the same as moving focus out of the field.

The chosen mode (edit or preview) **survives switching tabs and rotating the screen**: a task tab leaves the composition when you go to Comments, and without this you would land back in edit mode every time.

## Diagrams, syntax highlighting and the internet

Rendered text is drawn by a **WebView** — there is no other sane way to reproduce Mermaid diagrams and syntax highlighting on a phone. The markup itself is parsed by a library bundled into the app, but **Mermaid and the highlighter are fetched from the network**.

Hence the one real difference from the browser: **with no internet a diagram stays a code block and code stays unhighlighted**. Text, lists, tables, images and links still render as usual. Once you are online again, the next open puts everything in place.

Inside a code block **nothing is substituted**: `#2550` does not become a link, `@root` does not become a mention, and `/close` does not run.

## Images

The app has one path: **the image button**. Pasting from the clipboard and dropping a file, as in the browser, are not here — the input field has no such gestures.

The chosen file is uploaded and `![](url)` is appended **at the end of the text**, not at the caret. While the upload runs the button dims and a second tap does nothing.

## Mentions and task references

`@` opens the member list: avatar, name, and a «GitLab» marker on those without a Tessera account. `#123` in rendered text is a link — tap it and that task opens.

The difference is the same as in the browser: **only a comment sends a mention notification**. In a description `@Name` is highlighted, but the person gets nothing. See the article on [comments](/help/comments).

## What the app does not have

- **A floating bar over the selection** — formatting goes through the button strip.
- **A paperclip and attachments from inside the text** — use the Files tab.
- **Pasting an image from the clipboard, or dropping a file.**
- **Ctrl+Enter** — a comment is sent with the Send button.
