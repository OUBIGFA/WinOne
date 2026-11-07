import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {GhostNetworkSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/networkSlot.ts';
import {GhostHandoffStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/handoffStore.ts';
import {GHOST_HANDOFF_RENEW_INTERVAL_MS, GHOST_HANDOFF_TTL_MS} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {guardedResponseFetch} from './seams/outbound-fetch.ts';

const root = fileURLToPath(new URL('../../../', import.meta.url));
async function fixture(t, options = {}) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/unit');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'upload-'));
  let now = 1000;
  const store = new GhostHandoffStore({root: () => directory, now: () => now});
  const [file] = await store.import('fixture', [{name: 'report.csv', bytes: Buffer.from('a,b\n1,2\n')}], () => {});
  const calls = [], touched = [];
  const hostname = options.blocked ? '127.0.0.1' : 'uploads.example.invalid';
  const ghost = {enabled: true, manifest: {id: 'fixture', kind: 'chip', network: {hosts: [hostname], secrets: []}}};
  const slot = new GhostNetworkSlot({getGhost: () => ghost, captureRequestGuard: () => () => {},
    getLoginEmail: () => null, readSecret: () => null,
    fetchImpl: options.blocked ? guardedResponseFetch : async (url, init) => {calls.push({url, init}); return new Response('{"ok":true}', {status: 200, headers: {'Content-Type': 'application/json'}});},
    fetchPublicImpl: async () => {throw Error('unexpected public-network bypass');},
    readGhostMedia: async () => null, takeDirDeposit: () => null, writeSaveDeposit: async () => null,
    saveGhostMedia: async () => {throw Error('unexpected media deposit');}, isSupportedMediaMime: () => false,
    readHandoffFile: (_id, ref, signal) => {signal.throwIfAborted();return store.read(_id, ref);},
    touchHandoffFiles: async (id, refs) => {touched.push({id, refs});await store.touch(id, refs);},
  });
  now += GHOST_HANDOFF_RENEW_INTERVAL_MS + 1;
  return {slot, store, file, calls, touched, now: () => now, url: 'https://' + hostname + '/upload'};
}

test('MCP handoff upload is consumed by the real NetworkSlot multipart path and only successful uploads renew it', async t => {
  const f = await fixture(t);
  const result = await f.slot.handleFetchRequest('fixture', {url: f.url, method: 'POST', upload: {handoff: [f.file.ref], field: 'sheet'}});
  assert.equal(result.ok, true, JSON.stringify(result));
  assert.equal(f.calls.length, 1);
  const request = f.calls[0].init;
  assert.match(request.headers['Content-Type'] ?? request.headers['content-type'], /multipart\/form-data/);
  const body = Buffer.from(request.body).toString('utf8');
  assert.match(body, /name="sheet"/); assert.match(body, /report\.csv/); assert.match(body, /a,b\n1,2\n/);
  assert.equal(f.touched.length, 1);
  assert.equal((await f.store.list('fixture'))[0].expiresAt, f.now() + GHOST_HANDOFF_TTL_MS);
  await f.store.delete('fixture', f.file.ref);
  const absent = await f.slot.handleFetchRequest('fixture', {url: f.url, method: 'POST', upload: {handoff: [f.file.ref]}});
  assert.equal(absent.ok, false); assert.equal(absent.errorCode, 'HANDOFF_UNAVAILABLE');
  assert.equal(f.calls.length, 1); assert.equal(f.touched.length, 1);
});

test('MCP handoff upload cannot borrow another plugin copy, mix sources, repeat refs or bypass HTTPS', async t => {
  const f = await fixture(t);
  const other = await f.slot.handleFetchRequest('other', {url: f.url, method: 'POST', upload: {handoff: [f.file.ref]}});
  assert.equal(other.ok, false); assert.equal(other.errorCode, 'HANDOFF_UNAVAILABLE');
  for (const payload of [
    {url: f.url, method: 'POST', upload: {handoff: [f.file.ref], hashes: ['a'.repeat(64)]}},
    {url: f.url, method: 'POST', upload: {handoff: [f.file.ref, f.file.ref]}},
    {url: f.url.replace('https:', 'http:'), method: 'POST', upload: {handoff: [f.file.ref]}},
  ]) assert.equal((await f.slot.handleFetchRequest('fixture', payload)).ok, false);
  assert.equal(f.calls.length, 0); assert.equal(f.touched.length, 0);
});

test('MCP handoff upload preserves the genuine single-hop SSRF denial even for a declared loopback host', async t => {
  const f = await fixture(t, {blocked: true});
  const result = await f.slot.handleFetchRequest('fixture', {url: f.url, method: 'POST', upload: {handoff: [f.file.ref]}});
  assert.equal(result.ok, false, 'declared network hosts must not exempt private addresses');
  assert.equal(f.touched.length, 0);
  assert.equal((await f.store.list('fixture'))[0].expiresAt, f.file.expiresAt);
});
