---
title: Notifications
category: Notifications
order: 70
keywords: notifications, channels, email, telegram, webhook, shoutrrr, routing, rules, quiet hours, digest, deadline, reminders, template
updated: 2026-08-25
---

Notifications in Tessera come in two kinds, and only the second is configurable.

**Internal** — the bell in the bottom bar: you were assigned a task, someone replied to your comment, you were mentioned. They always come, they can't be turned off, and the counter on the bell shows the unread ones.

**External** — the same event delivered outward: by email, to Telegram, by a webhook to your own service. By default nothing goes out: first you set up a **channel** (where to deliver), then a **rule** (what exactly to send to it).

All of this lives in **Settings** (the gear in the bottom bar) under **Notifications**.

![The Notifications section: the list of delivery channels](../assets/notifications-channels-light.png)

## Delivery channels

A channel is a single recipient address. There can be as many as you like, and each is switched on and off with its own toggle; a switched-off one stays in the list but receives nothing.

**Add channel** opens a form. The type is chosen once, at creation — you can't change it afterwards; a channel of the type you need is set up anew.

| Type                         | What you fill in                                                            |
| ---------------------------- | --------------------------------------------------------------------------- |
| **Email**                    | the address (the account address is filled in by default)                   |
| **Telegram**                 | the Chat ID and the bot token                                               |
| **Webhook**                  | the URL, the method (`POST` by default) and, if needed, an `Authorization` header |
| **Shoutrrr (any service)**   | one Service URL string — `slack://…`, `discord://…`, `ntfy://…` and a dozen more |
| **System notifications**     | nothing: the channel is set up by the client itself                          |

**Telegram.** The bot is created through `@BotFather`, which also hands out the token. The Chat ID is your own identifier in Telegram (or `@channel_name` if the bot is an admin there); you need to message the bot once yourself, otherwise it isn't allowed to write first.

**Webhook** is the fallback for everything else: a small JSON with the fields `kind`, `title`, `text` and `link` goes to the address you specify, and the receiving side then does with it whatever it wants. The address has to be external: by default the server refuses to reach local and internal addresses, so a channel can't be pointed inside someone else's network.

**Shoutrrr** is the same idea, but without your own handler: a single URL describes both the service and the credentials, and Slack, Discord, ntfy, Gotify, Matrix, Pushover, Teams and others are supported. The URL format for each service is in shoutrrr's own documentation.

**System notifications** are the device or browser you signed in from. Such a channel appears in the list on its own and is marked **this device** if it's opened from that very device; you can't add it by hand, but you can rename, disable and delete it. In a browser it shows the operating system's pop-up notification while the tab with Tessera is open, and it requires permission — if it hasn't been granted yet, an **Allow** button appears under the channel name. The mobile app with this channel gets notifications even when closed.

### Secrets, testing and deletion

The bot token, the Service URL and the `Authorization` header are stored encrypted and aren't shown back. When you edit a channel, the secret field is empty: leave it empty and the previous value is kept, enter a new one and it's replaced. You can wipe the secret entirely with the eraser next to the field.

**Test** sends a probe message to the channel right now, bypassing the queue. Success marks the channel with a **verified** label, and it's the best way to make sure the token is right — an error comes back as text from the service itself. For email you may get the reply “SMTP isn't configured on the server”: the letter is then only written to the server log, and that's a question for the instance administrator, not for the channel's settings.

When you delete a channel, Tessera warns you if it's used in rules: the rules themselves stay, but they stop delivering anywhere.

## Message template

Every channel (except the system one) has its own text template. An empty template means the default text: the gist of the notification plus a link to the app.

The **Edit** button opens the editor: on the left is the template field with a live preview on sample data, on the right is the list of fields, each inserted with a click at the cursor position.

| Field            | What it substitutes                    |
| ---------------- | -------------------------------------- |
| `{{.Text}}`      | the ready notification text            |
| `{{.Title}}`     | the title by event type                |
| `{{.Kind}}`      | the event type (`assigned`, `comment`, …) |
| `{{.TaskNumber}}` | the task number                       |
| `{{.TaskTitle}}` | the task title                         |
| `{{.Actor}}`     | who triggered it                       |
| `{{.Workspace}}` | the workspace name                     |
| `{{.Link}}`      | the link to the app                    |

The syntax is Go templates, and conditions are available too: `{{if .TaskNumber}}#{{.TaskNumber}} {{end}}{{.Text}}`. A broken template won't be saved: the error appears in the preview right away, and on an attempt to save — under the channel form.

## Routing rules

Rules decide which event goes to which channels. They are checked **top to bottom, and the first match fires** — the rest are no longer considered. As long as there are no rules, external channels stay silent, no matter how many are set up.

![Routing rules: the list of rules and the form for a new rule](../assets/notifications-routes-light.png)

A rule has three settings:

- **Events** — which notification types it catches. Empty = any.
- **Workspace** — limit it to one workspace. Empty = all.
- **Delivery channels** — where to send; you can pick several.

A separate **Mute** toggle turns a rule into a silencer: a matching notification goes nowhere and doesn't reach the rules below. It's a way to cut out a noisy event type without touching the rest — for example, put “Integration sync → mute” as the first rule and a general “any events → email” below it.

There are nine event types: task assignment, comments, mentions, task update, task move, archiving, deadline approaching, reminders, integration sync.

The order of rules is set by their order in the list — an exception rule must sit **above** the general one, otherwise the general one fires first and the exception never runs.

## Deadlines and reminders

The bottom block of the section is about the notifications Tessera sends itself, by time.

**Notify about an approaching deadline** and two choices under it: **Remind** — how long before the due date (from “at the deadline” to “2 days before”) and **Repeat** — whether to repeat afterwards (once, every hour, every 3 or 6 hours, once a day). The due-date notification goes to the task's assignees and its author.

The same three settings exist on each task separately — by clicking the date on the card, in the “Notifications” block. The value “Default” means “as in the settings”, and any other value overrides the general rule for that task. If the due date is moved, the notification is re-armed.

**Deliver reminders to external channels** is about the “Reminders” section: when a reminder fires, whether to send it to the channels. The switch acts only on future ones: turning it on won't bring you old reminders.

**Group into a digest** — a window of 5, 15, 30 or 60 minutes. The notifications gathered over the window arrive as a single message per channel (“Digest — N notifications”) instead of a stream of separate ones. This doesn't affect system notifications: a push that arrives half an hour late is useless.

**Quiet hours** — a “do not disturb” window that can cross midnight (for example, 22:00–08:00). External notifications in that window are **held** and arrive all at once after it ends; system notifications to the device simply aren't shown during quiet hours — a pop-up can't be postponed. Internal notifications on the bell come as usual. The time is counted in your time zone — it's taken from the localization settings.

Changes in this block are applied with the **Save** button — unlike channels and rules, which are saved immediately.

## How delivery really works

External notifications aren't sent at the moment of the event: they go into a queue that the server processes once every ten seconds. That's why the normal delay for an email or a Telegram message is up to a few dozen seconds, not an instant.

If delivery fails, Tessera retries it up to five times with a growing pause (roughly after 1, 4, 9 and 16 minutes). Errors that won't clear on their own — a wrong token, a non-existent address, a rejected request — get no retries: those are fixed only by editing the channel.

## If notifications don't arrive

1. **Is there a rule.** Channels without rules receive nothing — that's the most common cause.
2. **A rule higher in the list.** The first matching rule takes the event for itself; check that a silencer rule isn't intercepting it.
3. **The channel and the rule are enabled.** A disabled channel is dim in the list; a disabled rule is too.
4. **The channel is verified.** Press **Test**: it will show the real service error.
5. **Quiet hours.** If it's a quiet window right now, external notifications will come after it ends, and system ones won't come at all.
6. **Digest.** With the digest on, a message waits for the end of the window and arrives together with the rest.
7. **Browser permission** — for system notifications; the **Allow** button under the channel appears if there's none.
8. **Email doesn't go out at all** — SMTP may not be configured on the server: the channel test will tell you.

## What's next

- How due dates and reminders are built on their own — [Reminders](/help/reminders).
- What happens to the delivery queue on the server side — [Background jobs](/help/admin-jobs) (for the instance administrator).
