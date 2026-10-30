// Real nightly storage, original learning rules and memory conflict protection.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';
import {SessionManager} from '@earendil-works/pi-coding-agent';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Use build/test-onebot.mjs with an isolated output');
async function fixture() {
  const directory = await fs.mkdtemp(path.join(base, 'nightly-'));
  let plan = {};
  const calls = [];
  const sessions = new Map();
  const host = {
    conversation: async id => sessions.get(id) ?? ({id, model: {provider: 'fixture', model: 'model'}}),
    auxiliaryText: async request => { calls.push(request); return {ok: true, text: JSON.stringify(plan)}; },
    installedSkills: async () => [],
  };
  const options = {directory, host};
  const runtime = createOneBotRuntime(options);
  await runtime.call('profiles.create', {input: {id: 'learner', name: 'Learner'}});
  await runtime.call('sessions.bind', {botId: 'learner', sessionId: 'N'.repeat(26)});
  return {runtime, options, calls, sessions, setPlan: value => { plan = value; }};
}
async function settled(runtime) {
  for (let n = 0; n < 100; n++) {
    const view = await runtime.call('routines.nightly.get', {botId: 'learner'});
    if (view.lastRun && !view.running) return view;
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw Error('Nightly learning never wrote a result');
}

test('nightly learning keeps its original builtin, frequency and pause preference after reopen', async () => {
  const f = await fixture();
  let runtime = f.runtime;
  try {
    const view = await runtime.call('routines.nightly.get', {botId: 'learner'});
    assert.equal(view.enabled, true);
    assert.equal(view.frequency, 'daily');
    assert.equal(view.pendingTurns, 0);
    await runtime.call('routines.nightly.setEnabled', {botId: 'learner', enabled: false});
    await runtime.call('routines.nightly.setFrequency', {botId: 'learner', frequency: 'weekly'});
    const row = (await runtime.call('routines.list', {botId: 'learner'})).find(item => item.builtin === 'nightly-learning');
    assert.equal(row.enabled, false);
    assert.equal(row.triggers[0].expression, '30 3 * * 1');
    await assert.rejects(runtime.call('routines.remove', {botId: 'learner', id: row.id}), /BUILTIN_ROUTINE/);
    await runtime.close();
    runtime = createOneBotRuntime(f.options);
    const reopened = await runtime.call('routines.nightly.get', {botId: 'learner'});
    assert.equal(reopened.enabled, false);
    assert.equal(reopened.frequency, 'weekly');
    assert.equal(f.calls.length, 0, 'reading settings must not run the helper model');
  } finally { await runtime.close(); }
});

test('nightly learning counts only real owner replies and replay after reopen never counts twice', async () => {
  const f = await fixture();
  let runtime = f.runtime;
  const id = 'N'.repeat(26), workspace = path.join(f.options.directory, 'workspace'), sessionDirectory = path.join(f.options.directory, 'pi');
  await fs.mkdir(workspace);
  await fs.mkdir(sessionDirectory);
  const manager = SessionManager.create(workspace, sessionDirectory);
  const append = (inputId, source, userText, answer) => {
    manager.appendCustomEntry('winone.input', {id: inputId, text: userText, hostDispatched: source === 'automation', presentation: source === 'hidden' ? {hidden: true} : undefined});
    manager.appendMessage({role: 'user', content: [{type: 'text', text: userText}], timestamp: Date.now()});
    return manager.appendMessage({role: 'assistant', content: [{type: 'text', text: answer}], timestamp: Date.now(), stopReason: 'stop',
      api: 'openai-completions', provider: 'fixture', model: 'model', usage: {input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0}}});
  };
  append('owner-turn', 'owner', 'Please keep all replies concise and use Chinese.', 'Understood.');
  append('scheduled-turn', 'automation', 'Invented automatic preference that must not be learnt.', 'Scheduled result.');
  append('welcome', 'hidden', '[UI_ACTION_TRIGGER] welcome', 'Hello.');
  f.sessions.set(id, {id, sessionDirectory, sessionFile: manager.getSessionFile(), model: {provider: 'fixture', model: 'model'}});
  try {
    await runtime.call('learning.scan');
    assert.equal((await runtime.call('routines.nightly.get', {botId: 'learner'})).pendingTurns, 1);
    await runtime.call('learning.scan');
    assert.equal((await runtime.call('routines.nightly.get', {botId: 'learner'})).pendingTurns, 1);
    await runtime.close();
    runtime = createOneBotRuntime(f.options);
    await runtime.call('learning.scan');
    assert.equal((await runtime.call('routines.nightly.get', {botId: 'learner'})).pendingTurns, 1);
    f.setPlan({memories: [{op: 'create', type: 'user', name: 'language', title: 'Language', description: 'Preferred reply language',
      body: 'Use concise Chinese replies.', reason: 'Explicit owner request', evidence: {messageId: 'owner-turn', quote: 'keep all replies concise and use Chinese'}}]});
    await runtime.call('routines.nightly.runNow', {botId: 'learner'});
    const view = await settled(runtime);
    assert.equal(view.pendingTurns, 0);
    assert.equal(view.lastRun.turns, 1);
    assert.equal(view.lastRun.outcome, 'learned');
    assert.equal((await runtime.call('memory.read', {botId: 'learner', filename: 'user_language.md'})).body, 'Use concise Chinese replies.');
  } finally { await runtime.close(); }
});

test('nightly consolidation uses the auxiliary model, rejects fabricated evidence and supports undo and redo', async () => {
  const f = await fixture();
  try {
    await f.runtime.call('memory.write', {botId: 'learner', input: {type: 'user', name: 'style', title: 'Style', description: 'Writing style', body: 'Keep replies concise. Keep replies concise.'}});
    f.setPlan({memories: [
      {op: 'update', filename: 'user_style.md', title: 'Style', description: 'Writing style', body: 'Keep replies concise.', reason: 'Remove repeated wording'},
      {op: 'create', type: 'user', name: 'unfounded', title: 'Unfounded', description: 'Must not be written', body: 'Invented preference.', reason: 'Not supported', evidence: {messageId: 'fake', quote: 'This message does not exist'}},
    ]});
    await f.runtime.call('routines.nightly.runNow', {botId: 'learner'});
    const view = await settled(f.runtime);
    assert.equal(view.lastRun.outcome, 'learned');
    assert.equal(view.lastRun.items.length, 1);
    assert.equal((await f.runtime.call('memory.list', {botId: 'learner'})).length, 1);
    const read = () => f.runtime.call('memory.read', {botId: 'learner', filename: 'user_style.md'});
    assert.equal((await read()).body, 'Keep replies concise.');
    assert.equal(f.calls.length, 1);
    const action = {botId: 'learner', runId: view.lastRun.id, itemId: view.lastRun.items[0].id};
    await f.runtime.call('routines.nightly.revert', {...action, redo: false});
    assert.equal((await read()).body, 'Keep replies concise. Keep replies concise.');
    await f.runtime.call('routines.nightly.revert', {...action, redo: true});
    assert.equal((await read()).body, 'Keep replies concise.');
  } finally { await f.runtime.close(); }
});

test('closing nightly learning cancels the real helper boundary and permits reopening the same owner', async () => {
  const f = await fixture();
  let started, signal, cancelled = false;
  const active = new Promise(resolve => { started = resolve; });
  f.options.host.auxiliaryText = async request => {
    signal = request.signal; started();
    if (!signal) return {ok: false, reason: 'missing-cancellation'};
    return new Promise(resolve => {
      const abort = () => { cancelled = true; resolve({ok: false, reason: 'aborted'}); };
      if (signal.aborted) abort(); else signal.addEventListener('abort', abort, {once: true});
    });
  };
  try {
    await f.runtime.call('routines.nightly.runNow', {botId: 'learner'});
    await active;
    assert.ok(signal, 'the helper did not receive the routine cancellation signal');
    await f.runtime.close();
    assert.equal(cancelled, true);
    const reopened = createOneBotRuntime(f.options);
    try { assert.equal((await reopened.call('routines.nightly.get', {botId: 'learner'})).lastRun.outcome, 'failed'); }
    finally { await reopened.close(); }
  } finally { await f.runtime.close(); }
});

test('manual nightly runs while its schedule is paused and repeated view reads never cancel it', async () => {
  const f = await fixture();
  let started, release, signal;
  const active = new Promise(resolve => { started = resolve; });
  f.options.host.auxiliaryText = request => {
    f.calls.push(request); signal = request.signal; started();
    return new Promise(resolve => {
      release = () => resolve({ok: true, text: '{}'});
      signal.addEventListener('abort', () => resolve({ok: false, reason: 'aborted'}), {once: true});
    });
  };
  try {
    await f.runtime.call('routines.nightly.setEnabled', {botId: 'learner', enabled: false});
    await f.runtime.call('routines.nightly.runNow', {botId: 'learner'});
    await active;
    await f.runtime.call('routines.nightly.get', {botId: 'learner'});
    assert.equal(signal.aborted, false, 'reading a paused schedule aborted its explicit manual run');
    release();
    const view = await settled(f.runtime);
    assert.equal(view.enabled, false);
    assert.equal(view.lastRun.outcome, 'nothing');
    assert.equal(f.calls.length, 1);
  } finally { release?.(); await f.runtime.close(); }
});
