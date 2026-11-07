import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {randomUUID} from 'node:crypto';
import {GhostBrowserExtensionSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/browserExtensionSlot.ts';
import {BrowserExtensionBridge} from '../cindy/ghost/apps/desktop/src/main/browser-extension/bridge.ts';
import {createNativeBinding} from '../cindy/ghost/apps/desktop/src/main/browser-extension/native-binding.ts';
import {LocalBrowserClient, isolatedRegistry} from './fixtures/browser-local.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
async function fixture(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/browser');
  await fs.mkdir(base, {recursive: true}); const directory = await fs.mkdtemp(path.join(base, 'browser-'));
  const registry = isolatedRegistry(), deliveries = [];
  let native, bridge, owner = 'private-owner';
  const ghost = {enabled: true, namespace: null, dir: path.join(directory, 'browser-fixture'), publishingSource: {type: 'local'},
    approval: {state: 'approved', revision: randomUUID()}, manifest: {id: 'browser-fixture', version: '1.0.0', name: 'Private browser fixture',
      node: {entry: 'worker.cjs'}, browserExtension: {extensionIds: ['a'.repeat(32)], pageControl: true, nativeHost: {name: 'com.winone.fixture.local', binary: 'native'}}}};
  const binaryDir = path.join(ghost.dir, 'native', 'win32-x64'); await fs.mkdir(binaryDir, {recursive: true});
  await fs.writeFile(path.join(binaryDir, 'cindy-browser-native.exe'), 'SANITIZED TEST BINARY; NEVER EXECUTED');
  const make = () => new GhostBrowserExtensionSlot({getGhost: id => id === 'browser-fixture' ? ghost : null,
    isVerifiedOfficial: () => false, policyDir: () => path.join(directory, 'pairings'), ownerScopeKey: () => owner,
    sendToGhost: (id, payload) => {deliveries.push({id, payload}); return true;}, wake: async () => false,
    createBridge: options => (bridge = new BrowserExtensionBridge({...options, ports: [0], pollTimeout: 20, jobTimeout: 2000, connectWait: 0})),
    createNativeBinding: options => (native = createNativeBinding({...options, platform: 'win32', arch: 'x64', home: path.join(directory, 'home'),
      env: {SystemRoot: path.join(directory, 'fake-windows')}, root: path.join(directory, 'native-registration'), run: registry.run})),
  });
  let slot = make();slot.reconcile([ghost]);
  let status = await slot.handleRequest('browser-fixture', {kind: 'status'});assert.equal(status.ok, true, JSON.stringify(status));
  t.after(() => slot.dispose());
  return {directory, registry, deliveries, ghost, request: (...args) => slot.handleRequest('browser-fixture', ...args),
    bridge: () => bridge, native: () => native, slot: () => slot,
    async reopen() {await slot.dispose();slot = make();slot.reconcile([ghost]);return slot.handleRequest('browser-fixture', {kind: 'status'});},
    changeOwner() {owner = 'other-owner';}, status};
}

async function connected(f, token = true) {
  const client = new LocalBrowserClient(f.bridge().listeningPort);
  const response = await client.connect(token ? f.native().token : undefined);
  assert.equal(response.status, 200);assert.ok(response.value.session, JSON.stringify(response.value));
  return client;
}

test('Local browser original slot/native binding authenticates protocol-v3 pairing and persists it without leaking credentials', async t => {
  const f = await fixture(t), client = await connected(f);
  const status = await f.request({kind: 'status'});assert.equal(status.extension_connected, true);assert.equal(status.nativeBinding, 'ready');
  assert.ok(!JSON.stringify([status, (await client.request('/health')).value]).includes(f.native().token));
  assert.equal(f.registry.values.size, 3);
  assert.ok(f.registry.commands.every(command => command.file.startsWith(f.directory + path.sep)), 'no real system command may execute or be selected');
  const paired = await f.request({kind: 'setTrustedClients', clients: [client.trusted()]});assert.equal(paired.ok, true);
  assert.equal(await f.slot().activate('browser-fixture'), true);
  assert.equal(f.slot().getPageControl().pluginName, 'Private browser fixture');
  const files = await fs.readdir(path.join(f.directory, 'pairings'));assert.ok(files.some(name => name.endsWith('.json')));
  await f.reopen();
  const restored = new LocalBrowserClient(f.bridge().listeningPort);restored.id = client.id;restored.privateKey = client.privateKey;restored.publicKeyText = client.publicKeyText;restored.encodedKey = client.encodedKey;
  restored.headers = {...client.headers, 'x-my-browser-client': client.id};
  const reconnect = await restored.connect();assert.ok(reconnect.value.session, 'saved public-key pairing must restore before connections are accepted');
  assert.equal(reconnect.value.native_binding_required, false);
  assert.equal((await restored.request('/health', undefined, {Origin: 'https://ordinary-webpage.example'})).status, 403);
});

test('Local browser delivered/acknowledged jobs are never repolled and stopping reports unknown instead of replaying', async t => {
  const f = await fixture(t), client = await connected(f);
  const pending = f.request({kind: 'act', action: 'navigate', payload: {url: 'https://www.example.com', browser: client.id}});
  const first = await client.request('/poll');assert.ok(first.value.job, JSON.stringify(first));
  const second = await client.request('/poll');assert.equal(second.value.job, null, 'delivered job must never be reissued');
  assert.equal((await client.request('/ack', {id: first.value.job.id})).value.ok, true);
  await f.slot().dispose();
  const result = await pending;assert.equal(result.ok, false);assert.equal(result.error, 'BRIDGE_STOPPED');assert.equal(result.execution, 'unknown');
  await assert.rejects(f.bridge().start(), /BRIDGE_STOPPED/);
});

test('Local browser signed relay reaches only its declared plugin and duplicate start/result never replays work', async t => {
  const f = await fixture(t), client = await connected(f);
  const id = randomUUID(); const input = {id, request: {op: 'translate', text: 'Local selected text', target: 'en', source: {url: 'https://www.example.com/page?token=fixture-secret', title: 'Fixture'}}};
  const started = await client.learning('/learning/start', input);assert.equal(started.status, 200, JSON.stringify(started));
  const repeated = await client.learning('/learning/start', input);assert.equal(repeated.value.ok, true);
  const relays = f.deliveries.filter(item => item.payload.name === 'browser-extension-relay');
  assert.equal(relays.length, 1);assert.equal(relays[0].id, 'browser-fixture');assert.equal(relays[0].payload.id, id);
  assert.ok(!relays[0].payload.request.source.url.includes('fixture-secret'), 'relay source URLs retain original credential redaction');
  assert.equal((await f.request({kind: 'relayProgress', id, progress: {stage: 'generating', receivedChars: 12}})).ok, true);
  assert.equal((await f.request({kind: 'relayResult', id, result: {ok: true, text: 'Original result'}})).ok, true);
  await f.request({kind: 'relayResult', id, result: {ok: true, text: 'Replacement must be ignored'}});
  const result = await client.learning('/learning/result', {id});assert.equal(result.value.done, true);assert.equal(result.value.result.text, 'Original result');
  assert.equal(f.deliveries.filter(item => item.payload.name === 'browser-extension-relay').length, 1);
});

test('Local browser native registration closes only owned copies and changed package sources cannot inherit saved pairings', async t => {
  const f = await fixture(t), client = await connected(f);
  await f.request({kind: 'setTrustedClients', clients: [client.trusted()]});
  await f.slot().dispose();assert.equal(f.registry.values.size, 0);
  f.ghost.publishingSource = {type: 'other-local-source'};await f.reopen();
  const fresh = new LocalBrowserClient(f.bridge().listeningPort);fresh.id = client.id;fresh.privateKey = client.privateKey;fresh.encodedKey = client.encodedKey;fresh.headers = {...client.headers};
  const response = await fresh.connect();assert.equal(response.value.pairing_required, true, 'source change must not silently inherit a prior native/public-key pairing');
  assert.equal((await f.request({kind: 'act', action: 'upload', payload: {paths: ['outside-fixture-file']}})).errorCode, 'INVALID_REQUEST');
});
