// Host-side behavior behind Cindy's automation page (b7e46bc): sidebar unread
// index, mark-all-read, project per-entry edits with reconcile events, AI
// pre-run check generation, the permission default change and Pi templates.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {createAutomationRuntime} from './automations.mjs';
import {ScheduleStore} from './scheduler-store.mjs';

const definition = {name: 'Check project', prompt: 'Inspect current files', kind: 'cron', cronExpr: '0 9 * * *', timezone: 'Asia/Shanghai', recurring: true, manual: true, agentKind: 'pi', modelAgentKind: 'pi', model: 'fixture', providerId: 'service', workspaceKind: 'dialogue', useWorktree: false, notify: {desktop: false, feishu: false}};
const directory = () => fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY ?? os.tmpdir(), 'automation-cindy-'));

test('existing fresh tasks keep ask while new tasks and bound tasks store no permission', async () => {
  const root = await directory();
  const seed = new ScheduleStore(path.join(root, 'automations.db'));
  const now = Date.now();
  await seed.insert({...definition, id: 'legacy-fresh', status: 'paused', createdAt: now, updatedAt: now});
  await seed.insert({...definition, id: 'legacy-bound', targetSessionId: 'conversation', status: 'paused', createdAt: now, updatedAt: now});
  await seed.insert({...definition, id: 'legacy-full', permissionMode: 'bypassPermissions', status: 'paused', createdAt: now, updatedAt: now});
  seed.close();
  const runtime = await createAutomationRuntime({directory: root, passive: true, dispatch: async () => ({sessionId: 'fixture'})});
  try {
    assert.equal((await runtime.call('get', {id: 'legacy-fresh'})).permissionMode, 'ask');
    assert.equal((await runtime.call('get', {id: 'legacy-bound'})).permissionMode, undefined);
    assert.equal((await runtime.call('get', {id: 'legacy-full'})).permissionMode, 'bypassPermissions');
    const created = await runtime.call('create', {input: definition});
    assert.equal(created.permissionMode, undefined);
  } finally { await runtime.close(); }
  // The migration runs once: a later permission-free row is not rewritten.
  const reopened = await createAutomationRuntime({directory: root, passive: true, dispatch: async () => ({sessionId: 'fixture'})});
  try {
    assert.ok((await reopened.call('list')).filter((schedule) => schedule.name === definition.name && schedule.permissionMode === undefined && !schedule.targetSessionId).length >= 1);
  } finally { await reopened.close(); }
});

test('unread index lists terminal runs per session and mark-all-read clears them with one event', async () => {
  const events = [];
  const runtime = await createAutomationRuntime({directory: await directory(), passive: true, event: (event) => events.push(event), dispatch: async () => ({sessionId: 'conversation-a', resultText: 'done'})});
  try {
    const schedule = await runtime.call('create', {input: definition});
    await runtime.call('run', {id: schedule.id});
    await runtime.settled();
    const index = await runtime.call('sidebarIndexRuns');
    assert.deepEqual(index.inflightRunIds, []);
    const [entry] = index.runs;
    assert.equal(entry.scheduleId, schedule.id);
    assert.equal(entry.sessionId, 'conversation-a');
    assert.equal(entry.status, 'success');
    assert.equal(entry.readAt, undefined);
    assert.equal(await runtime.call('inflightCount', {id: schedule.id}), 0);
    assert.equal(await runtime.call('markAllRead'), 1);
    assert.ok(events.some((event) => event.type === 'all-read'));
    assert.ok((await runtime.call('sidebarIndexRuns')).runs.every((run) => run.readAt !== undefined));
    assert.equal(await runtime.call('markAllRead'), 0);
  } finally { await runtime.close(); }
});

test('project entries upsert and remove by id and report reconcile changes', async () => {
  const events = [];
  const project = await directory();
  const runtime = await createAutomationRuntime({directory: await directory(), passive: true, event: (event) => events.push(event), dispatch: async () => ({sessionId: 'fixture'})});
  try {
    const config = {id: 'auto-fixture', name: 'Project check', prompt: 'Review', cronExpr: '0 9 * * *', timezone: 'UTC', recurring: true, manual: true};
    await runtime.call('project.upsertSchedule', {workingDir: project, config});
    let [schedule] = await runtime.call('list');
    assert.equal(schedule.source, 'project');
    assert.equal(schedule.projectConfigId, 'auto-fixture');
    const first = events.findLast((event) => event.type === 'reconciled');
    assert.deepEqual({inserted: first.inserted, isFirstTime: first.isFirstTime, hashChanged: first.hashChanged}, {inserted: 1, isFirstTime: true, hashChanged: true});
    await runtime.call('project.reconcile', {workingDir: project});
    assert.equal(events.findLast((event) => event.type === 'reconciled').hashChanged, false);
    await runtime.call('project.upsertSchedule', {workingDir: project, config: {...config, name: 'Renamed'}});
    [schedule] = await runtime.call('list');
    assert.equal(schedule.name, 'Renamed');
    await runtime.call('project.removeSchedule', {workingDir: project, id: 'auto-fixture'});
    assert.equal((await runtime.call('list')).length, 0);
    assert.equal(events.findLast((event) => event.type === 'reconciled').deleted, 1);
  } finally { await runtime.close(); }
});

test('AI pre-run check generation writes the script, self-tests it and reports model failures', async () => {
  const project = await directory();
  const requests = [];
  const tests = [];
  const runtime = await createAutomationRuntime({
    directory: await directory(), passive: true, dispatch: async () => ({sessionId: 'fixture'}),
    text: async (request) => { requests.push(request); return {ok: true, text: '```js\nprocess.exit(0);\n```'}; },
    hookSelfTest: async (request) => { tests.push(request); return {status: 'passed', decision: 'run', exitCode: 0, durationMs: 1, stdout: '', stderr: '', stdoutTruncated: false, stderrTruncated: false, timedOut: false, aborted: false}; },
  });
  try {
    const result = await runtime.call('hook.generate', {description: 'Run only when files changed', scheduleName: 'Nightly', workingDir: project, providerId: 'service', model: 'fixture'});
    assert.equal(result.ok, true);
    assert.match(result.command, /^xdt-node /);
    assert.equal(await fs.readFile(result.filePath, 'utf8'), 'process.exit(0);\n');
    assert.ok(result.filePath.startsWith(project));
    assert.equal(requests[0].providerId, 'service');
    assert.match(requests[0].prompt, /Run only when files changed/);
    assert.equal(tests[0].command, result.command);
    assert.equal(result.test.decision, 'run');
  } finally { await runtime.close(); }
  const failing = await createAutomationRuntime({directory: await directory(), passive: true, dispatch: async () => ({sessionId: 'fixture'}), text: async () => ({ok: false, reason: 'timeout', attempts: []})});
  try {
    assert.deepEqual(await failing.call('hook.generate', {description: 'Check', workingDir: project}), {ok: false, reason: 'timeout', attempts: []});
  } finally { await failing.close(); }
});

test('automations run on this computer, templates use Pi and bot routines stay out of the list', async () => {
  const root = await directory();
  const runtime = await createAutomationRuntime({directory: root, passive: true, dispatch: async () => ({sessionId: 'fixture'})});
  try {
    await assert.rejects(runtime.call('create', {input: {...definition, agentDeviceId: 'other-computer'}}), /this computer/);
    const created = await runtime.call('create', {input: {...definition, agentDeviceId: undefined}});
    assert.equal(created.agentDeviceId, undefined);
    assert.ok((await runtime.call('templates')).templates.every((template) => template.agentKind === 'pi'));
  } finally { await runtime.close(); }
  const store = new ScheduleStore(path.join(root, 'automations.db'));
  const now = Date.now();
  await store.insert({...definition, id: 'routine', source: 'bot', status: 'paused', createdAt: now, updatedAt: now});
  store.close();
  const reopened = await createAutomationRuntime({directory: root, passive: true, dispatch: async () => ({sessionId: 'fixture'})});
  try {
    assert.ok((await reopened.call('list')).every((schedule) => schedule.source !== 'bot'));
  } finally { await reopened.close(); }
});
