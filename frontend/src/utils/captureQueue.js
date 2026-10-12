// The offline capture queue of «Входящие» (#3042, minimal scope of #3031): a note
// or reminder captured without a connection waits on the device and goes out
// when the connection is back. Only creating is queued — editing stays online.
// Every record carries the id the client gave it (`client_id`), so a resend
// after a lost answer finds the first one on the server instead of a twin.
//
// Stored per server (utils/serverStorage), oldest first.
import { sGet, sSet } from './serverStorage'

const KEY = 'tessera_capture_queue'

export function newClientId() {
  return globalThis.crypto?.randomUUID?.() || fallbackUUID()
}
function fallbackUUID() {
  const h = [...Array(32)].map(() => Math.floor(Math.random() * 16).toString(16))
  h[12] = '4'
  h[16] = ((parseInt(h[16], 16) & 3) | 8).toString(16)
  const s = h.join('')
  return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20)}`
}

export function queued() {
  try {
    const v = JSON.parse(sGet(KEY) || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}
function save(list) {
  try {
    sSet(KEY, JSON.stringify(list))
  } catch {
    /* quota / private mode — the capture is lost with the tab, as before */
  }
}

// enqueue({ kind: 'note'|'reminder', wsId, data }) — data carries client_id.
export function enqueue(item) {
  const list = queued().filter((x) => x.data?.client_id !== item.data?.client_id)
  list.push({ ...item, at: new Date().toISOString() })
  save(list)
  return list.length
}

// flush sends the queue in order through `send(item)`. It stops at the first
// item that fails for want of a connection (the rest wait for the next try);
// an item the server refuses (4xx — a deleted task, a lost membership) is
// dropped and reported, or it would block the queue forever.
export async function flush(send) {
  const sent = []
  const dropped = []
  for (const item of queued()) {
    try {
      sent.push({ item, res: await send(item) })
    } catch (e) {
      if (e?.offline || !e?.status || e.status >= 500) break
      dropped.push({ item, error: e })
    }
    const done = new Set([...sent, ...dropped].map((x) => x.item.data.client_id))
    save(queued().filter((x) => !done.has(x.data.client_id)))
  }
  return { sent, dropped, left: queued().length }
}
