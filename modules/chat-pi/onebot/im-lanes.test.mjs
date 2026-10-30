// Cindy's original IM lane operations on real connection records and Pi
// transcripts. Native control callbacks are explicit; they never touch the
// developer's channels, accounts or installed application.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {createOneBotRuntime} from './runtime.mjs';
import {createOneBotPaths} from './content-service.mjs';
import {withIMScope} from './im-scope.mjs';
import {addBotImConnectionRecord, updateBotImConnectionRecord} from '../cindy/bots/botImConnectionStore.ts';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Run through build/test-onebot.mjs with an isolated output');
const answer = {role: 'assistant', content: [{type: 'text', text: 'Received'}], api: 'openai-completions', provider: 'fixture', model: 'fixture',
  stopReason: 'stop', usage: {input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0}}, timestamp: 2};

async function fixture() {
  const directory = await fs.mkdtemp(path.join(base, 'im-lanes-'));
  const rows = new Map(), managers = new Map(), resets = [], archives = [];
  const ids = Object.fromEntries(['main', 'group', 'topic', 'other', 'guest', 'foreign'].map((name, i) => [name, String.fromCharCode(65 + i).repeat(26)]));
  const host = {
    laneSessions: async () => ({conversations: [...rows.values()], activity: {[ids.group]: {phase: 'running'}}}),
    restartLane: async id => { resets.push(id); managers.get(id).appendCustomEntry('winone.context-reset', {}); },
    archiveLane: async id => { archives.push(id); rows.get(id).archived = true; },
  };
  const runtime = createOneBotRuntime({directory, host});
  for (const id of ['alpha', 'beta']) await runtime.call('profiles.create', {input: {id, name: id}});
  for (const [name, botId, routeKey] of [
    ['main', 'alpha', 'canonical'], ['group', 'alpha', 'im:telegram:777:g:-1001'],
    ['topic', 'alpha', 'im:telegram:777:g:-1001:t:42'], ['other', 'alpha', 'im:telegram:7770:g:-1001'],
    ['guest', 'alpha', 'im:telegram:777:dm:1234'], ['foreign', 'beta', 'im:telegram:777:g:-1001'],
  ]) {
    const id = ids[name], sessionDirectory = path.join(directory, 'history', id);
    await fs.mkdir(sessionDirectory, {recursive: true});
    const manager = SessionManager.create(directory, sessionDirectory);
    manager.appendMessage({role: 'user', content: name + ' owner text', timestamp: 1});
    manager.appendMessage(answer); managers.set(id, manager);
    rows.set(id, {id, directory, title: 'telegram · Study group', sessionDirectory, sessionFile: manager.getSessionFile(), archived: false});
    await runtime.call('sessions.bind', {botId, sessionId: id, routeKey, role: name === 'main' ? 'canonical' : 'channel'});
  }
  const scope = {paths: createOneBotPaths(directory), key: 'im-lane-fixture', closed: false};
  const connection = await withIMScope(scope, async () => {
    const row = await addBotImConnectionRecord({botId: 'alpha', channel: 'telegram'});
    return updateBotImConnectionRecord(row.id, {botContextId: '777'});
  });
  return {runtime, rows, managers, ids, resets, archives, connection};
}

test('IM lane lists and controls stay inside one connection and retain the shared owner chat', async () => {
  const {runtime, ids, resets, archives, connection} = await fixture();
  try {
    const list = await runtime.call('imLanes.list', {id: connection.id});
    assert.equal(list.connectionId, connection.id);
    assert.equal(list.lanes[0].kind, 'owner-dm');
    assert.deepEqual(list.lanes.map(row => row.sessionId).sort(), [ids.main, ids.group, ids.topic, ids.guest].sort());
    assert.equal(list.lanes.find(row => row.sessionId === ids.group).state, 'running');
    assert.equal(list.lanes.find(row => row.sessionId === ids.topic).threadId, '42');
    const groups = await runtime.call('imLanes.groups', {id: connection.id});
    assert.deepEqual(groups.groups, [{chatId: '-1001', chatName: 'Study group', activation: 'mention'}]);
    for (const sessionId of [ids.other, ids.foreign]) {
      await assert.rejects(runtime.call('imLanes.reset', {id: connection.id, sessionId}), /LANE_NOT_FOUND/);
      await assert.rejects(runtime.call('imLanes.archive', {id: connection.id, sessionId}), /LANE_NOT_FOUND/);
    }
    assert.equal(resets.length, 0);
    const retired = await runtime.call('imLanes.retire', {id: connection.id});
    assert.deepEqual(retired.archivedSessionIds.sort(), [ids.group, ids.topic, ids.guest].sort());
    assert.deepEqual(archives.sort(), [ids.group, ids.topic, ids.guest].sort());
    assert.deepEqual((await runtime.call('imLanes.list', {id: connection.id})).lanes.map(row => row.sessionId), [ids.main]);
  } finally { await runtime.close(); }
});

test('archiving an IM list row does not stop its session and a later message restores the row', async () => {
  const {runtime, ids, rows, managers, archives, connection} = await fixture();
  try {
    const archived = await runtime.call('imLanes.archive', {id: connection.id, sessionId: ids.group});
    assert.ok(!archived.lanes.some(row => row.sessionId === ids.group));
    assert.equal(rows.get(ids.group).archived, false);
    assert.equal(archives.length, 0, 'list archive is not session archive');
    assert.ok(!((await runtime.call('imLanes.list', {id: connection.id})).lanes.some(row => row.sessionId === ids.group)));
    managers.get(ids.group).appendMessage({...answer, content: [{type: 'text', text: 'New activity'}]});
    assert.ok((await runtime.call('imLanes.list', {id: connection.id})).lanes.some(row => row.sessionId === ids.group));
  } finally { await runtime.close(); }
});

test('lane restart clears the context count but preserves transcript and ownership', async () => {
  const {runtime, ids, managers, resets, connection} = await fixture();
  try {
    const before = managers.get(ids.topic).getBranch().filter(row => row.type === 'message').map(row => row.id);
    const value = await runtime.call('imLanes.reset', {id: connection.id, sessionId: ids.topic});
    assert.equal(value.lanes.find(row => row.sessionId === ids.topic).contextMessageCount, 0);
    assert.deepEqual(resets, [ids.topic]);
    assert.deepEqual(managers.get(ids.topic).getBranch().filter(row => row.type === 'message').map(row => row.id), before);
    assert.equal((await runtime.call('sessions.lookup', {sessionId: ids.topic})).botId, 'alpha');
  } finally { await runtime.close(); }
});
