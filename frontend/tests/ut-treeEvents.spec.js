import { describe, it, expect } from 'vitest'
import { treeEventNeedsRefresh, milestoneTreeTarget } from '@/utils/treeEvents'

const WS = 'ws-1'
const ev = (type, over = {}) => ({ type, scope: WS, data: { id: 'x' }, ...over })
const needs = (type, over) => treeEventNeedsRefresh(ev(type, over), WS)

describe('treeEventNeedsRefresh', () => {
  it('re-reads the tree for every group mutation', () => {
    for (const type of ['group.created', 'group.updated', 'group.deleted', 'group.moved']) {
      expect(needs(type), type).toBe(true)
    }
  })

  it('re-reads the tree for every project mutation', () => {
    for (const type of [
      'project.created',
      'project.updated',
      'project.deleted',
      'project.moved',
    ]) {
      expect(needs(type), type).toBe(true)
    }
  })

  // The estimation config is patched into the store in place (onEstimationEvent);
  // two requests to arrive at the same rows would be pure waste.
  it('leaves estimation events to the store patch', () => {
    expect(needs('project.estimation')).toBe(false)
    expect(needs('workspace.estimation')).toBe(false)
  })

  // Boards have their own patcher (workspaces store onBoardEvent) that keeps the
  // expanded lists — refresh() deliberately does not touch those.
  it('leaves board events to onBoardEvent', () => {
    expect(needs('board.created')).toBe(false)
    expect(needs('board.updated')).toBe(false)
    expect(needs('board.deleted')).toBe(false)
  })

  it('ignores events that cannot touch the tree', () => {
    expect(needs('task.updated')).toBe(false)
    expect(needs('note.created')).toBe(false)
    expect(needs('notification')).toBe(false)
    expect(needs('column.deleted')).toBe(false)
  })

  // Events are workspace-scoped; another workspace's tree is not the one drawn.
  it('ignores a tree event from another workspace', () => {
    expect(treeEventNeedsRefresh(ev('project.created', { scope: 'ws-2' }), WS)).toBe(false)
  })

  // Before a workspace is picked there is no tree to re-read, and refresh() would
  // bail anyway — say so here rather than lean on that.
  it('ignores everything while no workspace is selected', () => {
    expect(treeEventNeedsRefresh(ev('project.created', { scope: '' }), '')).toBe(false)
    expect(treeEventNeedsRefresh(ev('project.created'), '')).toBe(false)
  })

  it('survives a malformed event', () => {
    expect(treeEventNeedsRefresh(null, WS)).toBe(false)
    expect(treeEventNeedsRefresh({}, WS)).toBe(false)
    expect(treeEventNeedsRefresh({ scope: WS }, WS)).toBe(false)
  })

  // Sprint events are NOT a tree refresh: groups/projects don't move, only a
  // project's own milestone rows do (milestoneTreeTarget below).
  it('leaves milestone events to milestoneTreeTarget', () => {
    expect(needs('milestone.created')).toBe(false)
    expect(needs('milestone.updated')).toBe(false)
    expect(needs('milestone.deleted')).toBe(false)
  })
})

describe('milestoneTreeTarget', () => {
  const msEv = (type, data) => ({ type, scope: WS, data })

  it('names the project of a create/update (the milestone row travels)', () => {
    expect(milestoneTreeTarget(msEv('milestone.created', { id: 'm1', project_id: 'p1' }), WS)).toBe(
      'p1',
    )
    expect(milestoneTreeTarget(msEv('milestone.updated', { id: 'm1', project_id: 'p2' }), WS)).toBe(
      'p2',
    )
  })

  // `milestone.deleted` is a bare {id}, and so is the GitLab sync's
  // `milestone.updated` — no project to scope to, so every loaded list is stale.
  it('asks for every list when the event names no project', () => {
    expect(milestoneTreeTarget(msEv('milestone.deleted', { id: 'm1' }), WS)).toBe(null)
    expect(milestoneTreeTarget(msEv('milestone.updated', { id: 'm1' }), WS)).toBe(null)
    expect(milestoneTreeTarget(msEv('milestone.created', { id: 'm1', project_id: '' }), WS)).toBe(
      null,
    )
  })

  it('ignores anything that is not a sprint event of this workspace', () => {
    expect(milestoneTreeTarget(msEv('task.updated', { id: 't1' }), WS)).toBe(undefined)
    expect(milestoneTreeTarget(msEv('board.created', { id: 'b1' }), WS)).toBe(undefined)
    expect(
      milestoneTreeTarget({ type: 'milestone.created', scope: 'ws-2', data: { id: 'm1' } }, WS),
    ).toBe(undefined)
    expect(milestoneTreeTarget(msEv('milestone.created', { id: 'm1' }), '')).toBe(undefined)
  })

  it('survives a malformed event', () => {
    expect(milestoneTreeTarget(null, WS)).toBe(undefined)
    expect(milestoneTreeTarget({}, WS)).toBe(undefined)
    expect(milestoneTreeTarget({ type: 'milestone.deleted', scope: WS }, WS)).toBe(null)
  })
})
