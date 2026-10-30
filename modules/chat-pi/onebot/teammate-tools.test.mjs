// Cindy's teammate tools (routines, creation, Todo, workbench, task digest)
// on the original access rules and real, isolated storage. The Host bridge is
// a fake that owns conversations; Pi writes the transcript the tools read.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {createOneBotRuntime} from './runtime.mjs';

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
  const directory = await fs.mkdtemp(path.join(base, 'teammate-tools-'));
  const workspace = path.join(directory, 'owner-task'), sessionDirectory = path.join(directory, 'pi');
  await fs.mkdir(workspace); await fs.mkdir(sessionDirectory);
  // An ordinary owner task in Chat: one request and the latest reply.
  const manager = SessionManager.create(workspace, sessionDirectory);
  manager.appendCustomEntry('winone.input', {id: 'owner-request', text: 'Prepare the quarterly report'});
  manager.appendMessage({role: 'user', content: [{type: 'text', text: 'Prepare the quarterly report'}], timestamp: Date.now()});
  manager.appendMessage({role: 'assistant', content: [{type: 'text', text: 'Which quarter should I use?'}], timestamp: Date.now(), stopReason: 'stop',
    api: 'openai-completions', provider: 'fixture', model: 'model', usage: {input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0}}});
  const conversations = new Map([[TASK, {found: true, id: TASK, title: 'Quarterly report', directory: workspace, sessionDirectory, sessionFile: manager.getSessionFile(),
    archived: false, store: 'chat', oneBotTask: false, route: {harness: 'pi', providerId: 'fixture', model: 'model', effort: '', fastMode: false}}],
  [CANONICAL, {found: true, id: CANONICAL, title: 'Lead', directory: workspace, sessionDirectory, sessionFile: '', archived: false, store: 'onebot', oneBotTask: false, route: null}]]);
  const activity = new Map(), sent = [], hookTests = [];
  const host = {
    activitySnapshots: async () => [...activity.values()],
    sessionMetas: async ids => ids.flatMap(id => conversations.has(id) ? [{...conversations.get(id)}] : []),
    remind: async () => ({shown: true}),
    taskConversation: async id => conversations.get(id) ?? {found: false},
    taskFacts: async id => ({found: conversations.has(id), activity: activity.get(id) ?? null,
      chain: [{harness: 'pi', providerId: 'fixture', model: 'model'}, {harness: 'pi', providerId: 'fixture', model: 'model-next'}], windows: {}}),
    sendToTask: async input => { sent.push(input); return {ok: true, wakeKind: 'queued', queuedMessageId: input.clientId}; },
    testPreRunHook: async input => { hookTests.push(input); return {preRunHook: {decision: 'run', status: 'ok', stdout: 'CHANGED'}}; },
    routeAvailable: async () => false,
    installedSkills: async () => [],
    auxiliaryText: async () => ({ok: false, reason: 'no_model'}),
  };
  const runtime = createOneBotRuntime({directory, host, routineDispatch: async schedule => ({sessionId: schedule.targetSessionId, resultText: 'DONE'})});
  await runtime.call('profiles.create', {input: {id: 'lead', name: 'Lead'}});
  await runtime.call('sessions.bind', {botId: 'lead', sessionId: CANONICAL});
  const tool = (method, args = {}, principal = 'owner') => runtime.call(method, {callerSessionId: CANONICAL, args, requestId: 'turn', ownerAuthorized: principal === 'owner', principal});
  return {runtime, directory, workspace, activity, sent, hookTests, tool};
}

test('teammate routines, creation and Todo follow the caller and the turn principal', async () => {
  const f = await fixture();
  try {
    const input = {name: 'Weekly digest', prompt: 'Summarise the week.', enabled: true, triggers: [{id: 'weekly', kind: 'cron', expression: '0 9 * * 1', timezone: 'Asia/Shanghai'}]};
    const saved = await f.tool('teammate.routines.save', input);
    assert.equal(saved.ok, true, JSON.stringify(saved));
    assert.equal(saved.result.botId, 'lead');
    const listed = await f.tool('teammate.routines.list');
    assert.ok(listed.result.some(row => row.id === saved.result.id));
    assert.ok(Array.isArray((await f.tool('teammate.routines.sources')).result));
    // Standing grants are the owner's decision (Cindy TEAMMATE_STANDING_GRANT_TOOLS).
    for (const [method, args] of [['teammate.routines.save', input], ['teammate.routines.delete', {id: saved.result.id}], ['teammate.routines.runNow', {id: saved.result.id}], ['teammate.routines.preRunHook', {script: 'process.exit(2)'}]]) {
      const refused = await f.tool(method, args, 'autonomous');
      assert.equal(refused.errorCode, 'NEEDS_OWNER_REQUEST', method);
    }
    assert.ok(Array.isArray((await f.tool('teammate.routines.history', {id: saved.result.id}, 'autonomous')).result));
    assert.equal((await f.tool('teammate.routines.delete', {id: saved.result.id})).ok, true);
    // A non-teammate conversation never owns a routine.
    const stranger = await f.runtime.call('teammate.routines.list', {callerSessionId: TASK, args: {}, principal: 'owner'});
    assert.deepEqual(stranger, {ok: false, message: '当前调用未绑定伙伴主任务'});

    // Pre-run check: the script lands in the chat folder and is self-tested by the Host.
    const hook = await f.tool('teammate.routines.preRunHook', {script: 'console.log("CINDY_PRECHECK_OK"); process.exit(0);', scheduleName: 'Repo check'});
    assert.equal(hook.ok, true, JSON.stringify(hook));
    assert.equal(hook.result.attached, false);
    assert.ok(hook.result.filePath.startsWith(path.join(f.workspace, 'scripts', 'schedule-checks')));
    assert.match(await fs.readFile(hook.result.filePath, 'utf8'), /CINDY_PRECHECK_OK/);
    assert.deepEqual(f.hookTests.map(item => [item.sessionId, item.command]), [[CANONICAL, hook.result.command]]);

    const created = await f.tool('teammate.create', {name: 'Researcher', description: 'Finds sources'});
    assert.equal(created.ok, true, JSON.stringify(created));
    assert.equal(created.action, 'created');
    assert.equal((await f.runtime.call('profiles.get', {botId: created.bot.id})).name, 'Researcher');
    assert.equal((await f.tool('teammate.create', {name: '  '})).errorCode, 'INVALID_ARGS');

    const todo = await f.tool('teammate.todos.update', {patch: {key: 'report', title: 'Send the quarterly report', outcome: 'The team has the Q3 report', origin: 'assigned'}});
    assert.equal(todo.ok, true, JSON.stringify(todo));
    const page = await f.tool('teammate.todos.list', {query: 'quarterly'});
    assert.equal(page.total, 1);
    assert.equal((await f.tool('teammate.todos.list', {key: 'report'})).items[0].id, todo.todo.id);
    const preflight = await f.tool('teammate.todos.preflight', {events: [{source: 'mail', sequence: 1, key: 'report', project: 'kept'}]});
    assert.equal(preflight.ok, true, JSON.stringify(preflight));
    assert.equal(preflight.events[0].project, 'kept');
    const recorded = await f.tool('teammate.todos.ingest', {source: 'mail', sequence: 1, skip: 'no-action'});
    assert.equal(recorded.ok, true, JSON.stringify(recorded));
  } finally { await f.runtime.close(); }
});

test('the workbench and task digest read owner tasks through the Host and continue only followed tasks', async () => {
  const f = await fixture();
  try {
    // Before the scheduler is chosen the teammate follows nothing it did not start.
    assert.equal((await f.tool('teammate.workbench.read', {task_id: TASK})).errorCode, 'TASK_NOT_RESPONSIBLE');
    await f.runtime.call('autonomy.setSettings', {patch: {schedulerBotId: 'lead'}});
    await until(() => f.runtime.call('routines.list', {botId: 'lead'}), rows => rows.some(row => row.builtin === 'follow-up'), 'follow-up never started');
    const now = Date.now();
    const snapshot = {sessionId: TASK, phase: 'running', startedAtMs: now - 120000, lastActivityAtMs: now, turnGeneration: 1, toolInFlight: false};
    f.activity.set(TASK, snapshot);
    await f.runtime.call('autonomy.activity', {snapshots: [snapshot]});
    const glance = await until(() => f.tool('teammate.workbench.get'), result => result.ok && result.workbench.working.length === 1, 'task never reached the workbench');
    assert.equal(glance.workbench.scope, 'all');
    assert.equal(glance.workbench.working[0].task_id, TASK);

    const read = await f.tool('teammate.workbench.read', {task_id: TASK});
    assert.equal(read.ok, true, JSON.stringify(read));
    assert.deepEqual(read.transcript.items.map(item => [item.role, item.text]), [['user', 'Prepare the quarterly report'], ['assistant', 'Which quarter should I use?']]);

    const digest = await f.tool('teammate.digest', {session_id: TASK});
    assert.equal(digest.ok, true, JSON.stringify(digest));
    assert.equal(digest.phase, 'running');
    assert.equal(digest.title, 'Quarterly report');
    assert.equal(digest.last_user_ask, 'Prepare the quarterly report');
    assert.equal(digest.last_assistant_line, 'Which quarter should I use?');
    assert.deepEqual(digest.current_route, {harness: 'pi', model: 'model', provider_id: 'fixture', effort: null, fast: false});
    assert.equal(digest.suggested_route, null, 'a running task gets no route advice');
    assert.ok(Buffer.byteLength(JSON.stringify(digest)) <= 2048);

    // The scheduler follows every owner task, so a wake-up may continue it.
    const continued = await f.tool('teammate.workbench.continue', {task_id: TASK, message: 'Use Q3 and finish the report.'}, 'autonomous');
    assert.equal(continued.ok, true, JSON.stringify(continued));
    assert.equal(continued.delivery, 'queued');
    assert.deepEqual(f.sent.map(item => [item.sessionId, item.dispatcherSessionId, item.message]), [[TASK, CANONICAL, 'Use Q3 and finish the report.']]);
    // A teammate's own chat is never a workbench target.
    assert.equal((await f.tool('teammate.workbench.continue', {task_id: CANONICAL, message: 'x'})).errorCode, 'TASK_NOT_ACCESSIBLE');

    // Taking responsibility needs the owner's request; releasing does not.
    assert.equal((await f.tool('teammate.workbench.responsibility', {task_id: TASK, responsible: true}, 'autonomous')).errorCode, 'NEEDS_OWNER_REQUEST');
    assert.equal((await f.tool('teammate.workbench.responsibility', {task_id: TASK, responsible: true})).responsible, true);
    assert.equal((await f.tool('teammate.workbench.responsibility', {task_id: TASK, responsible: false}, 'autonomous')).responsible, false);

    // The owner's own workbench view applies the same rule.
    const owner = await f.runtime.call('workbench.readTask', {botId: 'lead', taskId: TASK});
    assert.equal(owner.ok, true, JSON.stringify(owner));
  } finally { await f.runtime.close(); }
});
