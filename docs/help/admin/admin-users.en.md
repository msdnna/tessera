---
title: Instance users
category: Administration
order: 80
keywords: administration, admin, users, account, block, deactivation, password reset, permissions, role, instance
updated: 2026-10-08
platforms: web
---

The **Administration** screen manages the accounts of the whole Tessera instance: who can sign in, who has admin rights, and who to hand a recovery link. These are not workspace settings — the members of a particular workspace are invited and removed separately; here it's about entire accounts.

![The Administration screen, the Users section: the table of the instance's accounts](../assets/admin-users-light.png)

## Who the instance administrator is

**The first registered user automatically becomes the administrator** — the one who set up the instance. After that, rights are granted by hand, from an existing admin account.

An administrator sees an **Administration** item in the **“⋯”** menu of the sidebar footer — that's what opens this screen. Other users don't have the item, and going straight to the `/admin` address sends them back: access is checked on the server too, so you can't “guess the link”.

Instance admin rights give you: this screen, its **Integrations** section (sign-in through GitLab), the **Background jobs** window and control of the GitLab integration. Everyday work — boards, tasks, documents — doesn't depend on them.

## How the screen is laid out

On the left is the column of sections: **Users**, **Server** (the instance address and the API and web client versions), **Integrations** ([sign-in through GitLab](/help/admin-gitlab-oauth)) and **Background jobs** — the last item opens the [jobs window](/help/admin-jobs). The open section is kept in the address, so you can bookmark it. Items marked **soon** — invitations across all workspaces, roles and permissions — are still in the works.

## The list and search

The **Users** section is a table of every account; their total is next to the heading. The search field (the `/` key) filters by name and email as you type, and the switch next to it — **All**, **Active**, **Admins**, **Blocked** — has a count on each option.

Each row is a card of its own: avatar, name and email, **role**, **status** and a **“⋯”** menu. Your own row is marked **you**, and an unconfirmed email is marked **unverified** next to the address. The mark is informational: email confirmation doesn't block work, but neither the recovery email nor email notifications will reach an unconfirmed address.

From the keyboard: `↓` from the search moves into the table, `↑` `↓` walk the rows, `↵` opens the row's menu.

## Granting and revoking rights

The role in a row is a button: **Admin** (in the accent colour) or **User**. Clicking it opens the role menu. Granting rights is one item, **Administrator**. Revoking is the **User** item, and it takes two presses: the first turns it red and names whose rights are being taken away, the second revokes them. A careless click shouldn't leave the instance without an administrator.

You can't change your own role — in your own row the role is shown with a lock, and the server will reject such an attempt. This way a sole administrator can't accidentally demote themselves and lock themselves out of every admin screen.

## Blocking sign-in

**Block sign-in** in the row's “⋯” menu — also a two-press item — forbids the user from signing in. The account itself is kept whole: tasks, comments, authorship and mentions stay in place; the person just can't sign in any more. This is the standard way to handle someone leaving or to close a contractor's access — unlike deletion, it breaks nothing in the history.

A blocked account's row is dimmed, its status reads **Blocked** with a red dot, and the menu offers **Unblock sign-in** instead — access comes back with a single press. You can't block your own account.

## A link to recover access

**Copy password reset link** in the “⋯” menu (in your own row — the key button) creates a **password reset link** and copies it to the clipboard. The link is valid for **one hour**.

This helps when a user has forgotten their password and email isn't configured on the instance, or messages don't reach them: the administrator hands the link over through any available channel. If SMTP is configured, an email with the link also goes to the account's address.

The link is, in effect, temporary access to the account, so hand it over personally and don't post it in shared chats.

## What's next

What happens on the instance in the background — synchronizations, notification delivery, recurring tasks — is shown by the [Background jobs](/help/admin-jobs) window.
