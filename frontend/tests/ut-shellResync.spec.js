import { describe, it, expect, vi } from 'vitest'
import { resyncShell } from '@/utils/shellResync'

// What the shell re-reads after a gap in the realtime stream (#2932).
//
// The layout's resync used to rebuild the sidebar and nothing else. The bell is
// fed by socket events alone — `notes.load()` runs once in onMounted — so every
// notification delivered while the socket was down (laptop asleep, backend
// restarted, link flapping) was lost until a page reload, with the unread count
// still reading as whatever it was before the outage. Same for the GitLab
// conflicts badge, refreshed only on mount and on a workspace change.

// A workspaces store stub: resync() optionally lands on a different workspace,
// the way it does when the server cut our socket because our membership moved.
function stores({ landsOn = 'w1' } = {}) {
  const calls = []
  const workspaces = {
    currentId: 'w1',
    resync: vi.fn(async () => {
      calls.push('workspaces')
      workspaces.currentId = landsOn
    }),
  }
  const notifications = { load: vi.fn(async () => calls.push('notifications')) }
  const conflicts = { load: vi.fn(async () => calls.push('conflicts')) }
  return { workspaces, notifications, conflicts, calls }
}

describe('resyncShell', () => {
  it('re-reads the notification feed, so the bell survives an outage', async () => {
    const s = stores()
    await resyncShell(s)
    expect(s.notifications.load).toHaveBeenCalledTimes(1)
  })

  it('re-reads the GitLab conflicts badge', async () => {
    const s = stores()
    await resyncShell(s)
    expect(s.conflicts.load).toHaveBeenCalledTimes(1)
  })

  it('rebuilds the sidebar too', async () => {
    const s = stores()
    await resyncShell(s)
    expect(s.workspaces.resync).toHaveBeenCalledTimes(1)
  })

  // Ordering matters: conflicts are workspace-scoped, and the reconnect may be
  // the only notice that our membership moved. Asking before the workspace set
  // settles would refetch the workspace we just left.
  it('asks for conflicts only after the workspace set has settled', async () => {
    const s = stores({ landsOn: 'w2' })
    await resyncShell(s)
    expect(s.calls[0]).toBe('workspaces')
    expect(s.conflicts.load).toHaveBeenCalledWith('w2')
  })

  // The whole point is that the caller can await the refresh — a resync that
  // resolved before its requests did would let a reconnect handler think the
  // shell is current while it is still mid-flight.
  it('waits for every refresh it starts', async () => {
    const s = stores()
    let releaseFeed
    s.notifications.load = vi.fn(() => new Promise((r) => (releaseFeed = r)))
    let done = false
    const p = resyncShell(s).then(() => (done = true))
    await Promise.resolve()
    expect(done).toBe(false)
    releaseFeed()
    await p
    expect(done).toBe(true)
  })
})
