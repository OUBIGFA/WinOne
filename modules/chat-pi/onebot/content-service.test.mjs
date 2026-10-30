// Real Bun SQLite + upstream filesystem services, not mocked FTS/CRUD.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {BOT_MEMORY_CHANGED} from '../cindy/bots/shared/botMemory.ts';

import {createOneBotContentService, createOneBotPaths} from './content-service.mjs';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Run through build/test-onebot.mjs --bun <BunExe>; an isolated output is required');
const record = {type: 'reference', name: 'boundary', title: '数据边界', description: '中文检索边界说明', body: '共享数据边界索引配置说明。'};
const todo = {key: 'quote:one', title: '比较报价', outcome: '得到可核查的比较', next: {kind: 'advance', label: '准备比较', instruction: '只准备草稿'}, sources: [{kind: 'conversation', id: 'fixture', label: '本地对话'}]};
async function fixture(options = {}) {
  const directory = await fs.mkdtemp(path.join(base, 'content-'));
  const profiles = new Map(['alpha', 'beta'].map(id => [id, {key: id + ':1', status: 'active', canonicalSessionId: id + '-session'}]));
  const refreshes = [];
  const readBot = async id => {
    const row = profiles.get(id);
    return row && {...row, assertCurrent() { if (profiles.get(id) !== row) throw Error('PROFILE_CHANGED'); }};
  };
  const service = createOneBotContentService({directory, readBot, requestRefresh: async id => {refreshes.push(id);}, ...options});
  const call = (method, args = {}, botId = 'alpha') => service.call(method, {botId, ...args});
  return {directory, profiles, service, call, refreshes, readBot};
}

test('memory write/list/search/index/reopen use one Bot Home and keep other Bots isolated', async () => {
  const f = await fixture();
  try {
    await f.call('memory.write', {input: record});
    const [entry] = await f.call('memory.list');
    assert.equal(entry.filename, 'reference_boundary.md');
    assert.equal(entry.title, record.title);
    assert.match(await f.call('memory.index'), /reference_boundary\.md/);
    assert.equal((await f.call('memory.search', {query: '边界'}))[0].filename, entry.filename);
    assert.equal((await f.call('memory.list', {}, 'beta')).length, 0);
    assert.match(await fs.readFile(path.join(f.directory, 'bots/alpha/memories/MEMORY.md'), 'utf8'), /数据边界/);
    assert.deepEqual(f.refreshes, ['alpha-session']);
    await f.service.close();
    const reopened = createOneBotContentService({directory: f.directory, readBot: f.readBot});
    try { assert.equal((await reopened.call('memory.read', {botId: 'alpha', filename: entry.filename})).body, record.body); }
    finally { await reopened.close(); }
  } finally { await f.service.close(); }
});

test('two settings editors conflict instead of overwriting; delete uses the saved version', async () => {
  const f = await fixture();
  try {
    await f.call('memory.write', {input: record});
    const original = await f.call('memory.read', {filename: 'reference_boundary.md'});
    const updates = await Promise.allSettled(['甲', '乙'].map(body => f.call('memory.update', {...original, expectedUpdatedAt: original.updatedAt, body})));
    assert.equal(updates.filter(x => x.status === 'fulfilled').length, 1);
    assert.ok(updates.find(x => x.status === 'rejected').reason.message.includes(BOT_MEMORY_CHANGED));
    await assert.rejects(f.call('memory.delete', {filename: original.filename, expectedUpdatedAt: original.updatedAt}), error => error.message.includes(BOT_MEMORY_CHANGED));
    const current = await f.call('memory.read', {filename: original.filename});
    await f.call('memory.delete', {filename: original.filename, expectedUpdatedAt: current.updatedAt});
    assert.deepEqual(await f.call('memory.list'), []);
    assert.deepEqual(await f.call('memory.search', {query: '边界'}), []);
  } finally { await f.service.close(); }
});

test('imports retain exact whitespace, replay idempotently and reject edited source records', async () => {
  const f = await fixture();
  const input = {id: 'hermes', title: '导入资料', text: '\n  原文\n\n', type: 'reference'};
  try {
    await f.call('memory.importDocument', input);
    await f.call('memory.importDocument', input);
    assert.equal((await f.call('memory.read', {filename: 'reference_import_hermes_0.md'})).body, input.text);
    assert.equal((await f.call('memory.list')).length, 1);
    await assert.rejects(f.call('memory.importDocument', {...input, text: '\n不同原文\n'}), /Imported memory was edited/);
  } finally { await f.service.close(); }
});

test('idle memory handles can be evicted without losing data or breaking simultaneous calls', async () => {
  const f = await fixture({maxIdleStores: 0});
  try {
    await Promise.all(['alpha', 'beta'].map(botId => f.call('memory.write', {input: record}, botId)));
    for (let index = 0; index < 3; index++) {
      assert.deepEqual(await Promise.all(['alpha', 'beta'].map(botId => f.call('memory.read', {filename: 'reference_boundary.md'}, botId).then(r => r.body))), [record.body, record.body]);
    }
  } finally { await f.service.close(); }
});

test('profile content validates every field first and never exposes writable host policy', async () => {
  const f = await fixture();
  try {
    await f.call('profile.content.write', {patch: {identitySource: '身份 A', userContextSource: '用户画像', systemPromptOverride: '附加说明'}});
    await assert.rejects(f.call('profile.content.write', {patch: {identitySource: '身份 B', userContextSource: '中'.repeat(65536)}}), /64 KiB/);
    await assert.rejects(f.call('profile.content.write', {patch: {config: {permissionMode: 'bypassPermissions'}}}), /只能更新/);
    const content = await f.call('profile.content.read');
    assert.deepEqual(content, {identitySource: '身份 A', userContextSource: '用户画像', systemPromptOverride: '附加说明'});
    assert.equal('config' in content, false);
  } finally { await f.service.close(); }
});

test('Skills preserve scripts when enabled/disabled, support query paging and native mounts', async () => {
  const f = await fixture();
  try {
    for (const slug of ['check-one', 'check-two']) await f.call('skills.save', {input: {slug, name: slug, description: '数据检查', body: '读取后检查。'}});
    const script = path.join(f.directory, 'bots/alpha/skills/check-one/check.mjs');
    await fs.writeFile(script, '// retained resource\n');
    const source = await f.call('skills.readSource', {slug: 'check-one'});
    await f.call('skills.saveSource', {slug: 'check-one', source, options: {enabled: false}});
    assert.equal(await fs.readFile(path.join(f.directory, 'bots/alpha/disabled-skills/check-one/check.mjs'), 'utf8'), '// retained resource\n');
    assert.equal((await f.call('skills.read', {slug: 'check-one'})).enabled, false);
    const mounts = await f.call('skills.mounts');
    assert.deepEqual(mounts.skills.map(s => s.name), ['check-two']);
    const page = await f.call('skills.query', {query: '数据', limit: 1});
    assert.equal(page.total, 2); assert.equal(page.nextOffset, 1);
    assert.equal((await f.call('skills.query', {query: '数据', offset: page.nextOffset})).skills.length, 1);
    assert.equal((await f.call('skills.list', {}, 'beta')).length, 0);
    assert.deepEqual(await f.call('skills.delete', {slug: 'check-one'}), {deleted: true});
    assert.equal((await f.call('skills.list')).length, 1);
  } finally { await f.service.close(); }
});

test('todo receipts prevent uncertain requests replay; success is not fabricated completion', async () => {
  const f = await fixture();
  try {
    const saved = await f.call('todos.patch', {patch: todo});
    const action = {id: saved.id, revision: saved.revision, requestId: 'attempt-1'};
    assert.equal((await f.call('todos.prepareAction', action)).dispatch, true);
    await f.call('todos.settleAction', {...action, result: {ok: false, uncertain: true}});
    assert.equal((await f.call('todos.prepareAction', {...action, requestId: 'attempt-2'})).dispatch, false);
    assert.equal((await f.call('todos.read')).items[0].status, 'open');
    assert.deepEqual((await f.call('todos.read', {}, 'beta')).items, []);
    const observed = await f.call('todos.ingest', {input: {source: 'local-event', sequence: 1, patch: {...todo, key: 'automatic:one', origin: 'assigned'}}});
    assert.equal(observed.todo.origin, 'discovered');
  } finally { await f.service.close(); }
});

test('unknown/invalid/deleting identities cannot create files; stale ownership is rejected', async () => {
  const f = await fixture();
  try {
    await assert.rejects(f.call('memory.write', {input: record}, 'absent'), /不存在/);
    await assert.rejects(f.call('memory.write', {input: record}, '../alpha'), /标识无效/);
    await f.call('memory.write', {input: record});
    f.profiles.set('alpha', {...f.profiles.get('alpha'), key: 'alpha:2', status: 'deleting'});
    await assert.rejects(f.call('memory.write', {input: {...record, name: 'forbidden'}}), /正在删除/);
    await assert.rejects(fs.access(path.join(f.directory, 'bots/absent')));
    await assert.rejects(fs.access(path.join(f.directory, 'bots/alpha/memories/reference_forbidden.md')));
  } finally { await f.service.close(); }
});

test('closing drains accepted writes, rejects new requests and releases all database handles', async () => {
  const f = await fixture();
  const accepted = f.call('memory.write', {input: record});
  const closed = f.service.close();
  assert.equal(f.service.close(), closed);
  await assert.rejects(f.call('memory.list'), /正在关闭/);
  await accepted; await closed;
  // On Windows, renaming after close exercises release of the live database.
  await fs.rename(path.join(f.directory, 'bots/alpha/memories/fts.db'), path.join(f.directory, 'bots/alpha/memories/closed.db'));
  assert.match(await fs.readFile(path.join(f.directory, 'bots/alpha/memories/MEMORY.md'), 'utf8'), /数据边界/);
});

test('junctions and hard-linked memory files do not redirect reads/writes out of the Home', async () => {
  const f = await fixture();
  const outside = await fs.mkdtemp(path.join(base, 'outside-'));
  try {
    await fs.mkdir(path.join(f.directory, 'bots/alpha'), {recursive: true});
    await fs.symlink(outside, path.join(f.directory, 'bots/alpha/memories'), 'junction');
    await assert.rejects(f.call('memory.write', {input: record}), /路径不能经过链接/);
    assert.deepEqual(await fs.readdir(outside), []);
    await fs.unlink(path.join(f.directory, 'bots/alpha/memories'));
    await fs.mkdir(path.join(f.directory, 'bots/alpha/memories'));
    const external = path.join(outside, 'keep.md');
    await fs.writeFile(external, 'external bytes');
    await fs.link(external, path.join(f.directory, 'bots/alpha/memories/reference_boundary.md'));
    await assert.rejects(f.call('memory.write', {input: record}), /路径不能经过链接/);
    assert.equal(await fs.readFile(external, 'utf8'), 'external bytes');
  } finally { await f.service.close(); }
});

test('generated Skill metadata cannot follow a nested junction into external files', async () => {
  const f = await fixture();
  const outside = await fs.mkdtemp(path.join(base, 'generated-outside-'));
  try {
    const shelf = path.join(f.directory, 'bots/alpha/.runtime-skills/skills');
    await fs.mkdir(shelf, {recursive: true});
    await fs.symlink(outside, path.join(shelf, 'personal-skill-library'), 'junction');
    await assert.rejects(f.call('skills.mounts'), /路径不能经过链接/);
    assert.deepEqual(await fs.readdir(outside), []);
  } finally { await f.service.close(); }
});

test('data roots must be explicit and cannot be replaced under an existing service', async () => {
  assert.throws(() => createOneBotPaths('relative'), /绝对数据目录/);
  const f = await fixture();
  try {
    await fs.rename(f.directory, f.directory + '-original');
    await fs.mkdir(f.directory);
    await assert.rejects(f.call('memory.write', {input: record}), /数据目录已被替换/);
    assert.deepEqual(await fs.readdir(f.directory), []);
  } finally { await f.service.close(); }
});
