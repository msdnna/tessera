// Reads letters out of the e2e backend's log (#3035). Without SMTP the server's
// no-op mailer logs every letter whole — «mail (noop) to <addr> — <subject>»
// followed by the body — so a spec can pick up the code or the link a person
// would have got by mail. E2E_BACKEND_LOG points at that log; without it the
// specs that need mail skip themselves.
import { existsSync, readFileSync } from 'fs'

export const MAIL_LOG = process.env.E2E_BACKEND_LOG || '/tmp/tessera-e2e-backend.log'

export const mailLogAvailable = () => existsSync(MAIL_LOG)

// The newest letter to an address, as the text from its header line up to the
// next log line (log lines start with a date; body lines don't).
function lastLetter(to) {
  const lines = readFileSync(MAIL_LOG, 'utf-8').split('\n')
  const head = `mail (noop) to ${to.toLowerCase()} `
  for (let i = lines.length - 1; i >= 0; i--) {
    if (!lines[i].toLowerCase().includes(head)) continue
    const body = [lines[i]]
    for (let j = i + 1; j < lines.length && !/^\d{4}\/\d{2}\/\d{2} /.test(lines[j]); j++) {
      body.push(lines[j])
    }
    return body.join('\n')
  }
  return ''
}

async function poll(read, what, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const v = read()
    if (v) return v
    await new Promise((r) => setTimeout(r, 200))
  }
  throw new Error(`no ${what} in ${MAIL_LOG}`)
}

// waitCode waits for a letter to `to` that carries a six-digit code, newer than
// `notCode` (the previous one, when a resend is expected).
export function waitCode(to, notCode = '') {
  return poll(() => {
    const m = /\b(\d{6})\b/.exec(lastLetter(to))
    return m && m[1] !== notCode ? m[1] : ''
  }, `code for ${to}`)
}

// waitToken waits for a letter to `to` with a `token=` link and returns the token.
export function waitToken(to) {
  return poll(() => {
    const m = /token=([0-9a-f]{64})/.exec(lastLetter(to))
    return m ? m[1] : ''
  }, `link for ${to}`)
}
