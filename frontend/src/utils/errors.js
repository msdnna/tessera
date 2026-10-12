// Turn backend / network error strings into friendly, localized text for display.
// The backend surfaces two ugly shapes we must tame before showing a user:
//   • gin/validator binding errors, e.g.
//     "Key: 'Email' Error:Field validation for 'Email' failed on the 'email' tag"
//   • bare English sentinels, e.g. "invalid credentials".
// Anything we recognise is mapped; short unknown messages pass through; clearly
// internal noise falls back to a generic line. Applied centrally in the axios
// response interceptor so every `message.error(e.message)` caller benefits.
//
// This runs outside a setup context (the interceptor is plain module code), so
// the catalog is reached through `i18n.global.t` rather than useI18n(). The
// lookup happens per call, not at import time — an error raised after the user
// switched language must be phrased in the new one (#2799).
import { i18n } from '@/i18n'
import { formatFileSize } from '@/utils/docPdf'

// The one refusal that carries a number, so it can't be a table entry: the
// document validator names its own ceiling in bytes ("content exceeds 4194304
// bytes"). It surfaces in the editor's autosave line, where a raw English
// sentence with a seven-digit byte count is the least useful phrasing possible —
// the reader wants "4 МБ" and the fact that the page has to be split (#2932).
const CONTENT_TOO_LARGE = /^content exceeds (\d+) bytes$/

// The second one, same shape and same reason: the in-call chat refuses a batch
// of files whose *total* is over MaxChatTotalBytes, and names the ceiling in
// bytes so every client can put it in the reader's own units. The composer
// mirrors the rule (utils/confChat.js), so this is what a stale tab or a second
// picker hears (#2932).
const ATTACHMENTS_TOO_LARGE = /^attachments exceed (\d+) bytes$/

// The notification-channel editor's refusals (#2932). Unlike most of the table
// below these are not stale-tab rarities — no client mirrors the channel rules,
// so an ordinary user meets them while setting a channel up. Each carries a
// variable part (the type, the field it wants, the ceiling), which is why they
// are patterns rather than table entries.
const CHANNEL_TYPE_UNKNOWN = /^unknown channel type: "(.*)"$/
// The type is spelled out rather than matched as a word: the channel types are a
// closed set on the wire, and a loose `(\w+) channel requires …` would rewrite an
// ordinary English sentence that happens to contain those three words.
const CHANNEL_REQUIRES_FIELD =
  /^(email|telegram|webhook|shoutrrr|device) channel requires ([a-z_]+)$/
const TEMPLATE_TOO_LONG = /^template exceeds (\d+) characters$/
// Only the wrapper is ours: the tail is the Go template parser naming the
// offending action, which has no translation and is carried verbatim — taken
// from the original string, not the lowercased copy used for matching.
const TEMPLATE_ERROR_PREFIX = 'template error: '

// The keys are the wire format — exactly what the backend sends — so they stay
// English; only the catalog key on the right-hand side is ours to translate.
const SENTINELS = {
  'invalid credentials': 'invalidCredentials',
  'invalid email or password': 'invalidCredentials',
  'email already registered': 'emailRegistered',
  'email already exists': 'emailRegistered',
  'email already in use': 'emailInUse',
  'user already exists': 'userExists',
  'user not found': 'userNotFound',
  'invalid or expired token': 'linkInvalid',
  'invalid token': 'linkInvalid',
  'token expired': 'linkExpired',
  'account is deactivated': 'accountDeactivated',
  'account deactivated': 'accountDeactivated',
  'job is already running': 'jobAlreadyRunning',
  // A typo in the current-password field (#2932) — the one refusal on this list
  // an ordinary user reaches by ordinary means. SettingsView prints it inline
  // under the form rather than as a toast, so the bare English line stayed on
  // screen until the next attempt.
  'current password is incorrect': 'currentPasswordWrong',
  // Adding a member by an address no account uses. The membership dialog also
  // *branches* on this one (unknown address → offer an invitation instead), and
  // it compares against `err.raw` rather than this sentence — see api/index.js.
  'no user with that email': 'noUserWithEmail',
  // The two ends of the password rule (#2932). Reached only when a client's own
  // check was bypassed — the forms mirror the rule (utils/password.js) so the
  // user hears about a too-long passphrase while typing it.
  'password is too short': 'passwordTooShort',
  'password is too long': 'passwordTooLong',
  // Same shape for the approval route's cap (#2932): the panel mirrors
  // maxDocApprovalSteps (utils/docApprovals.js) and greys the remaining names
  // out, so this is only reached when that check was bypassed.
  'too many approvers': 'tooManyApprovers',
  // The two ways accepting an invitation is refused (#2932). The accept page
  // phrases both itself — it also *branches* on the wrong-address one, against
  // `err.raw` — but the same call is made from the register form, where the
  // refusal surfaces as the form's error line.
  'invalid or expired invitation': 'invitationInvalid',
  'this invitation is for a different email': 'invitationWrongAccount',
  // The confirmation code (#3057) and the address change (#3058). The code
  // screen phrases the three code refusals itself (with the attempts left) and
  // branches on `err.raw`; these are for anywhere else they surface.
  'invalid code': 'codeInvalid',
  'too many attempts': 'codeTooManyAttempts',
  'code expired': 'codeExpired',
  'this is already your email': 'emailSame',
  // The workspace membership guards (#2932). The dialog mirrors all four
  // (utils/members.js) and greys the row out, so these are reached from a stale
  // tab — another admin changed something while this one was open — which is
  // exactly when a bare English line is least welcome.
  "cannot change the workspace owner's role": 'cannotChangeOwnerRole',
  'cannot remove the workspace owner': 'cannotRemoveOwner',
  'cannot change your own role': 'cannotChangeOwnRole',
  'cannot remove yourself from the workspace': 'cannotRemoveSelf',
  // Members v2 (#3063). The window offers neither door to someone it would
  // refuse, so these, too, are stale-tab answers — or a race between two admins.
  'only the owner can transfer the workspace': 'transferOwnerOnly',
  'you already own the workspace': 'transferToSelf',
  'the new owner must be a member of the workspace': 'transferNotMember',
  'the owner cannot leave the workspace — transfer it first': 'ownerCannotLeave',
  'already a member of this workspace': 'alreadyMember',
  'access request declined recently': 'accessDeclinedRecently',
  'the request is already decided': 'accessAlreadyDecided',
  'a group with this name already exists': 'memberGroupNameTaken',
  'every group member must be a member of the workspace': 'memberGroupOutsider',
  // What a manager hears once they are not one any more. Distinct from the bare
  // `forbidden` above: that one is the generic gate, this one names the rank
  // required, and it is the sentence a demoted admin meets on every subsequent
  // click of the dialog they were just using.
  'requires admin or owner': 'requiresManager',
  'requires admin': 'requiresAdmin',
  // The two bounds a saved board view now has (#2932). Unlike most of this list
  // these are not stale-tab rarities: no client mirrors either rule, so the
  // toolbar's «Сохранить вид» is where an ordinary user meets them — the count
  // once their picker is full, the size if a future config grows past the
  // ceiling. Both are plain sentences rather than patterns: the ceilings are a
  // decision about a dropdown, not a number the reader can act on.
  'view limit reached': 'boardViewLimitReached',
  'view config is too large': 'boardViewConfigTooLarge',
  // Views v2 (#2970): a rename or a «сделать общим» into a list that already
  // has the name; a shared view edited by somebody who is not a manager.
  'view name taken': 'boardViewNameTaken',
  'not your view': 'boardViewNotYours',
  // The ceiling on a document's task links, for the same reason as the two above:
  // no client mirrors it, so the links panel is where it is met. Its sibling
  // refusal ("block id is too long") is deliberately absent — the anchor is drawn
  // by the editor, never typed, so a client that trips it is broken rather than
  // full, and the generic fallback says as much.
  'link limit reached': 'docLinkLimitReached',
  // The rate limiter's refusal (middleware.RateLimit), and the one sentinel on
  // this list an ordinary user meets without doing anything wrong (#2932). The
  // login route is throttled at ten attempts a minute keyed by IP *and* by
  // email, so a whole office behind one address shares one budget: mistype a
  // password on a busy morning and the form answered with a bare English
  // "too many requests", which does not even say the thing that matters — that
  // waiting fixes it.
  'too many requests': 'tooManyRequests',
  // The transport's own refusal, from middleware.BodyLimit — not a handler's
  // sentence at all, which is why it reads like a protocol error rather than
  // something a person did. An ordinary user reaches it by writing a very long
  // task description (#2932).
  'request body too large': 'bodyTooLarge',
  // Not an error but a warning, and the one message on this list that rides a
  // 200: the channel probe answers {ok:true, warning} when the server has no
  // SMTP, and both clients print that string as-is (#2932).
  'smtp is not configured': 'smtpNotConfigured',
  // The media server behind conferences and its recorder are optional parts of
  // an instance; their 503s used to reach the call screen in English (#3107).
  'conferences are not configured on this server': 'conferencesNotConfigured',
  'recording is not configured on this server': 'recordingNotConfigured',
  forbidden: 'forbidden',
  unauthorized: 'unauthorized',
  'not found': 'notFound',
}

function t(key, params) {
  return i18n.global.t(`errors.${key}`, params)
}

// Collapse a gin/validator error ("...failed on the 'X' tag") into one friendly
// sentence based on which validation tags failed.
function fromValidator(raw) {
  const tags = [...raw.matchAll(/failed on the '(\w+)' tag/g)].map((m) => m[1])
  if (!tags.length) return null
  if (tags.includes('email')) return t('validation.email')
  if (tags.every((tag) => tag === 'required')) return t('validation.required')
  if (tags.includes('min')) return t('validation.tooShort')
  if (tags.includes('max')) return t('validation.tooLong')
  return t('validation.generic')
}

export function humanizeError(raw) {
  if (!raw) return t('generic')
  const msg = String(raw).trim()
  const low = msg.toLowerCase()
  // hasOwn, not a bare lookup: `low` comes from the server, and "constructor"
  // would otherwise hit Object.prototype and be mapped as if it were a sentinel.
  if (Object.hasOwn(SENTINELS, low)) return t(`sentinel.${SENTINELS[low]}`)
  const oversized = CONTENT_TOO_LARGE.exec(low)
  if (oversized) {
    return t('sentinel.docContentTooLarge', { limit: formatFileSize(Number(oversized[1])) })
  }
  const oversizedBatch = ATTACHMENTS_TOO_LARGE.exec(low)
  if (oversizedBatch) {
    return t('sentinel.chatAttachmentsTooLarge', {
      limit: formatFileSize(Number(oversizedBatch[1])),
    })
  }
  const unknownChannel = CHANNEL_TYPE_UNKNOWN.exec(low)
  if (unknownChannel) return t('sentinel.channelTypeUnknown', { type: unknownChannel[1] })
  const channelNeeds = CHANNEL_REQUIRES_FIELD.exec(low)
  if (channelNeeds) {
    return t('sentinel.channelRequiresField', { type: channelNeeds[1], field: channelNeeds[2] })
  }
  const longTemplate = TEMPLATE_TOO_LONG.exec(low)
  if (longTemplate) return t('sentinel.templateTooLong', { limit: Number(longTemplate[1]) })
  if (low.startsWith(TEMPLATE_ERROR_PREFIX)) {
    return t('sentinel.templateError', { detail: msg.slice(TEMPLATE_ERROR_PREFIX.length) })
  }
  if (low.startsWith('key:') && low.includes('validation')) {
    return fromValidator(msg) || t('validation.generic')
  }
  if (
    low.includes('network error') ||
    low.includes('timeout') ||
    low.includes('econnrefused') ||
    low.includes('failed to fetch')
  ) {
    return t('network')
  }
  return msg
}
