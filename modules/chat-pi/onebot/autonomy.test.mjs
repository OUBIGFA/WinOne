// Cindy's original teammate follow-up (task watch, runtime, follow-up routine)
// on real, isolated storage. The Host bridge is a fake that reports
// conversation activity and metadata; routine dispatch is captured.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';
import {islandSnapshot, pendingInteractions} from './autonomy.mjs';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Use build/test-onebot.mjs with an isolated output');
const CANONICAL = 'C'.repeat(26), TASK = 'T'.repeat(26);

async function until(read, check, message) {
  for (let n = 0; n < 200; n++) {
    const value = await read();
    if (check(value)) return value;
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  throw Error(message);
}

async function fixture() {
  const directory = await fs.mkdtemp(path.join(base, 'autonomy-'));
  const activity = new Map(), metas = new Map(), reminders = [], dispatched = [];
  const host = {
    activitySnapshots: async () => [...activity.values()],
    sessionMetas: async ids => ids.flatMap(id => metas.has(id) ? [metas.get(id)] : []),
    remind: async input => { reminders.push(input); return {shown: true}; },
  };
  const runtime = createOneBotRuntime({directory, host, routineDispatch: async (schedule, context) => {
    dispatched.push({schedule, context});
    return {sessionId: schedule.targetSessionId, resultText: 'FOLLOWED_UP'};
  }});
  await runtime.call('profiles.create', {input: {id: 'lead', name: 'Lead'}});
  await runtime.call('sessions.bind', {botId: 'lead', sessionId: CANONICAL});
  metas.set(CANONICAL, {id: CANONICAL, title: 'Lead', workingDir: directory, archived: false, store: 'onebot'});
  return {runtime, directory, activity, metas, reminders, dispatched};
}

test('Host activity becomes Cindy island snapshots with summarized interactions', () => {
  assert.equal(islandSnapshot({sessionId: 'a', phase: 'idle'}), null);
  assert.equal(islandSnapshot({sessionId: 'a', phase: 'interrupted'}), null);
  const running = islandSnapshot({sessionId: 'a', phase: 'running', startedAtMs: 10, lastActivityAtMs: 20, turnGeneration: 2, toolInFlight: true});
  assert.equal(running.phase, 'running');
  assert.equal(running.currentTurnActive, true);
  assert.equal(running.attention, false);
  assert.equal(running.turnGeneration, 2);
  assert.equal(running.startedAtMs, 10);
  const question = {sessionId: 'b', phase: 'needs-interaction', interactions: [{id: 'q1', kind: 'question', tool: 'ask_user_question', questions: [{question: 'Overwrite the report?'}]}]};
  assert.equal(islandSnapshot(question).interactionKind, 'ask_user_question');
  assert.deepEqual(pendingInteractions(question), [{kind: 'ask_user_question', requestId: 'q1', questions: [{question: 'Overwrite the report?'}]}]);
  // Permission arguments never reach the follow-up; only the tool name does.
  const permission = {sessionId: 'c', phase: 'needs-interaction', interactions: [{id: 'p1', tool: 'bash', args: {command: 'secret'}}]};
  assert.equal(islandSnapshot(permission).interactionKind, 'permission');
  assert.deepEqual(pendingInteractions(permission), [{kind: 'permission', requestId: 'p1', toolName: 'bash'}]);
});

test('the scheduler teammate follows a finished owner task and the daily todo check toggles its builtin', async () => {
  const f = await fixture();
  try {
    assert.deepEqual(await f.runtime.call('autonomy.settings'), {schedulerBotId: null, isCustomized: false, defaults: {schedulerBotId: null}});
    await assert.rejects(f.runtime.call('autonomy.setSettings', {patch: {schedulerBotId: 'missing'}}), /not found/i);
    await assert.rejects(f.runtime.call('autonomy.setSettings', {patch: {schedulerBotId: 7}}), /schedulerBotId/);
    assert.equal((await f.runtime.call('autonomy.setSettings', {patch: {schedulerBotId: 'lead'}})).schedulerBotId, 'lead');
    // Starting the routine engine starts the follow-up and lands its builtin rule.
    await until(() => f.runtime.call('routines.list', {botId: 'lead'}), rows => rows.some(row => row.builtin === 'follow-up'), 'follow-up builtin never landed');
    await until(() => f.runtime.call('autonomy.glance', {botId: 'lead'}).catch(() => null), Boolean, 'autonomy never became ready');

    // An owner task runs, then completes in the same turn: the scheduler hears it.
    const now = Date.now();
    f.metas.set(TASK, {id: TASK, title: 'Build the weekly report', workingDir: f.directory, archived: false, store: 'chat'});
    const running = {sessionId: TASK, phase: 'running', startedAtMs: now, lastActivityAtMs: now, turnGeneration: 1, toolInFlight: false};
    f.activity.set(TASK, running);
    await f.runtime.call('autonomy.activity', {snapshots: [running]});
    await until(() => f.runtime.call('autonomy.glance', {botId: 'lead'}), glance => JSON.stringify(glance).includes(TASK), 'running task never reached the glance');
    const completed = {...running, phase: 'completed', lastActivityAtMs: now + 1};
    f.activity.set(TASK, completed);
    await f.runtime.call('autonomy.activity', {snapshots: [completed]});
    const [run] = await until(async () => f.dispatched, rows => rows.length > 0, 'the follow-up routine never ran');
    assert.equal(run.schedule.targetSessionId, CANONICAL);
    assert.match(run.schedule.prompt, /"type":\s*"finished"/);
    assert.ok(run.schedule.prompt.includes(TASK));
    assert.deepEqual(f.reminders, [], 'a followed task must not also remind the owner');

    assert.equal(await f.runtime.call('todos.dailyCheck', {botId: 'lead'}), false);
    assert.equal(await f.runtime.call('todos.dailyCheck', {botId: 'lead', enabled: true}), true);
    const rule = (await f.runtime.call('routines.list', {botId: 'lead'})).find(row => row.builtin === 'todo-check');
    assert.equal(rule.enabled, true);
    assert.equal(rule.triggers[0].expression, '0 9 * * 1-5');
    assert.equal(await f.runtime.call('todos.dailyCheck', {botId: 'lead', enabled: false}), false);
    assert.equal(await f.runtime.call('todos.dailyCheck', {botId: 'lead'}), false);
    assert.equal((await f.runtime.call('routines.list', {botId: 'lead'})).find(row => row.builtin === 'todo-check').id, rule.id, 'turning it off only pauses the same rule');
    await assert.rejects(f.runtime.call('todos.dailyCheck', {botId: 'lead', enabled: 'yes'}), /enabled/i);

    assert.deepEqual(await f.runtime.call('autonomy.resetSettings'), {schedulerBotId: null, isCustomized: false, defaults: {schedulerBotId: null}});
  } finally { await f.runtime.close(); }
});

test('without a scheduler or a delegation owner tasks stay unobserved', async () => {
  const f = await fixture();
  try {
    // The owner turned the scheduler off: their own tasks wake no teammate.
    await f.runtime.call('autonomy.setSettings', {patch: {schedulerBotId: null}});
    await until(() => f.runtime.call('routines.list', {botId: 'lead'}), rows => rows.some(row => row.builtin === 'follow-up'), 'follow-up builtin never landed');
    const asking = {sessionId: TASK, phase: 'needs-interaction', startedAtMs: Date.now(), lastActivityAtMs: Date.now(), turnGeneration: 1, toolInFlight: false,
      interactions: [{id: 'q1', kind: 'question', questions: [{question: 'Which folder?'}]}]};
    f.metas.set(TASK, {id: TASK, title: 'Sort photos', workingDir: f.directory, archived: false, store: 'chat'});
    f.activity.set(TASK, asking);
    await f.runtime.call('autonomy.activity', {snapshots: [asking]});
    await new Promise(resolve => setTimeout(resolve, 200));
    assert.deepEqual(f.reminders, []);
    assert.deepEqual(f.dispatched, []);
  } finally { await f.runtime.close(); }
});
