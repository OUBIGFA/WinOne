import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {randomUUID} from 'node:crypto';
import {createLocalBrowserBackend} from './fixtures/browser-extension-backend.mjs';

const repository = fileURLToPath(new URL('../../../', import.meta.url));
const system = {skip: !process.env.WINONE_GHOST_SANDBOX};
async function fixture(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(repository, '_temp/plugin-task-close/resume-20261106/mail/browser');
  await fs.mkdir(base, {recursive: true}); const f = await createLocalBrowserBackend(await fs.mkdtemp(path.join(base, 'browser-')));
  t.after(() => f.close()); return f;
}
async function until(read, accept, label, budget = 7000) {
  const deadline = Date.now() + budget;let last;
  while (Date.now() < deadline) {last = await read();if (accept(last)) return last;await new Promise(resolve => setTimeout(resolve, 25));}
  assert.fail(label + ': ' + JSON.stringify(last));
}

test('Browser backend actual main.js/SDK pairs through the original v3 slot and isolated native registration', system, async t => {
  const f = await fixture(t); t.diagnostic('browser backend constructed');
  const {client, response} = await f.client(); t.diagnostic('native/public-key handshake complete');
  assert.equal(response.status, 200); assert.ok(response.value.session); assert.equal(response.value.protocol, 3);
  const status = await f.browser({kind: 'status'}); assert.equal(status.extension_connected, true); assert.equal(status.nativeBinding, 'ready');
  assert.equal(f.registry.values.size, 3);
  assert.ok(f.registry.commands.length >= 4); assert.ok(f.registry.commands.every(command => command.file.startsWith(f.directory + path.sep)), 'not one real OS executable may be selected');
  const nativeToken = await f.token(status.port);
  assert.ok(!JSON.stringify([status, (await f.stats()), (await client.request('/health')).value]).includes(nativeToken), 'native token must not enter SDK state, public status or /health');
  assert.equal((await f.call('pair', {clients: [client.trusted()]})).result.ok, true);
  t.diagnostic('SDK pairing persisted; closing/reopening Host');
  await f.reopen(); t.diagnostic('Host reopened; reconnecting saved pairing');
  const restored = await f.client(client); t.diagnostic('saved pairing reconnect complete'); assert.equal(restored.response.status, 200); assert.ok(restored.response.value.session, 'actual Host reopen must restore saved pairing before accepting connections');
  assert.equal(restored.response.value.native_binding_required, false);
  assert.equal((await restored.client.request('/health', undefined, {Origin: 'https://ordinary.example.invalid'})).status, 403);
  assert.equal((await f.stats()).clients[0].id, client.id);
  t.diagnostic('closing only fixture-owned registration');
  await f.close(); t.diagnostic('fixture registration closed'); assert.equal(f.registry.values.size, 0, 'only fixture-owned native registration is withdrawn');
});

test('Browser backend original signed relay reaches the real installed provider once and late results do not replace it', system, async t => {
  const f = await fixture(t), {client} = await f.client(); const id = randomUUID();
  const input = {id, request: {op: 'translate', text: 'Local selected text', target: 'en', source: {url: 'https://example.invalid/page?token=private-fixture', title: 'Owned page'}}};
  const start = await client.learning('/learning/start', input); assert.equal(start.status, 200, JSON.stringify(start));
  assert.equal((await client.learning('/learning/start', input)).value.ok, true);
  const completed = await until(() => client.learning('/learning/result', {id}), result => result.value.done, 'real main.js relay result did not settle');
  assert.equal(completed.value.result.text, 'LOCAL SELECTED TEXT');
  const state = await f.stats(); assert.equal(state.relays.length, 1); assert.equal(state.relays[0].id, id);
  assert.ok(!state.relays[0].request.source.url.includes('private-fixture'), 'original relay URL credential redaction was lost');
  await f.browser({kind: 'relayResult', id, result: {ok: true, text: 'Late replacement'}});
  assert.equal((await client.learning('/learning/result', {id})).value.result.text, 'LOCAL SELECTED TEXT');
  assert.equal((await f.stats()).relays.length, 1, 'duplicate starts must not call provider business logic again');
  const wrongClient = await f.client();
  assert.equal((await wrongClient.client.learning('/learning/result', {id})).status, 404, 'another pairing must not read this relay result');
});

test('Browser backend delivered/acknowledged jobs timeout unknown and never repoll, accept late success or replay after reopen', system, async t => {
  const f = await fixture(t), {client} = await f.client(); await f.call('pair', {clients: [client.trusted()]});
  const pending = f.browser({kind: 'act', action: 'navigate', payload: {url: 'https://example.invalid/owned-target', browser: client.id}});
  const delivery = await until(() => client.request('/poll'), result => !!result.value.job, 'actual SDK job did not reach original bridge');
  const job = delivery.value.job; assert.equal(job.action, 'navigate');
  assert.equal((await client.request('/poll')).value.job, null);
  assert.equal((await client.request('/ack', {id: job.id})).value.ok, true);
  assert.equal((await client.request('/poll')).value.job, null);
  const result = await pending; assert.equal(result.ok, false); assert.equal(result.error, 'BROWSER_TIMEOUT'); assert.equal(result.execution, 'unknown');
  const late = await client.request('/result', {id: job.id, result: {ok: true, execution: 'executed', url: job.payload.url}});
  assert.equal(late.status, 409); assert.equal(late.value.error, 'JOB_EXPIRED');
  await f.reopen(); const restored = await f.client(client);
  assert.equal((await restored.client.request('/poll')).value.job, null); assert.equal((await f.browser({kind: 'status'})).pending, 0);
  assert.equal((await f.browser({kind: 'act', action: 'upload', payload: {paths: ['outside-fixture-file']}})).errorCode, 'INVALID_REQUEST');
});

test('Browser backend stopping the actual Host invalidates old bridge and its in-flight work cannot resurrect on reconnect', system, async t => {
  const f = await fixture(t), {client} = await f.client(); await f.call('pair', {clients: [client.trusted()]});
  const pending = f.browser({kind: 'act', action: 'navigate', payload: {url: 'https://example.invalid/stop-target', browser: client.id}}).then(result => ({result}), error => ({error}));
  const delivery = await until(() => client.request('/poll'), result => !!result.value.job, 'actual delivered job before stop missing');
  assert.equal((await client.request('/ack', {id: delivery.value.job.id})).value.ok, true);
  const original = f.bridges[f.bridges.length - 1];
  await f.host.call('enabled', {pluginId: f.installed.pluginId, enabled: false});
  const ended = await pending;
  // Host stops its real logic process as well. If its transport no longer
  // delivers a business receipt, retain that real failure rather than inventing
  // an executed/accepted result. The original bridge remains terminal either way.
  if (ended.result) {assert.equal(ended.result.ok, false);assert.notEqual(ended.result.execution, 'executed');}
  else assert.ok(ended.error);
  assert.equal((await original.status()).error, 'BRIDGE_STOPPED'); await assert.rejects(original.start(), /BRIDGE_STOPPED/);
  assert.equal(f.registry.values.size, 0);
  await f.host.call('enabled', {pluginId: f.installed.pluginId, enabled: true});
  const restored = await f.client(client); assert.equal((await restored.client.request('/poll')).value.job, null);
  assert.equal((await f.browser({kind: 'status'})).pending, 0);
});
