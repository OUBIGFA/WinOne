import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {createGhostWorktrees} from './worktrees.mjs';

function fakeLedger() { return {sqlite: {exec() {}}, db: {}}; }

async function fixture() {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'winone-ghost-worktree-'));
  const repo = path.join(root, 'repo');
  const metas = new Map();
  const states = new Map();
  const calls = [];
  const manager = {
    detectCwd: async directory => ({isGitRepo: true, isInsideWorktree: false, gitInstalled: true, repoRoot: repo, currentBranch: 'main', directory}),
    listAll: () => [...metas.values()],
    listBranches: async () => ({branches: ['main'], current: 'main'}),
    revParseCommit: async () => 'head-sha',
    suggestName: async () => 'worker-tree',
    createWorktree: async (request, options) => {
      calls.push({request, options});
      const meta = {sessionId: request.sessionId, name: request.name, path: path.join(repo, '.cindy-worktrees', request.name), baseRepo: repo, branch: `cindy/${request.name}`, sourceBranch: request.sourceBranch};
      metas.set(request.sessionId, meta);
      return {ok: true, meta};
    },
    getForSession: sessionId => metas.get(sessionId) ?? null,
    cancelPrecreatedWorktree: async (sessionId, locator, options) => {
      assert.equal(locator.path, metas.get(sessionId)?.path);
      if (!await options.canRemove()) return {status: 'preserved'};
      metas.delete(sessionId);
      return {status: 'discarded', branchDeleted: true};
    },
    removeWorktreeForSession: async (sessionId, options) => {
      if (states.get(`preserve:${sessionId}`)) return;
      if (!await options.canRemove()) return;
      metas.delete(sessionId);
    },
  };
  const host = async (_channel, request) => states.get(request.taskId) === 'archived' ? {status: 'archived'} : {status: 'active'};
  const worktrees = createGhostWorktrees({root, ledger: fakeLedger(), host, manager, log: {warn() {}, info() {}}});
  return {root, repo, metas, states, calls, manager, worktrees};
}

test('Ghost worktree creation uses the real Worker session id and waits for full checkout', async t => {
  const f = await fixture();
  t.after(() => f.worktrees.close());
  const source = await f.worktrees.resolve(f.repo, undefined, f.repo);
  assert.deepEqual(source, {sourceBranch: 'main', source: 'current'});
  const directory = await f.worktrees.create('worker-session-1', f.repo, source.sourceBranch);
  assert.equal(directory, f.metas.get('worker-session-1').path);
  assert.equal(f.calls[0].request.sessionId, 'worker-session-1');
  assert.equal(f.calls[0].options.trackBackgroundCheckout, true);
  assert.equal(f.calls[0].options.awaitFullCheckout, true);
});

test('an unregistered worktree directory is rejected before creation', async t => {
  const f = await fixture();
  t.after(() => f.worktrees.close());
  f.manager.detectCwd = async directory => ({isGitRepo: true, isInsideWorktree: true, gitInstalled: true, repoRoot: directory});
  await assert.rejects(() => f.worktrees.create('worker-session-2', path.join(f.repo, 'unknown'), 'main'), error => error.code === 'INVALID_PARAMS');
  assert.equal(f.calls.length, 0);
});

test('failed bootstrap compensation removes only after the Host says the Worker is archived', async t => {
  const f = await fixture();
  t.after(() => f.worktrees.close());
  await f.worktrees.create('worker-session-3', f.repo, 'main');
  const retained = await f.worktrees.discard('worker-session-3');
  assert.deepEqual(retained, {status: 'preserved'});
  assert.ok(f.metas.has('worker-session-3'));
  f.states.set('worker-session-3', 'archived');
  const discarded = await f.worktrees.discard('worker-session-3');
  assert.equal(discarded.status, 'discarded');
  assert.equal(f.metas.has('worker-session-3'), false);
});

test('archive delegates keep/dirty preservation to the Manager and reports the disposition', async t => {
  const f = await fixture();
  t.after(() => f.worktrees.close());
  await f.worktrees.create('worker-session-4', f.repo, 'main');
  f.states.set('worker-session-4', 'archived');
  f.states.set('preserve:worker-session-4', true);
  const preserved = await f.worktrees.archive('worker-session-4');
  assert.equal(preserved.status, 'preserved');
  assert.ok(f.metas.has('worker-session-4'));
  f.states.delete('preserve:worker-session-4');
  const removed = await f.worktrees.archive('worker-session-4');
  assert.equal(removed.status, 'removed');
  assert.equal(f.metas.has('worker-session-4'), false);
});

test('a Manager creation error is returned as an exact worktree failure', async t => {
  const f = await fixture();
  t.after(() => f.worktrees.close());
  f.manager.createWorktree = async () => ({ok: false, error: {kind: 'unknown', message: 'source branch missing', rawStderr: 'fatal: bad revision'}});
  await assert.rejects(() => f.worktrees.create('worker-session-5', f.repo, 'missing'), error => {
    assert.equal(error.code, 'WORKTREE_CREATE_FAILED');
    assert.equal(error.details.rawStderr, 'fatal: bad revision');
    return true;
  });
});
