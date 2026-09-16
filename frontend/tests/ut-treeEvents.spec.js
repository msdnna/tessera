import { describe, it, expect } from 'vitest'
import { treeEventNeedsRefresh } from '@/utils/treeEvents'

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
})
