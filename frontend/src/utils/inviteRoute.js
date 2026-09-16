// Routing an invitation token across the account fork (#2932).
//
// An invitation reaches its recipient once, by mail, and nothing in the app
// hands out a second copy: the token is the whole of the invitee's proof that
// they own the invited address. The accept page, though, sits behind a session,
// so a link opened by someone who is signed out — or signed in as the wrong
// person — bounces through the login and register forms first. Every one of
// those hops used to be a plain `/login` or `/register`, which dropped the token
// on the floor; the invitee arrived on the board with no membership and had to
// go back to their inbox for the link.
//
// These helpers keep the token travelling: `?invite=` is what the register form
// reads, `?next=` is what the login form replays after a successful sign-in, and
// `inviteTokenFromNext` reads one back out of the other so the fork can be
// crossed in either direction.

// invitePath is the accept page's own URL — the shape the invitation mail links
// to, rebuilt here so a redirect can point back at the very same invitation.
export function invitePath(token) {
  const tok = typeof token === 'string' ? token : ''
  return tok ? `/invite?token=${encodeURIComponent(tok)}` : '/invite'
}

// inviteOutcomeRoute points at the accept page to *report* an outcome the
// server already decided, rather than to attempt an acceptance.
//
// Registration is the one fork where the token is spent server-side (it rides
// in the register call), so the accept page never runs for it and the invitee
// would otherwise be dropped on the board with no word about a token that
// granted nothing. `outcome` is what authResponse.invite said; the page reads it
// instead of calling accept, which would answer "already used" about the very
// invitation it just applied.
export function inviteOutcomeRoute(token, outcome) {
  const tok = typeof token === 'string' ? token : ''
  const out = typeof outcome === 'string' ? outcome : ''
  return { path: '/invite', query: { token: tok, outcome: out } }
}

// loginRouteForInvite sends a would-be invitee to the login form so it returns
// them to this invitation afterwards. Without a token it is the plain form.
export function loginRouteForInvite(token) {
  const tok = typeof token === 'string' ? token : ''
  return tok ? { path: '/login', query: { next: invitePath(tok) } } : { path: '/login' }
}

// registerRouteForInvite is the mirror image: the register form presents the
// token on sign-up, which is what joins the new account to the workspace.
export function registerRouteForInvite(token) {
  const tok = typeof token === 'string' ? token : ''
  return tok ? { path: '/register', query: { invite: tok } } : { path: '/register' }
}

// inviteTokenFromNext recovers the token from a `?next=` the login form is
// carrying, so the "no account yet?" link beside it can pass it on.
//
// Only a relative path into the accept page counts. `next` is a query parameter
// anyone can put in a link they hand out, and the value it yields here is fed
// straight back into another route — an absolute URL that merely ends in
// `/invite?token=…` is somebody else's origin and must not be read as ours.
export function inviteTokenFromNext(next) {
  if (typeof next !== 'string' || !next.startsWith('/invite?')) return ''
  const params = new URLSearchParams(next.slice(next.indexOf('?') + 1))
  return params.get('token') || ''
}
