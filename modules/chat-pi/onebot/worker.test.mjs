import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {createOneBotPaths} from './content-service.mjs';
import {withIMScope} from './im-scope.mjs';
import {addBotImConnectionRecord} from '../cindy/bots/botImConnectionStore.ts';
const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('An isolated OneBot test output is required');
const source = process.env.WINONE_ONEBOT_WORKER ?? fileURLToPath(new URL('./worker.mjs', import.meta.url));
async function worker(args = [source]) {
  const child = spawn(process.execPath, args, {windowsHide: true, stdio: ['pipe', 'pipe', 'pipe']});
  const waiting = new Map();
  let sequence = 0, stderr = '';
  child.stderr.on('data', bytes => {stderr += bytes;});
  const lines = createInterface({input: child.stdout});
  const exit = once(child, 'exit');
  const ready = new Promise((resolve, reject) => {
    child.on('error', reject);
    child.on('exit', (code) => {
      const error = Error(`Worker exited (${code}): ${stderr}`);
      reject(error);
      for (const request of waiting.values()) request.reject(error);
      waiting.clear();
    });
    lines.on('line', line => {
      const message = JSON.parse(line);
      if (message.type === 'ready') resolve(message);
      if (message.type === 'response') { waiting.get(message.id)?.resolve(message); waiting.delete(message.id); }
    });
  });
  await ready;
  function call(method, params) {
    const id = String(++sequence);
    return new Promise((resolve, reject) => { waiting.set(id, {resolve, reject}); child.stdin.write(JSON.stringify({id, method, params}) + '\n'); });
  }
  return {child, call, async close() {child.stdin.end(); const [code] = await exit; assert.equal(code, 0, stderr);}};
}

test('worker protocol preserves settings errors and acknowledged writes through EOF and restart', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'worker-'));
  let w = await worker();
  try {
    assert.equal((await w.call('profiles.list', {})).errorCode, 'NOT_READY');
    assert.equal((await w.call('init', {directory})).ok, true);
    assert.equal((await w.call('init', {directory})).errorCode, 'ALREADY_INITIALIZED');
    const created = await w.call('profiles.create', {input: {id: 'worker-bot', name: '测试 OneBot'}});
    assert.equal(created.ok, true); assert.match(created.value.identitySource, /OneBot/);
    const failure = await w.call('profiles.update', {input: {id: 'worker-bot', expectedVersion: 0, name: 'stale'}});
    assert.equal(failure.errorCode, 'PRECONDITION_FAILED'); assert.match(failure.error, /^\[PRECONDITION_FAILED\]/);
    const pending = w.call('memory.write', {botId: 'worker-bot', input: {type: 'user', name: 'pref', title: '偏好', description: '简洁沟通', body: '使用简体中文'}});
    const closed = w.close();
    assert.equal((await pending).ok, true);
    await closed;
    w = await worker();
    assert.equal((await w.call('init', {directory})).ok, true);
    const list = await w.call('memory.list', {botId: 'worker-bot'});
    assert.equal(list.value.length, 1); assert.equal(list.value[0].filename, 'user_pref.md');
    assert.equal((await w.call('cloud.connect', {})).errorCode, 'UNKNOWN_METHOD');
  } finally { await w.close(); }
});

test('worker first home opens without loading optional execution libraries', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'startup-'));
  // Source guard observes evaluated dependencies. Bundled latency and behavior
  // are measured separately through build/benchmark-onebot-startup.mjs.
  const w = await worker(['--preload', fileURLToPath(new URL('./fixtures/startup-import-guard.mjs', import.meta.url)), fileURLToPath(new URL('./worker.mjs', import.meta.url))]);
  try {
    assert.equal((await w.call('init', {directory})).ok, true);
    const profiles = await w.call('profiles.list', {});
    assert.equal(profiles.ok, true); assert.deepEqual(profiles.value, []);
    const previews = await w.call('bots.previews', {items: []});
    assert.equal(previews.ok, true); assert.deepEqual(previews.value, []);
  } finally { await w.close(); }
});

test('worker loads each saved IM transport on demand and closes without credentials', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'transport-load-'));
  const scope = {paths: createOneBotPaths(directory), key: directory, closed: false};
  const records = [];
  // Original records, deliberately without credentials: real transport init
  // remains idle and never contacts a platform. No fake factory is involved.
  for (const channel of ['telegram', 'feishu', 'discord', 'wecom', 'dingtalk']) {
    records.push(await withIMScope(scope, () => addBotImConnectionRecord({botId: 'test-bot', channel})));
  }
  scope.closed = true;
  const w = await worker();
  try {
    const initialized = await w.call('init', {directory, masterKey: Buffer.alloc(32, 7).toString('base64'), dispatchBaseUrl: 'http://127.0.0.1:1', dispatchKey: 'transport-load-fixture'});
    assert.equal(initialized.ok, true, initialized.error);
    for (const record of records) {
      const result = await w.call('im.reconnect', {id: record.id});
      assert.equal(result.ok, true, `${record.channel}: ${result.error}`);
      assert.equal(result.value.connections.find(item => item.id === record.id).status.kind, 'idle');
    }
  } finally { await w.close(); }
});
