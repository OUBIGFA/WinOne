import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import JSZip from 'jszip';
import {createGhostHost} from './host.mjs';
import {createAutomationRuntime} from '../automations.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
async function fixture(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/ghost-checks-tests');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'ghost-'));
  const host = createGhostHost({directory,
    executable: process.env.WINONE_GHOST_SANDBOX ?? path.join(directory, 'unused-helper.exe'),
    sdkFile: process.env.WINONE_GHOST_SDK ?? path.join(directory, 'unused-sdk.js'),
    mailSDKFile: fileURLToPath(new URL('../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)),
    snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT,
    masterKey: Buffer.alloc(32, 7).toString('base64'),
  });
  t.after(() => host.close());
  const archive = path.join(directory, 'check.cindy');
  async function packageOf(code, version = '1.0.0') {
    const zip = new JSZip();
    // This is an unpublished fixture for the verified 2d8d3c3 source build,
    // whose desktop package declares 0.0.0; it makes no stable-version claim.
    zip.file('ghost.json', JSON.stringify({schemaVersion: 3, minCindyVersion: '0.0.0', id: 'fixture-check', name: 'Fixture check', version,
      description: 'Checks one scheduled run', entry: 'main.js', subscribe: {hooks: ['will-schedule-run', 'will-routine-run']}}));
    zip.file('main.js', code);
    await fs.writeFile(archive, await zip.generateAsync({type: 'nodebuffer'}));
    return archive;
  }
  return {directory, host, packageOf};
}

const code = `cindy.onHostMessage(async message => {
  if (message.type !== 'event') return;
  const action = message.data.prompt.includes('stop') ? 'block' : message.data.prompt.includes('skip') ? 'skip' : 'allow';
  await cindy.send({type:'event-verdict',hookId:message.hookId,action,reason:'fixture decision'});
});`;

// Installation and saved failure handling use the real Cindy manager/gateway;
// these default tests neither launch a browser nor interact with the desktop.
test('a local plugin is inspected, receipt-bound, selectable and explicitly removable', async t => {
  const f = await fixture(t), file = await f.packageOf(code);
  const inspected = await f.host.call('inspect', {path: file});
  assert.deepEqual(inspected.points, ['will-schedule-run', 'will-routine-run']);
  await assert.rejects(f.host.call('install', {token: inspected.token}), /核对|确认/);
  const checked = await f.host.call('inspect', {path: file});
  const installed = await f.host.call('install', {token: checked.token, confirmed: true});
  const candidates = await f.host.call('candidates', {point: 'will-schedule-run'});
  assert.deepEqual(candidates, [{pluginId: installed.pluginId, name: 'Fixture check'}]);
  assert.equal((await f.host.call('resolve', {point: 'will-schedule-run', pluginId: installed.pluginId})).pluginId, installed.pluginId);
  await f.host.call('enabled', {pluginId: installed.pluginId, enabled: false});
  assert.deepEqual(await f.host.call('candidates', {point: 'will-schedule-run'}), []);
  await assert.rejects(f.host.call('remove', {pluginId: installed.pluginId}), /确认/);
  await f.host.call('remove', {pluginId: installed.pluginId, confirmed: true});
  assert.deepEqual(await f.host.call('list'), []);
});

test('changed package bytes cannot reuse the inspection confirmation', async t => {
  const f = await fixture(t), file = await f.packageOf(code);
  const inspected = await f.host.call('inspect', {path: file});
  await f.packageOf(code + '\n// changed', '1.1.0');
  await assert.rejects(f.host.call('install', {token: inspected.token, confirmed: true}), /变化|重新选择/);
  assert.deepEqual(await f.host.call('list'), []);
});

test('an unavailable saved check follows every user-selected failure policy without starting a sandbox', async t => {
  const {host} = await fixture(t);
  for (const action of ['allow', 'skip', 'block']) {
    const result = await host.call('screen', {point: 'will-schedule-run', target: {pluginId: 'missing', onFailure: action}, data: {scheduleId: 's', runId: 'r', name: 'Check', prompt: 'Check', executionMode: 'agent', trigger: 'manual', firedAt: 1}});
    assert.equal(result.action, action);
    if (action !== 'allow') assert.equal(result.failure, true);
  }
});

// Explicitly selected by build/test-native-ghost-checks.ps1 after the reviewed
// native helper and SDK are built. These are real hidden WebView2 processes,
// not JavaScript eval substitutes or mock plugin verdicts.
test('the real sandbox loads a genuine plugin and delivers allow, skip and block', {skip: !process.env.WINONE_GHOST_SANDBOX}, async t => {
  const f = await fixture(t), file = await f.packageOf(code);
  const checked = await f.host.call('inspect', {path: file});
  const installed = await f.host.call('install', {token: checked.token, confirmed: true});
  for (const [prompt, action] of [['continue', 'allow'], ['skip this run', 'skip'], ['stop this run', 'block']]) {
    const result = await f.host.call('screen', {point: 'will-schedule-run', target: {pluginId: installed.pluginId, onFailure: 'block'}, data: {scheduleId: 's', runId: prompt, name: 'Check', prompt, executionMode: 'agent', trigger: 'manual', firedAt: 1}});
    assert.equal(result.action, action);
    if (action !== 'allow') { assert.equal(result.failure, false); assert.equal(result.reason, 'fixture decision'); }
  }
  const result = await f.host.call('screen', {point: 'will-routine-run', target: {pluginId: installed.pluginId, onFailure: 'block'}, data: {routineId: 'routine', runId: 'routine-run', botId: 'bot', name: 'Routine', prompt: 'skip the routine', builtin: null, triggerIds: ['manual'], events: []}});
  assert.equal(result.action, 'skip'); assert.equal(result.failure, false);
});

test('a genuine plugin stops the real scheduler before task, command or model dispatch', {skip: !process.env.WINONE_GHOST_SANDBOX}, async t => {
  const f = await fixture(t), file = await f.packageOf(code);
  const inspected = await f.host.call('inspect', {path: file});
  const installed = await f.host.call('install', {token: inspected.token, confirmed: true});
  let catalog = new Set(), dispatched = 0;
  const notices = [];
  const plugins = {
    async refresh(point) { catalog = new Set((await f.host.call('candidates', {point})).map(row => row.pluginId)); },
    resolve(_point, id) { if (!catalog.has(id)) throw Error('Unavailable plugin'); return id; },
    screen(point, data, target) { return f.host.call('screen', {point, data, target}); },
  };
  const scheduler = await createAutomationRuntime({directory: path.join(f.directory, 'scheduler'), plugins, passive: true,
    dispatch: async () => { dispatched++; return {sessionId: 'must-not-be-created'}; }, notify: async (_schedule, run) => { notices.push(run.id); }});
  t.after(() => scheduler.close());
  const definition = {name: 'Plugin guard', prompt: 'skip this run', kind: 'cron', cronExpr: '0 9 * * *', timezone: 'UTC', recurring: true, manual: true, agentKind: 'pi', workspaceKind: 'dialogue', notify: {desktop: true, feishu: false}, preRunPlugin: {pluginId: installed.pluginId, onFailure: 'block'}};
  const task = await scheduler.call('create', {input: definition});
  await scheduler.call('run', {id: task.id}); await scheduler.settled();
  assert.equal(dispatched, 0);
  assert.equal((await scheduler.call('runs', {id: task.id}))[0].status, 'skipped');
  assert.equal(notices.length, 0);
  await scheduler.call('update', {id: task.id, patch: {prompt: 'stop this run'}});
  await scheduler.call('run', {id: task.id}); await scheduler.settled();
  const stopped = (await scheduler.call('runs', {id: task.id}))[0];
  assert.equal(stopped.status, 'failed'); assert.match(stopped.errorMsg, /fixture decision/);
  assert.equal(dispatched, 0);
  assert.equal(notices.length, 1, 'the early plugin stop and failed-run subscriber must not notify twice');
});
