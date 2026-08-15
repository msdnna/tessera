import { describe, it, expect } from 'vitest'
import {
  approvalProgress,
  canDecideNow,
  canRaiseApproval,
  documentApprovalState,
  orderedSteps,
  stepForApprover,
  stepState,
} from '@/utils/docApprovals'

// These rules mirror handlers/document_approvals.go. The tests below are written
// against the server's behaviour, not against this file's implementation: the
// point of the mirror is that the panel offers «Подписать» exactly when the
// server would accept it, so the interesting cases are the ones where a naive
// client would offer it and get a 409.

const step = (over = {}) => ({
  id: 's1',
  approver_id: 'u1',
  approver_name: 'Аня',
  position: 0,
  status: 'pending',
  ...over,
})

const route = (steps, over = {}) => ({
  id: 'a1',
  status: 'pending',
  mode: 'sequential',
  steps,
  ...over,
})

describe('orderedSteps', () => {
  it('walks the route by position, not by the order the API happened to return', () => {
    const a = route([
      step({ id: 's2', position: 1 }),
      step({ id: 's1', position: 0 }),
      step({ id: 's3', position: 2 }),
    ])
    expect(orderedSteps(a).map((s) => s.id)).toEqual(['s1', 's2', 's3'])
  })

  it('survives a route with no steps', () => {
    expect(orderedSteps(null)).toEqual([])
    expect(orderedSteps({})).toEqual([])
  })
})

describe('stepForApprover', () => {
  it('finds the caller', () => {
    const a = route([step({ id: 's1', approver_id: 'u1' }), step({ id: 's2', approver_id: 'u2' })])
    expect(stepForApprover(a, 'u2').id).toBe('s2')
  })

  it('does not match a deleted account against a missing user id', () => {
    // The server keeps approver_id NULL for a deleted account; a client that
    // matched null against an empty "me" would offer that person's signature to
    // whoever happened to be logged out.
    const a = route([step({ approver_id: null })])
    expect(stepForApprover(a, '')).toBeNull()
    expect(stepForApprover(a, undefined)).toBeNull()
  })
})

describe('canDecideNow', () => {
  it('lets the first pending approver sign a sequential route', () => {
    const a = route([
      step({ id: 's1', approver_id: 'u1', position: 0 }),
      step({ id: 's2', approver_id: 'u2', position: 1 }),
    ])
    expect(canDecideNow(a, 'u1')).toBe(true)
  })

  it('refuses the second approver until the first has signed', () => {
    const a = route([
      step({ id: 's1', approver_id: 'u1', position: 0 }),
      step({ id: 's2', approver_id: 'u2', position: 1 }),
    ])
    expect(canDecideNow(a, 'u2')).toBe(false)
  })

  it('reads position rather than array order when deciding whose turn it is', () => {
    // Same route, shuffled. A client that took the first pending element of the
    // list would say it is u2's turn, and the server would answer 409.
    const a = route([
      step({ id: 's2', approver_id: 'u2', position: 1 }),
      step({ id: 's1', approver_id: 'u1', position: 0 }),
    ])
    expect(canDecideNow(a, 'u2')).toBe(false)
    expect(canDecideNow(a, 'u1')).toBe(true)
  })

  it('asks everyone at once in a parallel route', () => {
    const a = route(
      [
        step({ id: 's1', approver_id: 'u1', position: 0 }),
        step({ id: 's2', approver_id: 'u2', position: 1 }),
      ],
      { mode: 'parallel' },
    )
    expect(canDecideNow(a, 'u1')).toBe(true)
    expect(canDecideNow(a, 'u2')).toBe(true)
  })

  it('refuses a second signature from someone who already decided', () => {
    const a = route(
      [
        step({ id: 's1', approver_id: 'u1', status: 'approved', position: 0 }),
        step({ id: 's2', approver_id: 'u2', position: 1 }),
      ],
      { mode: 'parallel' },
    )
    expect(canDecideNow(a, 'u1')).toBe(false)
  })

  it('refuses everyone once the route is closed', () => {
    for (const status of ['approved', 'rejected', 'cancelled']) {
      const a = route([step({ approver_id: 'u1' })], { status })
      expect(canDecideNow(a, 'u1')).toBe(false)
    }
  })

  it('refuses someone who is not on the route at all', () => {
    expect(canDecideNow(route([step({ approver_id: 'u1' })]), 'u9')).toBe(false)
  })
})

describe('stepState', () => {
  it('marks only the earliest pending step as current in a sequential route', () => {
    const steps = [
      step({ id: 's1', status: 'approved', position: 0 }),
      step({ id: 's2', position: 1 }),
      step({ id: 's3', position: 2 }),
    ]
    const a = route(steps)
    expect(stepState(a, steps[0])).toBe('signed')
    expect(stepState(a, steps[1])).toBe('current')
    expect(stepState(a, steps[2])).toBe('waiting')
  })

  it('marks every pending step as current in a parallel route', () => {
    const steps = [step({ id: 's1', position: 0 }), step({ id: 's2', position: 1 })]
    const a = route(steps, { mode: 'parallel' })
    expect(stepState(a, steps[0])).toBe('current')
    expect(stepState(a, steps[1])).toBe('current')
  })

  it('stops calling anything current once the route is closed', () => {
    const steps = [
      step({ id: 's1', status: 'rejected', position: 0 }),
      step({ id: 's2', position: 1 }),
    ]
    const a = route(steps, { status: 'rejected' })
    expect(stepState(a, steps[0])).toBe('rejected')
    expect(stepState(a, steps[1])).toBe('waiting')
  })
})

describe('approvalProgress', () => {
  it('counts signatures, not decisions', () => {
    // A rejection ends the route; counting it as progress would render
    // "2 из 3" next to «Отклонено».
    const a = route([
      step({ id: 's1', status: 'approved' }),
      step({ id: 's2', status: 'rejected' }),
      step({ id: 's3' }),
    ])
    expect(approvalProgress(a)).toEqual({ signed: 1, total: 3 })
  })
})

describe('documentApprovalState', () => {
  it('is null for a document never sent for approval', () => {
    // Different from "not approved": one has an answer, the other has no
    // question yet, and the panel says so differently.
    expect(documentApprovalState([])).toBeNull()
    expect(documentApprovalState(null)).toBeNull()
  })

  it('prefers the open route over a newer closed one', () => {
    const closed = { id: 'a2', status: 'approved' }
    const open = { id: 'a1', status: 'pending' }
    expect(documentApprovalState([closed, open]).id).toBe('a1')
  })

  it('falls back to the newest closed route', () => {
    const list = [
      { id: 'a2', status: 'rejected' },
      { id: 'a1', status: 'approved' },
    ]
    expect(documentApprovalState(list).id).toBe('a2')
  })
})

describe('canRaiseApproval', () => {
  it('allows a route when none is open', () => {
    expect(canRaiseApproval([{ status: 'approved' }])).toBe(true)
    expect(canRaiseApproval([])).toBe(true)
  })

  it('refuses a second open route', () => {
    expect(canRaiseApproval([{ status: 'pending' }])).toBe(false)
  })
})
