---
title: Notifications
category: Working with tasks
order: 70
keywords: notifications, channels, email, telegram, webhook, shoutrrr, routing, rules, quiet hours, digest, deadline, reminders, template, app, push, permissions
updated: 2026-08-25
---

There are two kinds of notifications. **Internal** — the feed under the bell in the top bar: assignments, comments, mentions. They always come. **External** — the same event delivered outward: by email, to Telegram, by a webhook to your own service.

External-delivery settings are shared across all your devices: what you set up in the app also works in the web version, and the other way round.

**Where it's configured.** The side menu (the “☰” button at the left of the top bar) → the **bell** in the row of icons above the project list. Don't confuse it with the bell in the very top bar — that one opens the notification feed, not the settings.

The screen is made of three blocks: **Channels**, **Routing rules**, **Deadlines and reminders**. By default nothing goes out: first you set up a channel, then a rule.

## This device

In the channel list the phone has its own row — a channel of the **System notifications** type with the **this device** label. The app sets it up itself on sign-in; there's no need to add it by hand.

It's what handles push notifications: if a rule routes an event here, it arrives as an Android notification — including when the app is closed. For this to work, the notification permission is needed: Android 13 and newer asks for it on first launch, and denying it turns off exactly this channel. You can grant the permission later in the app's system settings, under “Notifications”.

In a browser the same channel works only while the tab with Tessera is open — background push is available only in the mobile app.

## Channels

**Add channel** expands a form right in the list. The type is chosen once, at creation; you can't change it afterwards — a channel of the type you need is set up anew.

| Type                        | What you fill in                                                             |
| --------------------------- | ---------------------------------------------------------------------------- |
| **Email**                   | the address                                                                  |
| **Telegram**                | the Chat ID and the bot token (the bot is created through `@BotFather`; message it first) |
| **Webhook**                 | the URL, the method (`POST` by default), if needed an `Authorization` header  |
| **Shoutrrr (any service)**  | one Service URL string — `slack://…`, `discord://…`, `ntfy://…` and others    |

The **Enabled** toggle in the form and the toggle in the channel row are one and the same: a switched-off channel stays in the list but receives nothing.

Secrets (the bot token, the Service URL, the authorization header) are stored encrypted and aren't shown back. When you edit a channel, the field is empty: leave it empty and the previous value is kept, enter a new one and it's replaced.

**Test** sends a probe message right away, bypassing the queue, and on success marks the channel as **verified**. The error comes back as text from the service itself — that's the shortest way to understand what's wrong with the token or the address.

## Message template

Every channel (except the system one) has its own text template; an empty one means the default text. The template field is in the channel form, and below it are the field chips: a tap inserts a field at the end of the text.

Available are `{{.Text}}`, `{{.Title}}`, `{{.TaskNumber}}`, `{{.TaskTitle}}`, `{{.Actor}}`, `{{.Workspace}}` and `{{.Link}}`. The **Preview** button shows how it looks on sample data; a broken template shows an error and won't be saved.

## Routing rules

Rules decide which event goes to which channels, and are checked **top to bottom — the first match fires**. As long as there are no rules, external channels stay silent, no matter how many there are.

In a rule: **events** (empty = any), **workspace** (empty = all) and **delivery channels**. The **Mute** toggle makes a rule a silencer — a matching notification goes nowhere and doesn't reach the rules below. An exception rule only makes sense above the general one.

## Deadlines and reminders

The bottom block is about the time-based notifications Tessera sends itself:

- **Notify about deadlines**, **Remind** (how long before the due date) and **Repeat**. The same three settings exist on each task separately — in the task window, by tapping the date.
- **Reminders to external channels** — whether to send a reminder that has fired to the channels. This toggle doesn't affect the app's own local signal: it comes as a device alarm in any case.
- **Digest** — a window of 5–60 minutes: the gathered notifications arrive as a single message instead of a stream. This doesn't affect system notifications.
- **Quiet hours** — a “do not disturb” window that can cross midnight. External notifications are held until it ends, push to the device isn't shown at all during that window, and the feed under the bell fills up as usual. The time zone is taken from the phone.

This block is saved with the **Save** button — channels and rules are saved immediately.

## If notifications don't arrive

1. **The notification permission** for the app — without it there'll be no push.
2. **Is there a rule**: a channel without a rule receives nothing.
3. **A rule higher in the list** isn't intercepting the event (a silencer especially).
4. **The channel and the rule are enabled** — disabled ones are shown dim.
5. **Test** on the channel: it will show the real delivery error.
6. **Quiet hours and digest** — a message may be waiting for the end of the window.
7. **Battery saving.** If the system has put the app to sleep, a notification may come late; on phones with aggressive saving it's worth allowing Tessera to run in the background.

External notifications don't go out instantly: the server processes the queue once every ten seconds, and it retries a failed delivery up to five times with a growing pause.

## What's next

About reminders themselves and their delivery — [Reminders](/help/reminders).
