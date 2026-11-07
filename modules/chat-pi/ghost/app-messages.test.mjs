import test from 'node:test';
import assert from 'node:assert/strict';
import {createNativeAppMessageSender} from './platform-app-messages.mjs';
import {queuedAppMessageGuard} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/message.ts';

const record = {serverId: 'plugin-ui_fixture', tool: {_meta: {'cindy/plugin': {name: 'Host stamped plugin'}}}};
async function fixture(options = {}) {
  let current = true, checks = 0, receipt = options.receipt ?? {accepted: true};
  const requests = [];
  const host = async (method, input) => {
    if (method === 'session-snapshot') return {mcpAppEpoch: 3, remoteHostId: null, archived: false};
    assert.equal(method, 'task'); assert.equal(input.op, 'mcp-message'); requests.push(input);
    if (options.accept) return options.accept(input);
    return receipt;
  };
  const sender = createNativeAppMessageSender({host});
  const guard = async () => {checks++; if (!current) throw Error('MCP_APP_CARD_UNAVAILABLE');};
  return {sender, guard, requests, checks: () => checks, revoke() {current = false;}, receipt(value) {receipt = value;}};
}

test('MCP App messages retain original escaped commands, authored history and Host-stamped external provenance', async () => {
  const f = await fixture();
  await f.sender.send('task', record, [{type: 'text', text: '/compact'}, {type: 'resource_link', name: '/file', uri: 'https://example.invalid/asset'}], f.guard);
  const request = f.requests[0];
  assert.match(request.clientId, /^mcp-app-message:/);
  assert.equal(request.text, '\\/compact\n\\/file: https://example.invalid/asset');
  assert.equal(request.authoredText, '/compact\n/file: https://example.invalid/asset');
  assert.deepEqual(request.presentationSource, {pluginId: 'mcp-app:plugin-ui_fixture', name: 'Host stamped plugin'});
  assert.equal(request.expectedEpoch, 3);
  assert.equal(request.approved, undefined); assert.equal(request.ownerVerified, undefined);
  assert.equal(Object.getOwnPropertySymbols(request).length, 0);
  assert.ok(!JSON.stringify(request).includes('MCP_APP_MESSAGE_GUARD'));
  await f.sender.assert({sessionId: 'task', requestId: request.clientId});
  const before = f.checks();
  await f.sender.assert({sessionId: 'task', requestId: request.clientId});
  assert.equal(f.checks(), before + 1, 'every generation must call the actual guard, not a memo');
  f.revoke();
  await assert.rejects(f.sender.assert({sessionId: 'task', requestId: request.clientId}), /CARD_UNAVAILABLE/);
  f.sender.close();
});

test('MCP App messages wait for real async NativePiInbox acceptance and reject plain ok/not-accepted replies', async () => {
  let release, input;
  const pending = new Promise(resolve => {release = resolve;});
  const f = await fixture({accept: request => {input = request; return pending;}});
  let settled = false;
  const send = f.sender.send('task', record, [{type: 'text', text: 'Continue'}], f.guard).then(() => {settled = true;});
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(settled, false);
  assert.ok(input);
  await f.sender.assert({sessionId: 'task', requestId: input.clientId});
  release({accepted: true}); await send;
  assert.equal(settled, true); f.sender.close();
  for (const receipt of [{ok: true}, {accepted: false}]) {
    const denied = await fixture({receipt});
    await assert.rejects(denied.sender.send('task', record, [{type: 'text', text: 'Continue'}], denied.guard), /NOT_ACCEPTED/);
    await assert.rejects(denied.sender.assert({sessionId: 'task', requestId: denied.requests[0].clientId}), /CARD_UNAVAILABLE/);
    denied.sender.close();
  }
});

test('MCP App guard is session-bound; JSON/restart, settlement, cancellation and close cannot recreate it', async () => {
  const f = await fixture();
  await f.sender.send('task', record, [{type: 'text', text: 'Continue'}], f.guard);
  const request = f.requests[0];
  await assert.rejects(f.sender.assert({sessionId: 'other-task', requestId: request.clientId}), /CARD_UNAVAILABLE/);
  const restored = createNativeAppMessageSender({host: async () => null});
  const metadata = JSON.parse(JSON.stringify({sessionId: 'task', requestId: request.clientId}));
  await assert.rejects(restored.assert(metadata), /CARD_UNAVAILABLE/);
  await assert.rejects(queuedAppMessageGuard({clientId: request.clientId, sourcePlugin: request.presentationSource})(), /CARD_UNAVAILABLE/);
  assert.equal(queuedAppMessageGuard({clientId: 'ordinary-owner-input'}), undefined);
  f.sender.cancelSession('other-task'); await f.sender.assert(metadata);
  f.sender.settled(metadata); await assert.rejects(f.sender.assert(metadata), /CARD_UNAVAILABLE/);
  await f.sender.send('task', record, [{type: 'text', text: 'Again'}], f.guard);
  const second = {sessionId: 'task', requestId: f.requests[1].clientId};
  f.sender.cancelSession('task'); await assert.rejects(f.sender.assert(second), /CARD_UNAVAILABLE/);
  f.sender.close(); restored.close();
});

test('MCP App messages pass only the backend-proven image references without new path/base64 projection', async () => {
  const f = await fixture();
  const files = [{id: 'owned', name: 'image.png', category: 'image', mimeType: 'image/png', url: 'cindy-media://blobs/' + 'a'.repeat(64) + '.png', path: 'cindy-media://blobs/' + 'a'.repeat(64) + '.png'}];
  await f.sender.send('task', record, [{type: 'text', text: 'Use this image'}], f.guard, files);
  assert.equal(f.requests[0].files, files);
  assert.equal(f.requests[0].files[0].data, undefined);
  f.sender.close();
});
