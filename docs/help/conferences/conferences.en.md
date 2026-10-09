---
title: Conferences
category: Conferences
order: 65
keywords: conferences, meetings, call, meeting, standup, instant meeting, shortcuts, video, audio, microphone, camera, screen share, screen sharing, chat, invite, host, kick, mute, connection quality, minimised, floating window
updated: 2026-10-08
---

The **Conferences** section is audio and video calling inside Tessera: daily stand-ups, walking through a task, demos. A call does not take you out of the tracker — a conference can be pinned to a task, and the discussion stays in the room's chat next to the work rather than in a separate messenger. The section is available both in the browser and in the mobile app (on a phone some panels — chat and participants — open as bottom sheets).

![The Conferences section: a list of scheduled and finished calls](../assets/conferences-light.png)

## The list

**Conferences** sits in the sidebar. At the top of the list is the call that is **on right now**: a card of its own, marked "live · N min", with a **Join** button. Below are the scheduled ones, by day: **Today**, **Tomorrow** and on; the nearest meeting has its time highlighted and a **Join** button. Meetings without a time, those planned for a day that has passed but never started, and finished ones come last, each in a group of its own. The segments above the list are **All · Live · Scheduled · Finished**.

At the right of a row are a link button (copy the meeting's address to drop it into a chat) and a **⋯** menu. Deleting is the menu's last item and takes two presses: the first arms it, the second deletes; it cannot be undone, and the meeting's chat and recordings go with it.

**Schedule** (keys <kbd>N</kbd> then <kbd>V</kbd>) opens a form next to the button: a title, a start time, a description and how long recordings are kept. The start time is a hint for the participants, not a lock: a conference can be opened earlier. **Instant** (<kbd>N</kbd> then <kbd>J</kbd>) creates a meeting with no time and takes you straight into it — for when you need to talk right now. On this page <kbd>⇧</kbd> <kbd>J</kbd> takes you into the call that is on, or, if there is none, into the nearest scheduled one.

A conference is **not single-use**. When the last participant leaves it goes back to _Scheduled_ and can be entered again — one room is a convenient home for a standing daily meeting. Only deletion is irreversible.

## In the room

![A conference before joining: the meeting card with Join](../assets/conference-room-light.png)

An opened conference first shows the **lobby** — the meeting's card: title, time, who is invited, the description. **Join** (<kbd>⇧</kbd> <kbd>J</kbd>) turns your devices on and puts you in the call. Meanwhile the app's header holds an arrow back to the list, the meeting's name and, while the call is on, how long it has been running; the host has **End** on the right.

The browser will ask for permission to use the microphone and camera — without it the call cannot happen. The permission is granted to the site once; if you refused it by accident, restore it in the site settings in the browser's address bar.

The call takes the whole working area. The layout switch is above the tiles: **grid** — everyone as equal tiles, or **speaker** — a large tile follows whoever is speaking, the rest in a strip under it; the choice is remembered. During a screen share the stage always shows the share. In silence the stage stays on whoever spoke last — the tiles do not jump at every breath. A tile without video is tinted in the person's own colour, the speaker gets a glowing ring, and a raised hand is a yellow badge in the corner.

**The controls at the bottom:**

- **microphone** (<kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>D</kbd>) — on and off. The button fills with the level of your voice: a self-check of your volume, so you can see you are being heard before anyone has to say so;
- **camera** (<kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>E</kbd>) — video on and off;
- **screen sharing** — see below;
- **recording** — for the host and moderators, see "Recording";
- **raise hand** — a visible mark on your tile, so you do not have to talk over the speaker;
- **devices** — which microphone, which camera, which speaker;
- **participants** and **chat** — open the right panel on that tab (pressing again hides it);
- **Leave** (<kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>⇧</kbd> + <kbd>H</kbd>).

Reactions are already drawn in the design but do not work yet — the button is visible, inactive and labelled "Soon". **Full screen** is a button in the stage's corner; it is most useful when watching someone else's screen.

A **connection badge** appears on a participant's tile when their link is poor: before asking "you're breaking up?", you can see whose line it is.

## Screen sharing

Screen sharing runs **as a queue**, and the queue is kept by the server rather than by a browser: two people cannot start sharing at once and cut each other off. If a screen is already being shown, the button puts you in the queue and shows your place in it; when your turn comes, the room tells you that you can start.

The person sharing always sees a sharper picture than everybody else, and that is not a fault: they are looking at their **own raw capture**, while the others get an encoded stream that went through the server. The share is published at high resolution with sharpness prioritised, so under load the viewers lose smoothness rather than the legibility of the text.

## People, chat and invitations

On the right is a panel with two tabs: **In the room** and **Chat**. A button above the tiles hides it — the stage takes the whole width; the choice is remembered.

**In the room** lists who is in the call now, and a separate **Invited** list shows people who were asked but have not arrived yet. **Invite** (the person with a plus) is right there by the tabs: the invitee gets a notification with a link straight into the conference.

Each participant's volume is set **on your side** and only there — it does not change how anyone else hears them.

**Chat** is the conference's own conversation: links shared on the go, and attachments. A picture opens large **without leaving the call**. The chat history stays with the conference, so you can come back to what was said.

## Host and moderation

The host is the **creator of the conference**. They cannot be removed from the call, and they have priority in the screen-sharing queue.

Moderation — **removing** a participant or **muting them for everyone** — is available to the host, and also to the owner and admins of the workspace. An admin is still shown as an ordinary participant: they moderate without taking the host role, and can be removed themselves.

Every permission check happens **on the server**, which is what makes "mute for everyone" actual enforcement rather than a polite request to somebody else's browser.

## Minimised mode

You can go off to tasks or documents **without leaving the call**: the Minimise button above the tiles or <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>⇧</kbd> + <kbd>M</kbd> takes you back to the screen you came from, and the call carries on in a floating window.

The window has three parts. The head — the meeting's name and how long you have been in the call — is what you drag it by; its button (or the same <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>⇧</kbd> + <kbd>M</kbd>) brings the call back full size. In the middle are two tiles: the speaker (or the screen share) and the next participant. At the foot: microphone, camera, hand, how many more people are in the call, and **Leave**. Dropped near an edge of the screen, the window sticks to it; it resizes from any edge. Its position and size are remembered. The microphone, camera and leave keys work while minimised too.

A minimised call **survives a page reload**: the active conference is remembered, and after a refresh the client walks back into it on its own. The connection itself is rebuilt — a second of silence is normal here.

You only leave a call with **Leave** (in the room or in the mini window) or by closing the tab. Switching to another section does not drop the call — and it does not mislead the others either: if the tab closes, you disappear from the list on your own.

## Recording

A call can be recorded whole — a single video file with every participant and the screen share in it. The **server** does the recording, not your browser: the quality does not depend on your uplink, and you can close the tab.

Recording is started and stopped by the **host or a moderator**, with a button on the panel at the bottom of the room. While it runs, **every participant sees a red label with the recording's time** — in the room and in the minimised window: a call cannot be recorded unnoticed, and that is deliberate.

Finished recordings sit in a list **under the meeting's card**: download, size and duration. A file appears there a few seconds after you stop — that is how long muxing the video takes.

**Retention is chosen up front**, when the conference is scheduled: by default a recording lives for 30 days and is then deleted by itself. It is decided before the meeting rather than after, so nobody has to rule on a file's fate while watching a countdown. Zero means "keep indefinitely". The setting can be changed later in the conference settings, but it only applies to future recordings: a finished recording gets its deletion date stamped when it completes.

Recording is an **optional instance feature**: the administrator enables it separately. If it is not enabled, the button answers with an error — a question for your administrator, not a fault on your side.

## Worth knowing

- Conferences need a **secure connection** (`https://`). Over a plain address the browser will not hand over the microphone and camera at all, and the room says so plainly instead of quietly failing.
- If your administrator has not configured the media server, joining a call returns an error — that is a question for the instance administrator, not a fault on your side.
- Recording is **not a substitute for minutes**: there is no transcript or auto-summary yet, so decisions made along the way are still best dropped into the conference chat or into the description of the linked task.
