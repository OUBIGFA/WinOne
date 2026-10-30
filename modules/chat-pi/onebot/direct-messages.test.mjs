// Cindy send_to_agent on the OneBot task store: bounded teammate messages
// delivered into each companion's own main chat, traced in both timelines.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createOneBotRuntime} from './runtime.mjs';
import {createOneBotTaskClient} from './task-client.mjs';
import {UI_ACTION_TRIGGER_PREFIX} from '../cindy/bots/shared/interruptedTurn.ts';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('An isolated OneBot test output is required');

async function fixture({bindBeta = true} = {}) {
  const directory = await fs.mkdtemp(path.join(base, 'direct-')), alpha = 'A'.repeat(26), beta = 'B'.repeat(26);
  const dispatches = [], timeline = [], creations = [];
  const host = {
    readSession: async id => ({id, title: 'Chat', model: 'model', providerId: 'fixture', agentKind: 'pi', effort: '', fastMode: false,
      permissionMode: 'ask', workingDir: directory, status: 'active', source: 'bot'}),
    dispatch: async input => {dispatches.push(input); await input.onAccepted?.(); return {ok: true, targetSessionId: input.targetSessionId, wakeKind: 'created'};},
    persistTimelineMessage: async input => {timeline.push(input);},
    hasQueuedInput: async () => false, withdrawQueuedInput: async () => {}, abortSession: async () => {},
    validateTaskModel: async () => true,
    createCanonicalSession: async input => {
      creations.push(input);
      return runtime.call('sessions.replaceCanonical', {...input, sessionId: beta});
    },
  };
  const runtime = createOneBotRuntime({directory, tasks: host});
  for (const [id, name, session] of [['alpha', 'Alpha', alpha], ['beta', 'Beta', beta]]) {
    await runtime.call('profiles.create', {input: {id, name}});
    if (id !== 'beta' || bindBeta) await runtime.call('sessions.bind', {botId: id, sessionId: session});
  }
  return {runtime, alpha, beta, dispatches, timeline, creations};
}

test('a teammate message reaches the other main chat as a hidden turn and is traced in both chats', async () => {
  const {runtime, alpha, beta, dispatches, timeline} = await fixture();
  try {
    const roster = await runtime.call('tasks.agents.list', {callerSessionId: alpha});
    assert.equal(roster.ok, true);
    assert.deepEqual(roster.agents.map(({id, name, local}) => ({id, name, local})), [{id: 'beta', name: 'Beta', local: true}]);
    const sent = await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'beta', message: '  Can you check the report?  '});
    assert.equal(sent.ok, true, JSON.stringify(sent));
    assert.deepEqual({target_id: sent.target_id, target_name: sent.target_name, delivered: sent.delivered, replied: sent.replied}, {target_id: 'beta', target_name: 'Beta', delivered: true, replied: false});
    assert.equal(dispatches.length, 1);
    const [delivery] = dispatches;
    assert.equal(delivery.targetSessionId, beta);
    assert.match(delivery.clientId, /^bot-dm:/);
    assert.equal(delivery.message, '[Direct message from Cindy Bot "Alpha" (alpha)]\n\nCan you check the report?');
    // The recipient's chat shows the trace card, not a user bubble.
    assert.ok(delivery.persistedContent.startsWith(UI_ACTION_TRIGGER_PREFIX));
    const anchors = timeline.map(({sessionId, agentMeta}) => ({sessionId, ...agentMeta.botDirectMessage}));
    assert.deepEqual(anchors.map(({sessionId, direction, peerBotId, peerBotName, preview}) => ({sessionId, direction, peerBotId, peerBotName, preview})).sort((a, b) => a.sessionId.localeCompare(b.sessionId)), [
      {sessionId: alpha, direction: 'sent', peerBotId: 'beta', peerBotName: 'Beta', preview: 'Can you check the report?'},
      {sessionId: beta, direction: 'received', peerBotId: 'alpha', peerBotName: 'Alpha', preview: 'Can you check the report?'},
    ]);
    const view = await runtime.call('tasks.directMessage.get', {threadId: anchors[0].threadId, viewerBotId: 'beta'});
    assert.equal(view.ok, true);
    assert.deepEqual(view.thread.messages.map(({senderBotName, recipientBotName, content}) => ({senderBotName, recipientBotName, content})),
      [{senderBotName: 'Alpha', recipientBotName: 'Beta', content: 'Can you check the report?'}]);
    assert.equal((await runtime.call('tasks.directMessage.get', {threadId: anchors[0].threadId, viewerBotId: 'gamma'})).errorCode, 'NOT_FOUND');
  } finally { await runtime.close(); }
});

test('a teammate without a main chat is recovered through the native canonical path once', async () => {
  const {runtime, alpha, beta, creations, dispatches} = await fixture({bindBeta: false});
  try {
    for (const message of ['First', 'Second']) {
      const result = await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'beta', message});
      assert.equal(result.ok, true, JSON.stringify(result));
    }
    assert.equal(creations.length, 1);
    assert.equal(creations[0].botId, 'beta');
    assert.equal(creations[0].expectedCanonicalSessionId, null);
    assert.deepEqual(dispatches.map(item => item.targetSessionId), [beta, beta]);
    assert.equal((await runtime.call('profiles.get', {botId: 'beta'})).canonicalSessionId, beta);
  } finally { await runtime.close(); }
});

test('the real task client distinguishes durable queue acceptance from delivery', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'direct-queue-'));
  const alpha = 'A'.repeat(26), beta = 'B'.repeat(26), calls = [];
  const execution = {instanceId: 'accepted-pi', generation: 1};
  const server = http.createServer(async (request, response) => {
    assert.equal(request.headers.authorization, 'Bearer fixture-only-key');
    let raw = ''; for await (const chunk of request) raw += chunk;
    const {method, params} = JSON.parse(raw); calls.push({method, params});
    let value;
    if (method === 'session') value = {id: params.sessionId, title: 'Chat', model: 'model', providerId: 'fixture', agentKind: 'pi',
      permissionMode: 'ask', workingDir: directory, status: 'active', source: 'bot'};
    else if (method === 'dispatch') value = {queued: true, execution};
    else if (method === 'timeline') value = {saved: true};
    else { response.writeHead(400); response.end(JSON.stringify({error: 'Unexpected ' + method})); return; }
    response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify({value}));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const client = createOneBotTaskClient({baseUrl: `http://127.0.0.1:${server.address().port}/v1`, key: 'fixture-only-key',
    event: () => {}, settle: () => assert.fail('a teammate message is not an independent task')});
  const runtime = createOneBotRuntime({directory, tasks: client});
  try {
    for (const [id, name, sessionId] of [['alpha', 'Alpha', alpha], ['beta', 'Beta', beta]]) {
      await runtime.call('profiles.create', {input: {id, name}});
      await runtime.call('sessions.bind', {botId: id, sessionId});
    }
    const sent = await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'beta', message: 'Read this when you are ready.'});
    assert.equal(sent.ok, true, JSON.stringify(sent));
    assert.equal(sent.accepted, true);
    assert.equal(sent.delivered, false, 'queued acceptance does not mean the recipient consumed the input');
    assert.equal(sent.replied, false);
    assert.equal(sent.wake_kind, 'queued');
    assert.equal(calls.filter(call => call.method === 'dispatch').length, 1);
    const anchors = calls.filter(call => call.method === 'timeline');
    assert.equal(anchors.length, 2, 'accepted messages retain the trace in both chats');
    const view = await runtime.call('tasks.directMessage.get', {threadId: anchors[0].params.agentMeta.botDirectMessage.threadId, viewerBotId: 'alpha'});
    assert.equal(view.thread.messages.length, 1);
  } finally {
    await client.close(); await runtime.close(); server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
  }
});

test('teammate references use the live roster, keep owner text and never trust supplied status', async () => {
  const {runtime, alpha} = await fixture();
  try {
    const href = 'cindy://bot/beta', link = `[An old name](${href})`, text = `${link} please review this.`;
    const reference = {kind: 'bot', start: 0, end: link.length, href, botId: 'forged-id', name: 'Forged name',
      hostSnapshot: {availability: 'ready', activity: 'working', activeDelegations: 999}};
    const prepared = await runtime.call('tasks.input.prepare', {sessionId: alpha, text, agentReferences: [reference]});
    assert.match(prepared.modelText, /Name: "Beta"/);
    assert.match(prepared.modelText, /Bot ID: "beta"/);
    assert.match(prepared.modelText, /availability=unverified/);
    assert.ok(prepared.modelText.endsWith(' please review this.'));
    assert.ok(!prepared.modelText.includes('999'));
    assert.deepEqual(prepared.agentReferences, [{kind: 'bot', start: 0, end: link.length, href, botId: 'beta', name: 'Beta'}]);
    await assert.rejects(runtime.call('tasks.input.prepare', {sessionId: alpha, text, agentReferences: [{...reference, start: 1}]}), /伙伴引用无效/);
    await assert.rejects(runtime.call('tasks.input.prepare', {sessionId: alpha, text, agentReferences: [reference, reference]}), /伙伴引用无效/);
    const plain = await runtime.call('tasks.input.prepare', {sessionId: alpha, text: 'My own words'});
    assert.deepEqual(plain, {modelText: 'My own words', quote: null, agentReferences: []});
  } finally { await runtime.close(); }
});

test('teammate messages refuse self, unknown targets and a third unanswered message', async () => {
  const {runtime, alpha, dispatches} = await fixture();
  try {
    const self = await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'alpha', message: 'Hello me'});
    assert.equal(self.errorCode, 'SELF_MESSAGE');
    const unknown = await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'nobody', message: 'Hello'});
    assert.equal(unknown.errorCode, 'TARGET_BOT_NOT_FOUND');
    assert.deepEqual(unknown.data.available_agents.map(agent => agent.id).sort(), ['alpha', 'beta']);
    for (const text of ['First', 'Second']) assert.equal((await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'beta', message: text})).ok, true);
    const third = await runtime.call('tasks.agents.send', {callerSessionId: alpha, target_id: 'beta', message: 'Third'});
    assert.equal(third.errorCode, 'WAIT_FOR_PEER');
    assert.equal(dispatches.length, 2);
  } finally { await runtime.close(); }
});
