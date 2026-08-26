---
title: Signing in with GitLab (OAuth)
category: Administration
order: 82
keywords: administration, gitlab, oauth, sign-in, authorization, service token, sudo, org map, integration
updated: 2026-08-25
platforms: web
---

The **“Sign in with GitLab (OAuth)”** card is the top block of the [Administration](/help/admin-users) screen. It configures two things at once, and their sitting side by side is no accident:

- **signing in to Tessera through GitLab** — a “Sign in with GitLab” button on the sign-in screen instead of the email-plus-password pair;
- **the sync service token** — the credentials the GitLab task integration runs under.

Both parts are filled in once for the whole instance and are available only to the instance administrator.

![The “Sign in with GitLab (OAuth)” card on the administration screen](../assets/admin-oauth-light.png)

## The OAuth application on the GitLab side

First the application is registered in GitLab itself — in the admin area (**Admin → Applications**), in a group's settings or in your personal profile, depending on whose application you want it to be.

Two values are needed:

- **Scopes** — `read_api` is enough. It covers both the user's profile and the list of their groups (the Org map below relies on it). Broader permissions are not needed here: Tessera writes nothing under this token.
- **Redirect URI** — the exact address GitLab returns the user to. It is shown right in the card, in the hint line, and looks like `https://your-address/api/auth/gitlab/callback`. Copy it from there rather than assembling it by hand: GitLab compares the address character by character and refuses the sign-in on any mismatch.

Tessera derives the callback address from the `PUBLIC_URL` environment variable, and if it is not set — from the request headers. In production, set `PUBLIC_URL` explicitly: behind a reverse proxy the headers may not carry the port or scheme, and then the card will show — and Tessera will send to GitLab — an address GitLab does not recognize.

GitLab issues an **Application ID** and a **Secret** — these are what you enter into the card.

## Card fields

**GitLab URL** — the address of the GitLab instance, for example `https://gitlab.example.com`. The same address is used for synchronization.

**Application ID** and **Secret** — from the GitLab application.

**Sync service token** — a personal access token (PAT) of a GitLab service account with the `api` scope. All task synchronization runs under it, so users do not need to connect personal tokens. Issues then map onto Tessera accounts not by the token owner but by OAuth identity: whoever signed in through GitLab is the one recognized as the assignee.

**Write as the acting user (Sudo)** — a toggle after which creating an issue and write-back go out as the acting user rather than the service account. For this the service token has to be an admin PAT with the `api` **and** `sudo` scopes. Such a token can act on behalf of any GitLab user, so **keep the toggle off if GitLab or Tessera is reachable from an external network**: the cost of leaking such a token is out of all proportion to the convenience of correct authorship in the issue.

**Sign-in enabled** — shows the “Sign in with GitLab” button on the sign-in screen. The button appears only when the toggle is on, the Application ID is filled in and the GitLab URL is set all at once; with the pair unfilled it will not appear even with the toggle on.

**Org map** — the rules by which membership in GitLab groups turns into access to Tessera workspaces (detailed below).

## How secrets are stored

The Secret and the service token come back from the server not as a value but as a “stored” marker: the field shows `•••••• (stored; type to replace)`. An empty field means “leave as is” — saving the card does not wipe a secret just because you did not re-enter it.

To **erase** a stored value, click the eraser button on the right side of the field: it locks the field and changes the hint to “will be cleared on save”. To undo, use the neighboring arrow button, or simply start typing a new value: a replacement cancels the clearing. The actual erasure happens when you click **Save**.

Next to the service token field a warning appears — **“GitLab synchronization will stop”** — a removed token stops the sync for every binding on the instance, not just one.

## What happens on the first sign-in through GitLab

The user clicks the button on the sign-in screen, confirms access in GitLab and comes back. Tessera then looks for which account to link them to:

1. **A known GitLab identity** — sign in to the account already linked to this GitLab user.
2. **An email match** — if the address from GitLab matches that of an existing Tessera account, the accounts are linked: GitLab vouches for the address.
3. **Otherwise — registration**: an account is created without a password, with an already-verified email and a personal workspace, and any invitations sent to that address in advance are accepted.

If GitLab hides the user's email, Tessera substitutes a service address of the form `login@gitlab.local` — it can be changed in the profile later.

A **deactivated account** will not sign in through GitLab: the sign-in fails with an error, deactivation is not bypassed by switching the sign-in method. The reverse is also true — an account created through GitLab is deactivated on the administration screen like any other.

## Org map: GitLab groups → workspaces

The **Org map** field takes JSON where the key is the full path of a GitLab group and the value describes what membership in it grants:

```json
{
  "demo-group": {
    "workspace_id": "0cba7226-4501-4b06-a941-13d25defd8fd",
    "admins": ["p.dorohov"],
    "users": true
  }
}
```

- `workspace_id` — the Tessera workspace that access is granted to;
- `admins` — the GitLab logins that get the administrator role in this workspace;
- `users` — whether to grant the member role to everyone else in the group (`false` — only those listed in `admins` get access).

The rules are applied **on every sign-in** through GitLab, not once at registration: it is enough to add a person to the GitLab group, and after their next sign-in they will be in the workspace. Granting only adds rights and never demotes the workspace owner — revoking access remains a manual operation in the workspace members.

The key is compared against the group's **full path** as it appears in the GitLab address (`department/subgroup`, not just the last segment). If the path matches nothing, the rule simply does not fire — silently, with no error. An empty `{}` disables parsing entirely.

The card will not let you save invalid JSON: the message “org_map: invalid JSON” appears, and the card's other fields do not go to the server either — fix the JSON and save again.

## Verification

After saving:

1. Open the sign-in screen in a private window — the “Sign in with GitLab” button should appear.
2. Sign in with a test GitLab account and make sure the user ends up in the right workspaces.
3. If the sign-in fails, the message on the sign-in screen names the reason: “Sign-in with GitLab is not configured” — the Application ID/URL pair is unfilled or the toggle is off; “Could not authorize with GitLab” — the Secret or the Redirect URI did not match; “Could not fetch the GitLab profile” — the application lacks the `read_api` scope.

## What's next

The credentials are set — you can move on to [linking GitLab projects to boards](/help/admin-gitlab-project).
