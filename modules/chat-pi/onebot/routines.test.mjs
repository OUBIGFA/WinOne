// The original Cindy routine engine and scheduler on real, isolated storage.
// Only the native Pi dispatch boundary is supplied by the test.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Use build/test-onebot.mjs with an isolated output');
const input = {name: 'Daily report', prompt: 'Read the project and report progress.', enabled: true,
  triggers: [{id: 'daily', kind: 'cron', expression: '0 9 * * *', timezone: 'Asia/Shanghai'}]};

async function fixture(dispatch) {
  const directory = await fs.mkdtemp(path.join(base, 'routines-'));
  const calls = [];
  const options = {directory, routineDispatch: async (schedule, context) => {
    calls.push({schedule, context});
    if (dispatch) return dispatch(schedule, context);
    return {sessionId: schedule.targetSessionId, resultText: 'REPORT_DONE'};
  }};
  const runtime = createOneBotRuntime(options);
  for (const id of ['reporter', 'other']) await runtime.call('profiles.create', {input: {id, name: id}});
  await runtime.call('sessions.bind', {botId: 'reporter', sessionId: 'R'.repeat(26)});
  return {runtime, options, calls};
}
// Every active teammate also gets Cindy's follow-up builtin; it lands once
// the follow-up runtime has read task ownership after the engine starts.
async function builtins(runtime, botId) {
  for (let n = 0; n < 100; n++) {
    const rows = await runtime.call('routines.list', {botId});
    if (rows.some(row => row.builtin === 'follow-up')) return rows;
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw Error('The follow-up builtin never landed');
}
async function completed(runtime, id) {
  for (let n = 0; n < 100; n++) {
    const rows = await runtime.call('routines.history', {botId: 'reporter', id});
    if (rows[0]?.finishedAt) return rows[0];
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw Error('Routine never produced a durable result');
}

test('routine definitions survive reopen, validate upstream triggers and isolate teammate history', async () => {
  const f = await fixture();
  let runtime = f.runtime;
  try {
    const defaults = await builtins(runtime, 'reporter');
    assert.deepEqual(defaults.map(item => item.builtin).sort(), ['follow-up', 'nightly-learning']);
    const row = await runtime.call('routines.save', {botId: 'reporter', input});
    assert.equal(row.botId, 'reporter');
    assert.equal(row.revision, 1);
    await assert.rejects(runtime.call('routines.save', {botId: 'reporter', input: {...input, triggers: [{id: 'bad', kind: 'interval', intervalMs: 5}]}}), /one minute/);
    await assert.rejects(runtime.call('routines.remove', {botId: 'other', id: row.id}), /not found/i);
    await assert.rejects(runtime.call('routines.history', {botId: 'other', id: row.id}), /not found/i);
    await runtime.close();
    runtime = createOneBotRuntime(f.options);
    assert.deepEqual((await runtime.call('routines.list', {botId: 'reporter'})).map(item => item.id).sort(), [...defaults.map(item => item.id), row.id].sort());
    assert.deepEqual((await builtins(runtime, 'other')).map(item => item.builtin).sort(), ['follow-up', 'nightly-learning']);
    await runtime.call('routines.remove', {botId: 'reporter', id: row.id});
    assert.deepEqual((await runtime.call('routines.list', {botId: 'reporter'})).map(item => item.id).sort(), defaults.map(item => item.id).sort());
  } finally { await runtime.close(); }
});

test('a one-shot routine fires once and a paused teammate rejects manual execution', async () => {
  const f = await fixture();
  let runtime = f.runtime;
  try {
    const row = await runtime.call('routines.save', {botId: 'reporter', input: {...input, triggers: [{id: 'once', kind: 'once', at: Date.now() + 300}]}});
    assert.equal((await completed(runtime, row.id)).status, 'success');
    assert.equal(f.calls.length, 1);
    await runtime.close();
    runtime = createOneBotRuntime(f.options);
    await runtime.call('routines.list', {botId: 'reporter'});
    await new Promise(resolve => setTimeout(resolve, 1100));
    assert.equal(f.calls.length, 1, 'a consumed one-shot ran again after reopen');
    const profile = await runtime.call('profiles.get', {botId: 'reporter'});
    await runtime.call('profiles.update', {botId: 'reporter', input: {enabled: false, expectedVersion: profile.currentVersion}});
    await assert.rejects(runtime.call('routines.runNow', {botId: 'reporter', id: row.id}), /paused/i);
  } finally { await runtime.close(); }
});

test('closing an active routine confirms cancellation before releasing its storage', async () => {
  let started;
  const active = new Promise(resolve => { started = resolve; });
  let cancelled = false;
  const f = await fixture(async (_schedule, context) => {
    started();
    await new Promise((resolve, reject) => {
      const abort = () => { cancelled = true; reject(new DOMException('aborted', 'AbortError')); };
      if (context.signal.aborted) abort();
      else context.signal.addEventListener('abort', abort, {once: true});
    });
  });
  try {
    const row = await f.runtime.call('routines.save', {botId: 'reporter', input});
    await f.runtime.call('routines.runNow', {botId: 'reporter', id: row.id});
    await active;
    await f.runtime.close();
    assert.equal(cancelled, true);
    const reopened = createOneBotRuntime(f.options);
    try {
      const run = (await reopened.call('routines.history', {botId: 'reporter', id: row.id}))[0];
      assert.ok(['cancelled', 'interrupted'].includes(run.status), JSON.stringify(run));
    } finally { await reopened.close(); }
  } finally { await f.runtime.close(); }
});

test('a corrupt routine definition reports the error and still releases the OneBot owner lock', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'routines-corrupt-'));
  await fs.mkdir(path.join(directory, 'routines'));
  await fs.writeFile(path.join(directory, 'routines', 'routines.json'), '{broken', 'utf8');
  const runtime = createOneBotRuntime({directory});
  await assert.rejects(runtime.call('routines.sources'), /JSON/);
  await assert.rejects(runtime.close(), /JSON/);
  const reopened = createOneBotRuntime({directory});
  await reopened.close();
});

test('run-now uses the teammate canonical Pi chat, preserves hooks and records the real scheduler result', async () => {
  const {runtime, calls} = await fixture();
  try {
    const hook = {command: 'node approved-check.mjs', timeoutMs: 10000};
    const row = await runtime.call('routines.save', {botId: 'reporter', input: {...input, preRunHook: hook}});
    await runtime.call('routines.runNow', {botId: 'reporter', id: row.id, targetSessionId: 'CLIENT_CANNOT_CHOOSE'});
    const run = await completed(runtime, row.id);
    assert.equal(run.status, 'success');
    assert.equal(run.resultText, 'REPORT_DONE');
    assert.ok(run.scheduleRunId);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].schedule.targetSessionId, 'R'.repeat(26));
    assert.equal(calls[0].schedule.agentKind, 'pi');
    assert.equal(calls[0].schedule.source, 'bot');
    assert.deepEqual(calls[0].schedule.preRunHook, hook);
    assert.match(calls[0].schedule.prompt, /Read the project/);
    assert.equal((await runtime.call('routines.list', {botId: 'reporter'}))[0].activity, undefined);
  } finally { await runtime.close(); }
});
