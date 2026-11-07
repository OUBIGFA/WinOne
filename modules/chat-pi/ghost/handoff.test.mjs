import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {GhostHandoffStore, GhostHandoffQuotaError, handoffDiskName} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/handoffStore.ts';
import {GHOST_HANDOFF_TTL_MS, GHOST_HANDOFF_RENEW_INTERVAL_MS, GHOST_HANDOFF_QUOTA_BYTES} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {createGhostHandoff, withoutHandoffArg} from './platform-handoff.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
async function directory(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/unit');
  await fs.mkdir(base, {recursive: true});
  return fs.mkdtemp(path.join(base, 'handoff-'));
}

test('MCP handoff store enforces exact instance, default quota, atomic batches and relocation holds', async t => {
  const root = await directory(t);
  const store = new GhostHandoffStore({root: () => root});
  const [a] = await store.import('plugin-a', [{name: 'data.csv', bytes: Buffer.from('one')}], () => {});
  assert.equal((await store.read('plugin-a', a.ref)).bytes.toString(), 'one');
  assert.equal(await store.read('plugin-b', a.ref), null);
  await assert.rejects(store.assertQuota('plugin-a', [GHOST_HANDOFF_QUOTA_BYTES]), GhostHandoffQuotaError);
  await assert.rejects(store.assertQuota('plugin-a', Array(200).fill(0)), GhostHandoffQuotaError);
  let checks = 0;
  await assert.rejects(store.import('plugin-a', [{name: 'b.csv', bytes: Buffer.from('two')}, {name: 'c.csv', bytes: Buffer.from('three')}], () => { if (++checks === 3) throw Error('owner changed'); }), /owner changed/);
  assert.equal((await store.list('plugin-a')).length, 1);
  const release = store.hold('plugin-a');
  assert.equal(store.isBusy('plugin-a'), true);
  const unblock = store.blockForRelocation(['plugin-a']);
  assert.equal(store.hold('plugin-a'), null);
  release(); release();
  assert.equal(store.isBusy('plugin-a'), false);
  unblock();
  const next = store.hold('plugin-a'); assert.ok(next); next();
  assert.equal(await store.delete('plugin-b', a.ref), false);
  assert.equal(await store.delete('plugin-a', a.ref), true);
  assert.equal(await store.read('plugin-a', a.ref), null);
});

test('MCP handoff store renews page issuance/read and successful-use TTL, expires without revival', async t => {
  const root = await directory(t); let now = 1000;
  const store = new GhostHandoffStore({root: () => root, now: () => now});
  const [file] = await store.import('owner', [{name: 'CON.csv', bytes: Buffer.from('csv')}], () => {});
  assert.equal(handoffDiskName('CON.csv'), '_CON.csv');
  assert.equal(file.expiresAt, now + GHOST_HANDOFF_TTL_MS);
  now += GHOST_HANDOFF_RENEW_INTERVAL_MS + 1;
  const opened = await store.openForPage('owner', file.ref); assert.ok(opened); await opened.file.close();
  const renewed = (await store.list('owner'))[0];
  assert.equal(renewed.expiresAt, now + GHOST_HANDOFF_TTL_MS);
  now += 50;
  await store.touch('owner', [file.ref]);
  assert.equal((await store.list('owner'))[0].expiresAt, renewed.expiresAt);
  now = renewed.expiresAt + 1;
  await store.touch('owner', [file.ref]);
  assert.equal(await store.openForPage('owner', file.ref), null);
  assert.deepEqual(await store.list('owner'), []);
});

test('MCP handoff concurrent imports cannot exceed an original per-instance quota', async t => {
  const root = await directory(t);
  const store = new GhostHandoffStore({root: () => root, quota: {files: 1, bytes: 10}});
  const result = await Promise.allSettled([store.import('p', [{name: 'a.csv', bytes: Buffer.from('a')}], () => {}), store.import('p', [{name: 'b.csv', bytes: Buffer.from('b')}], () => {})]);
  assert.equal(result.filter(value => value.status === 'fulfilled').length, 1);
  assert.equal((await store.list('p')).length, 1);
});

async function adapterFixture(t) {
  const root = await directory(t);
  const ghost = {manifest: {id: 'fixture', tools: [{name: 'open', parameters: {type: 'object', properties: {cindy_handoff_files: {type: 'array'}}}}]}, enabled: true};
  let current = true, permission = 'full', generation = 1;
  const scope = {};
  const decisions = [];
  const adapter = createGhostHandoff({root, find: () => ghost, current: () => current,
    ownerScope: {capture: () => scope, isCurrent: value => value === scope && current, isStable: () => current},
    host: async (method, params) => {
      if (method === 'session-snapshot') return {workingDir: root, permissionMode: permission, permissionGeneration: generation, remoteHostId: null};
      if (method === 'handoff-authorize') { decisions.push(params); return {ok: permission === 'full'}; }
      throw Error('unexpected ' + method);
    }});
  t.after(() => adapter.close());
  const file = path.join(root, 'report.csv'); await fs.writeFile(file, 'a,b\n1,2\n');
  return {root, ghost, file, adapter, decisions, revoke() { current = false; }, ask() { permission = 'ask'; generation++; }};
}

test('MCP handoff adapter deduplicates resolved files, strips only reserved arrays, and copies the approved bytes', async t => {
  const f = await adapterFixture(t);
  const prepared = await f.adapter.prepare({ghost: f.ghost, tool: 'open', urls: [f.file, f.file, path.join(f.root, '.', 'report.csv')], session: {id: 'task', approved: true}});
  assert.deepEqual(prepared.args({keep: 1, cindy_handoff_files: [{ref: 'handoff:forged'}]}), {keep: 1});
  assert.deepEqual(withoutHandoffArg({keep: 1}), {keep: 1});
  assert.equal(f.decisions.length, 1);
  assert.equal(f.decisions[0].items.length, 1);
  assert.match(f.decisions[0].items[0].sha256, /^[a-f0-9]{64}$/);
  await fs.writeFile(f.file, 'not the approved contents');
  const committed = await prepared.commit();
  assert.equal(committed.files.length, 1);
  assert.equal((await f.adapter.store.read('fixture', committed.files[0].ref)).bytes.toString(), 'a,b\n1,2\n');
  await committed.discard(); committed.release();
  assert.deepEqual(await f.adapter.store.list('fixture'), []);
});

test('MCP handoff adapter rechecks live permissions before storing and never keeps a Full Access grant after Ask', async t => {
  const f = await adapterFixture(t);
  const prepared = await f.adapter.prepare({ghost: f.ghost, tool: 'open', urls: [f.file], session: {id: 'task', approved: true}});
  f.ask();
  await assert.rejects(prepared.commit(), /权限已变化/);
  assert.deepEqual(await f.adapter.store.list('fixture'), []);
  await assert.rejects(f.adapter.prepare({ghost: f.ghost, tool: 'open', urls: [f.file], session: {id: 'task', approved: true}}), /未授权/);
  assert.equal(f.decisions.length, 2);
});

test('MCP handoff adapter rejects undeclared, sensitive, over-count and oversized files before a permission card', async t => {
  const f = await adapterFixture(t);
  await assert.rejects(f.adapter.prepare({ghost: f.ghost, tool: 'other', urls: [f.file], session: {id: 'task'}}), /声明/);
  const secret = path.join(f.root, '.env'); await fs.writeFile(secret, 'fixture only');
  await assert.rejects(f.adapter.prepare({ghost: f.ghost, tool: 'open', urls: [secret], session: {id: 'task'}}), /凭证/);
  await assert.rejects(f.adapter.prepare({ghost: f.ghost, tool: 'open', urls: Array.from({length: 5}, (_, i) => f.file + i), session: {id: 'task'}}), /最多/);
  const big = path.join(f.root, 'big.csv'); const handle = await fs.open(big, 'w'); await handle.truncate(64 * 1024 * 1024 + 1); await handle.close();
  await assert.rejects(f.adapter.prepare({ghost: f.ghost, tool: 'open', urls: [big], session: {id: 'task'}}), /64 MiB/);
  assert.equal(f.decisions.length, 0);
});
