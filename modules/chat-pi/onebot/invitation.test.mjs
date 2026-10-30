// Real Cindy invitation queue + OneBot SQLite: preparation survives a missing
// model and a worker restart, without treating a new DB wrapper as a new owner.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';
import {createOneBotRuntime} from './runtime.mjs';
import {createOneBotPaths} from './content-service.mjs';
import {withIMScope} from './im-scope.mjs';
import {writeBotProfileFolder} from './invitation-host.mjs';
import {withBotProfileLocks} from '../cindy/bots/botProfileLock.ts';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Run through build/test-onebot.mjs with an isolated output');

async function until(runtime, botId, predicate) {
  const deadline = Date.now() + 3000;
  let profile;
  do {
    profile = await runtime.call('profiles.get', {botId});
    if (predicate(profile)) return profile;
    await delay(10);
  } while (Date.now() < deadline);
  assert.fail('Invitation did not reach its expected state: ' + JSON.stringify(profile.invitation));
}

test('invitation preparation waits for an in-flight Home read before replacing its identity file', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'invitation-lock-')), paths = createOneBotPaths(directory);
  let entered, release, written = false;
  const reading = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  const held = withBotProfileLocks([paths.home('invited')], async () => { entered(); await gate; });
  await reading;
  const write = withIMScope({paths, key: 'read-owner', closed: false}, () => writeBotProfileFolder(directory, 'invited', {identitySource: 'Prepared identity'}))
    .then(() => { written = true; });
  try {
    await delay(10);
    assert.equal(written, false, 'the invitation must share the reader’s file boundary');
  } finally { release(); await held; await write; }
  assert.equal(await fs.readFile(path.join(paths.home('invited'), 'SOUL.md'), 'utf8'), 'Prepared identity');
});

test('a real invitation waits for its model, welcomes once and stays ready after reopening', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'invitation-'));
  const botId = 'invited', sessionId = 'I'.repeat(26), welcomes = [];
  let available = false, checkedModel = false, creations = 0, runtime;
  const host = {
    routeAvailable: async () => { checkedModel = true; return available; },
    createCanonicalSession: async () => {
      creations++;
      await runtime.call('sessions.bind', {botId, sessionId});
      return {canonicalSessionId: sessionId};
    },
    welcome: async input => { welcomes.push(input); return {ok: true, clientId: input.clientId}; },
  };
  runtime = createOneBotRuntime({directory, host});
  try {
    await runtime.call('profiles.create', {input: {id: botId, name: 'Reader', identitySource: 'Carefully read the owner’s books.', prepareInvitation: true, locale: 'en'}});
    await until(runtime, botId, profile => profile.invitation?.stage === 'welcome' && checkedModel);
    assert.equal(welcomes.length, 0, 'a missing model never creates a pretend greeting');
    // Defaults invalidation releases Cindy's existing waiter, not another queue.
    available = true;
    await runtime.call('defaults.changed');
    const ready = await until(runtime, botId, profile => profile.invitation?.stage === 'ready');
    assert.equal(ready.canonicalSessionId, sessionId);
    assert.equal(creations, 1);
    assert.equal(welcomes.length, 1);
    assert.equal(welcomes[0].toolsDisabled, true);
    assert.equal(welcomes[0].targetSessionId, sessionId);
    assert.match(welcomes[0].message, /Write the entire greeting in en/);
    const content = await runtime.call('profile.content.read', {botId});
    assert.equal(content.identitySource, 'Carefully read the owner’s books.');
    await runtime.close();
    runtime = createOneBotRuntime({directory, host});
    await runtime.call('profiles.list');
    await delay(30);
    assert.equal((await runtime.call('profiles.get', {botId})).invitation.stage, 'ready');
    assert.equal(welcomes.length, 1, 'completed invitations must not replay after restart');
  } finally { await runtime.close(); }
});
