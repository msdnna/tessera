// Carrying an invitation token across the OAuth round trip (#2932).
//
// The accept page sends every signed-out visitor to the register form, and that
// form offers "sign up with GitLab" — a full top-level navigation to the
// provider and back through /oauth/callback. Nothing survives it: `?invite=` and
// `?next=` are query parameters of a page that is being left, and the callback
// lands on the board. An invitee who takes the SSO door therefore arrives with a
// personal workspace, no membership and no hint that anything was missed, while
// the only copy of the token sits in their mailbox.
//
// `?next=`-style routing cannot cross that gap — the return trip is the
// provider's redirect, not ours — so the token waits here instead, in
// sessionStorage: the same tab that started the login is the one that comes
// back, and a tab that is closed forgets the token rather than leaving it on
// disk for the next person at this machine.

const KEY = 'tessera_pending_invite'

// stashPendingInvite parks a token for the OAuth round trip. An empty or
// non-string token parks nothing, so a plain sign-in never redirects anywhere.
export function stashPendingInvite(token) {
  const tok = typeof token === 'string' ? token : ''
  try {
    if (tok) sessionStorage.setItem(KEY, tok)
    else sessionStorage.removeItem(KEY)
  } catch {
    // Storage can be denied outright (private mode, a locked-down webview). The
    // invitation is then left for the mail link to retry — losing it must not
    // take the sign-in down with it.
  }
}

// takePendingInvite reads the parked token and clears it in the same breath.
//
// One-shot on purpose, and taken on every return from the provider, failures
// included: a token left behind would hijack the *next* sign-in in this tab and
// send it to an invitation already spent.
export function takePendingInvite() {
  try {
    const tok = sessionStorage.getItem(KEY) || ''
    sessionStorage.removeItem(KEY)
    return tok
  } catch {
    return ''
  }
}
