---
title: Instance users
category: Administration
order: 80
keywords: administration, admin, users, account, deactivation, password reset, permissions, instance
updated: 2026-08-25
platforms: web
---

The **Administration** screen manages the accounts of the whole Tessera instance: who can sign in, who has admin rights, and who to hand a recovery link. These are not workspace settings — the members of a particular workspace are invited and removed separately; here it's about entire accounts.

![The Administration screen: the “Sign in with GitLab” card and the list of instance users](../assets/admin-users-light.png)

## Who an instance admin is

The **first registered user becomes an admin automatically** — the one who set the instance up. After that, rights are granted by hand, from an existing admin account.

An admin sees the **Administration** button (a shield) in the sidebar footer — that's what opens this screen. Other users don't have the button, and going straight to the `/admin` address sends them back: access is checked on the server as well, so you can't “guess the link”.

Instance admin rights unlock: this screen, the **Sign in with GitLab (OAuth)** card on it, the **Background jobs** window, and management of the GitLab integration. Everyday work — boards, tasks, documents — doesn't depend on them.

## List and search

Below the OAuth card comes the list of all accounts; the screen heading shows their total number. The **Search by name or email** field filters the list as you type, across both fields at once.

Status badges appear next to a name:

- **admin** — the account has instance admin rights;
- **you** — this is your own row;
- **deactivated** — sign-in is blocked, the row is dimmed;
- **unverified** — the account's email isn't confirmed yet. The badge is informational: email verification doesn't block work, but neither a recovery email nor email notifications will reach an unverified address.

## Grant and revoke rights

The **Make admin** button grants rights in a single click. The reverse action — **Revoke admin** — asks for confirmation: a careless click shouldn't leave the instance without an admin.

You can't change your own row — it has none of these buttons, and the server would reject such an attempt. This way the sole admin can't accidentally demote themselves and lock themselves out of every admin screen.

## Deactivate an account

**Deactivate** (with confirmation) blocks a user from signing in. The account itself is kept intact: tasks, comments, authorship and mentions all stay in place — the person just can't sign in any more. This is the proper way to offboard someone or cut off a contractor's access — unlike deletion, it breaks nothing in the history.

A deactivated account's row is dimmed, and the button turns into **Activate** — access comes back with the same single click. You can't deactivate your own account.

## Recovery link

The key button to the left of the other actions creates a **password reset link** and copies it to the clipboard. The link is valid for **one hour**.

This helps when a user has forgotten their password and email isn't configured on the instance, or the messages don't reach them: the admin passes the link along by any available channel. If SMTP is configured, an email with the link is additionally sent to the account's address.

The link is, in effect, temporary access to the account, so hand it over personally and don't post it in shared chats.

## What's next

What happens on the instance in the background — synchronizations, notification delivery, recurring tasks — is shown by the [Background jobs](/help/admin-jobs) window.
