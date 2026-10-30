// Cindy's per-reply learning review on real Pi transcripts: each finished reply
// is reviewed once, evidenced memories are saved and shown under that reply
// with the turn's own saves, and the working memory ledger reaches the owner's
// next message once after a native compaction.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {createOneBotRuntime} from './runtime.mjs';
import {readBotWorkingMemory, WORKING_MEMORY_REFRESH_TERMINATOR} from '../cindy/bots/botWorkingMemory.ts';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Run through build/test-onebot.mjs with an isolated output');
const meta = {api: 'openai-completions', provider: 'fixture', model: 'model',
  usage: {input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0}}};
const quote = 'always start with a one-line summary';

async function fixture() {
  const directory = await fs.mkdtemp(path.join(base, 'learning-review-'));
  const id = 'L'.repeat(26), workspace = path.join(directory, 'workspace'), sessionDirectory = path.join(directory, 'pi');
  await fs.mkdir(workspace);
  await fs.mkdir(sessionDirectory);
  const manager = SessionManager.create(workspace, sessionDirectory);
  const session = {id, sessionDirectory, sessionFile: manager.getSessionFile(), model: {provider: 'fixture', model: 'model'}};
  let plan = {memories: [], skills: []};
  const calls = [], events = [];
  const host = {
    conversation: async sessionId => sessionId === id ? session : {id: sessionId, model: session.model},
    auxiliaryText: async request => { calls.push(request); return {ok: true, text: JSON.stringify(plan)}; },
    installedSkills: async () => [],
  };
  const runtime = createOneBotRuntime({directory, host, tasks: {}, event: value => events.push(value)});
  await runtime.call('profiles.create', {input: {id: 'learner', name: 'Learner'}});
  await runtime.call('sessions.bind', {botId: 'learner', sessionId: id});
  /** One owner turn as the Host records it: the accepted input, the model text and the finished reply. */
  const turn = (inputId, text, answer) => {
    manager.appendCustomEntry('winone.input', {id: inputId, text, presentation: {text}});
    manager.appendMessage({role: 'user', content: [{type: 'text', text}], timestamp: Date.now()});
    return manager.appendMessage({role: 'assistant', content: [{type: 'text', text: answer}], ...meta, stopReason: 'stop', timestamp: Date.now()});
  };
  return {directory, id, manager, runtime, calls, events, turn, setPlan: value => { plan = value; }};
}
async function eventually(check, message) {
  for (let n = 0; n < 200; n++) {
    const value = await check();
    if (value) return value;
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw Error(message);
}
test('all six IM owner stamps survive reopening and remain bound to their exact input',async()=>{
 const f=await fixture();
 try{
  for(const channel of ['wechat','telegram','feishu','wecom','dingtalk','discord']){
   const inputId='owner-'+channel;const reply=f.turn(inputId,'My preference from '+channel,'Understood');
   await f.runtime.call('imSource.stamp',{sessionId:f.id,inputId,channel,owner:true,group:false,text:'My preference from '+channel});
   assert.equal(reply.length>0,true);
  }
  const {OneBotProfileStore}=await import('./profile-store.mjs');
  const view=new OneBotProfileStore(f.directory);
  try{for(const channel of ['wechat','telegram','feishu','wecom','dingtalk','discord']){
    const source=view.messageMeta(f.id,'owner-'+channel).imSource;
    assert.equal(source.im,channel);assert.equal(source.origin.sender.isOwner,true);assert.equal(source.userText,'My preference from '+channel);
  }}finally{view.close();}
 }finally{await f.runtime.close();}
});

test('each finished reply is reviewed once and shows what it taught the teammate', async () => {
  const f = await fixture();
  try {
    const reply = f.turn('owner-1', `From now on, ${quote} before the details.`, 'Understood: a one-line summary comes first.');
    f.setPlan({memories: [
      {type: 'feedback', name: 'summary-first', title: 'Summary first', description: 'Reply format', evidence: {messageId: 'owner-1', quote},
        body: 'Start every reply with a one-line summary.\n**Why:** the owner asked for it.\n**How to apply:** before any details.'},
      // A quote the owner never wrote is not evidence.
      {type: 'user', name: 'invented', title: 'Invented', description: 'Must not be saved', body: 'Invented preference.', evidence: {messageId: 'owner-1', quote: 'prefers long essays'}},
    ], skills: [], working: {commitments: [{text: 'Open each reply with a one-line summary', evidence: {messageId: 'owner-1', quote}}]}});
    await f.runtime.call('learning.scan');
    const receipts = await eventually(async () => (await f.runtime.call('messages.learning', {sessionId: f.id})).receipts[reply], 'the review never published a receipt');
    assert.deepEqual(receipts, [{kind: 'memory', key: 'feedback_summary-first.md', title: 'Summary first', action: 'created'}]);
    assert.equal(f.calls.length, 1);
    // The helper model reviews the owner's words and the reply, as data.
    assert.match(f.calls[0].responseInstructions, /^Review a completed teammate exchange/);
    assert.deepEqual(JSON.parse(f.calls[0].prompt).messages.map(row => [row.id, row.role]), [['owner-1', 'user'], [reply, 'assistant']]);
    assert.deepEqual((await f.runtime.call('memory.list', {botId: 'learner'})).map(row => row.filename), ['feedback_summary-first.md']);
    assert.ok(f.events.some(event => event.type === 'message-meta-changed' && event.payload?.sessionId === f.id && event.payload?.clientId === reply));
    const ledger = await eventually(() => readBotWorkingMemory(f.directory, 'learner'), 'the review never wrote the working memory');
    assert.deepEqual(ledger.commitments.map(item => [item.text, item.messageId]), [['Open each reply with a one-line summary', 'owner-1']]);
    // Rescanning, or reopening, never reviews the same reply again.
    await f.runtime.call('learning.scan');
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(f.calls.length, 1);
  } finally { await f.runtime.close(); }
});

test('a turn\'s own memory saves appear under that turn\'s reply once it finishes', async () => {
  const f = await fixture();
  try {
    await f.runtime.call('session.tool', {sessionId: f.id, requestId: 'owner-2', ownerAuthorized: true, method: 'memory.write',
      args: {input: {type: 'user', name: 'timezone', title: 'Time zone', description: 'Owner time zone', body: 'The owner works in UTC+8.'}}});
    // Nothing is shown before the turn has a finished reply.
    assert.deepEqual((await f.runtime.call('messages.learning', {sessionId: f.id})).receipts, {});
    const reply = f.turn('owner-2', 'I work in UTC+8, please remember that.', 'Saved your time zone.');
    await f.runtime.call('learning.scan');
    const receipts = await eventually(async () => (await f.runtime.call('messages.learning', {sessionId: f.id})).receipts[reply], 'the turn\'s save was never shown');
    assert.deepEqual(receipts, [{kind: 'memory', key: 'user_timezone.md', title: 'Time zone', action: 'created'}]);
    // The pending receipt is published once, not again on the next scan.
    await eventually(() => f.calls.length === 1, 'the reply was never reviewed');
    await f.runtime.call('learning.scan');
    assert.deepEqual((await f.runtime.call('messages.learning', {sessionId: f.id})).receipts, {[reply]: receipts});
    // Only a teammate conversation has receipts to read.
    await assert.rejects(f.runtime.call('messages.learning', {sessionId: 'X'.repeat(26)}), /不属于 OneBot/);
  } finally { await f.runtime.close(); }
});

test('after a native compaction the owner\'s next message carries the working memory once', async () => {
  const f = await fixture();
  try {
    const greeting = f.turn('owner-0', 'Hello', 'Hello, how can I help?');
    assert.equal((await f.runtime.call('tasks.input.prepare', {sessionId: f.id, text: 'Hello again'})).modelText, 'Hello again', 'no compaction yet');
    f.turn('owner-1', `From now on, ${quote} before the details.`, 'Understood.');
    f.setPlan({memories: [], skills: [], working: {commitments: [{text: 'Open each reply with a one-line summary', evidence: {messageId: 'owner-1', quote}}]}});
    await f.runtime.call('learning.scan');
    await eventually(() => readBotWorkingMemory(f.directory, 'learner'), 'the review never wrote the working memory');
    const kept = f.turn('owner-3', 'Draft the weekly report.', 'Here is the draft.');
    f.manager.appendCompaction('Earlier conversation summary.', kept, 120000);
    const refreshed = await f.runtime.call('tasks.input.prepare', {sessionId: f.id, text: 'Continue please'});
    assert.match(refreshed.modelText, /Working memory refresh/);
    assert.match(refreshed.modelText, /Open each reply with a one-line summary \(message_id: owner-1\)/);
    assert.ok(refreshed.modelText.endsWith(`${WORKING_MEMORY_REFRESH_TERMINATOR}\n\nContinue please`));
    // A send that never arrived does not consume it.
    assert.equal((await f.runtime.call('tasks.input.prepare', {sessionId: f.id, text: 'Continue please'})).modelText, refreshed.modelText);
    // Once a message carrying it was accepted, later messages are plain again.
    f.manager.appendCustomEntry('winone.input', {id: 'owner-4', text: refreshed.modelText, presentation: {text: 'Continue please'}});
    f.manager.appendMessage({role: 'user', content: [{type: 'text', text: refreshed.modelText}], timestamp: Date.now()});
    assert.equal((await f.runtime.call('tasks.input.prepare', {sessionId: f.id, text: 'Next'})).modelText, 'Next');
    // After a rewind above its source message, a withdrawn commitment never returns.
    f.manager.branch(greeting);
    f.manager.appendCompaction('Summary after rewind.', greeting, 130000);
    assert.equal((await f.runtime.call('tasks.input.prepare', {sessionId: f.id, text: 'Again'})).modelText, 'Again');
  } finally { await f.runtime.close(); }
});
