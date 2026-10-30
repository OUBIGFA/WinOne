import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';
import {OneBotProfileStore} from './profile-store.mjs';
const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('An isolated OneBot test output is required');
const root = () => fs.mkdtemp(path.join(base, 'runtime-'));

test('capability edits invalidate every live route of that Bot for the next turn',async()=>{
 const directory=await root(),events=[];
 const runtime=createOneBotRuntime({directory,event:value=>events.push(value)});
 try{
  const profile=await runtime.call('profiles.create',{input:{id:'resource-owner',name:'Resource owner'}});
  const canonical='ABCDEFGHIJKLMNOPQRSTUVWXYZ',channel='BCDEFGHIJKLMNOPQRSTUVWXYZA';
  await runtime.call('sessions.bind',{botId:profile.id,sessionId:canonical});
  await runtime.call('sessions.bind',{botId:profile.id,sessionId:channel,role:'channel',routeKey:'channel'});
  events.length=0;
  await runtime.call('profiles.update',{input:{id:profile.id,expectedVersion:profile.currentVersion,capabilities:{toolsetMode:'allowlist',toolsets:[]}}});
  assert.deepEqual(events.filter(event=>event.type==='runtime-refresh-requested').map(event=>event.sessionId).sort(),[canonical,channel].sort());
 }finally{await runtime.close();}
});

test('deleted Bot cleanup survives failed hooks and a runtime restart',async()=>{
 const directory=await root();
 let runtime=createOneBotRuntime({directory,lifecycle:{
  onDeleted:async()=>{throw Error('IM cleanup unavailable');},
  recycleHome:async()=>{throw Error('Home is still open');},
 }});
 try{
  await runtime.call('profiles.create',{input:{id:'cleanup',name:'Cleanup'}});
  const result=await runtime.call('lifecycle.action',{botId:'cleanup',action:'delete',confirmName:'Cleanup',keepTaskHistory:true});
  assert.equal(result.status,'deleted');
  assert.ok(result.warnings.includes('HOME_CLEANUP_PENDING'));
  assert.deepEqual(await runtime.call('lifecycle.pendingCleanup'),[{botId:'cleanup',imPending:true,homePending:true}]);
  await assert.rejects(runtime.call('profiles.get',{botId:'cleanup'}),/不存在/);
  await runtime.close();
  runtime=createOneBotRuntime({directory,lifecycle:{onDeleted:async()=>{},recycleHome:async()=>{}}});
  await assert.rejects(runtime.call('profiles.create',{input:{id:'cleanup',name:'Cannot reuse'}}),/清理|目录已存在/);
  const retried=await runtime.call('lifecycle.retryCleanup',{botId:'cleanup'});
  assert.deepEqual(retried,{warnings:[]});
  assert.deepEqual(await runtime.call('lifecycle.pendingCleanup'),[]);
 }finally{await runtime.close();}
});

test('upstream lifecycle pauses linked sessions, resumes the profile and keeps identity on restart',async()=>{
 const directory=await root(),closed=[],restarted=[];
 const runtime=createOneBotRuntime({directory,lifecycle:{closeSession:async id=>closed.push(id),restartRuntime:async id=>restarted.push(id)}});
 try{
  const bot=await runtime.call('profiles.create',{input:{id:'lifecycle',name:'生命周期',capabilities:{harness:'pi'}}});
  const session='ABCDEFGHIJKLMNOPQRSTUVWXYZ';
  await runtime.call('sessions.bind',{botId:bot.id,sessionId:session});
  const paused=await runtime.call('lifecycle.action',{botId:bot.id,action:'pause'});
  assert.equal(paused.status,'paused');assert.deepEqual(closed,[session]);
  assert.equal((await runtime.call('profiles.get',{botId:bot.id})).status,'paused');
  await assert.rejects(runtime.call('lifecycle.action',{botId:bot.id,action:'restart'}),/active|restart/);
  await runtime.call('lifecycle.action',{botId:bot.id,action:'resume'});
  const result=await runtime.call('lifecycle.action',{botId:bot.id,action:'restart'});
  assert.equal(result.status,'active');assert.deepEqual(restarted,[session]);
  assert.equal((await runtime.call('profiles.get',{botId:bot.id})).canonicalSessionId,session);
  await assert.rejects(runtime.call('lifecycle.action',{botId:bot.id,action:'delete',confirmName:'错误名称'}),/名称|完整/);
  assert.equal((await runtime.call('profiles.get',{botId:bot.id})).status,'active');
 }finally{await runtime.close();}
});
const input = {id: 'fixture', name: '本地伙伴', description: '帮助准备资料', capabilities: {harness: 'pi', permissions: 'ask', modelChain: [
  {harness: 'pi', providerId: 'fixture-provider', model: 'fixture-model', effort: 'deep', fastMode: false},
  {harness: 'pi', providerId: 'other-provider', model: 'other-model', effort: 'balanced', fastMode: true},
], toolCapabilityVersion: 1, toolsetMode: 'allowlist', toolsets: ['files'], mcpMode: 'allowlist', mcpServers: [], customExtension: {enabled: true}}};

test('profile portrait edits persist a version and reject an older settings form', async () => {
 const directory=await root();
 let runtime=createOneBotRuntime({directory});
 try {
  const created=await runtime.call('profiles.create',{input});
  const updated=await runtime.call('profiles.update',{input:{id:created.id,expectedVersion:created.currentVersion,avatar:'🐼',avatarColor:'green'}});
  assert.equal(updated.avatar,'🐼');
  assert.equal(updated.avatarColor,'green');
  assert.equal(updated.currentVersion,created.currentVersion+1);
  await assert.rejects(runtime.call('profiles.update',{input:{id:created.id,expectedVersion:created.currentVersion,avatar:'🐱'}}),/配置已更新/);
  await runtime.close();
  runtime=createOneBotRuntime({directory});
  const reopened=await runtime.call('profiles.get',{botId:created.id});
  assert.equal(reopened.avatar,'🐼');
  assert.equal(reopened.avatarColor,'green');
  assert.deepEqual(reopened.capabilities.customExtension,{enabled:true});
 } finally {await runtime.close();}
});

test('creating and renaming companions enforce the upstream normalized name identity', async () => {
 const directory=await root(),runtime=createOneBotRuntime({directory});
 try {
  const reader=await runtime.call('profiles.create',{input:{id:'reader',name:'Reader'}});
  await assert.rejects(runtime.call('profiles.create',{input:{id:'duplicate',name:' Ｒｅａｄｅｒ '}}), error=>error.code==='ALREADY_EXISTS');
  await assert.rejects(fs.access(path.join(directory,'bots','duplicate')),error=>error.code==='ENOENT');
  const writer=await runtime.call('profiles.create',{input:{id:'writer',name:'Writer'}});
  await assert.rejects(runtime.call('profiles.update',{input:{id:'writer',expectedVersion:writer.currentVersion,name:' reader '}}),error=>error.code==='ALREADY_EXISTS');
  const unchanged=await runtime.call('profiles.get',{botId:'writer'});
  assert.equal(unchanged.name,'Writer');
  assert.equal(unchanged.currentVersion,writer.currentVersion);
  const own=await runtime.call('profiles.update',{input:{id:'reader',expectedVersion:reader.currentVersion,name:'READER'}});
  assert.equal(own.name,'READER');
  assert.equal((await runtime.call('profiles.list')).length,2);
 } finally {await runtime.close();}
});

test('local profile API persists Pi routes and all capability fields, files and immutable versions', async () => {
  const directory = await root();
  const events = [];
  let runtime = createOneBotRuntime({directory, event: event => events.push(event)});
  try {
    const created = await runtime.call('profiles.create', {input});
    assert.equal(created.capabilities.modelChain.length, 2);
    assert.equal(created.capabilities.effort, 'deep');
    assert.deepEqual(created.capabilities.customExtension, {enabled: true});
    assert.equal(created.capabilities.permissions, 'ask');
    const updated = await runtime.call('profiles.update', {input: {id: created.id, expectedVersion: 1, name: '改名', identitySource: '新的身份', userContextSource: '用户偏好'}});
    assert.equal(updated.currentVersion, 2);
    assert.equal(updated.capabilities.modelChain.length, 2);
    assert.equal(await fs.readFile(path.join(directory, 'bots/fixture/SOUL.md'), 'utf8'), '新的身份');
    assert.equal(await fs.readFile(path.join(directory, 'bots/fixture/memories/USER.md'), 'utf8'), '用户偏好');
    const versions = await runtime.call('profiles.versions', {botId: created.id});
    assert.equal(versions.length, 2); assert.equal(versions[1].identitySource, input.description);
    await assert.rejects(runtime.call('profiles.update', {input: {id: created.id, expectedVersion: 1, name: '旧页面'}}), /配置已更新/);
    await runtime.close(); runtime = createOneBotRuntime({directory});
    assert.equal((await runtime.call('profiles.get', {botId: created.id})).name, '改名');
    assert.ok(events.some(e => e.type === 'profile-changed' && e.version === 2));
  } finally { await runtime.close(); }
});

test('mutable Home policy is ignored, but external SOUL/USER edits become a new version', async () => {
  const directory = await root();
  const runtime = createOneBotRuntime({directory});
  try {
    await runtime.call('profiles.create', {input});
    await fs.writeFile(path.join(directory, 'bots/fixture/config.json'), JSON.stringify({permissions: 'bypassPermissions', harness: 'codex'}));
    const source = '\n' + '身份'.repeat(7000) + '\n  ';
    await fs.writeFile(path.join(directory, 'bots/fixture/SOUL.md'), source);
    const current = await runtime.call('profiles.get', {botId: 'fixture'});
    assert.equal(current.currentVersion, 2);
    assert.equal(current.identitySource, source);
    assert.equal((await runtime.call('profiles.get', {botId: 'fixture'})).currentVersion, 2);
    assert.equal(current.capabilities.permissions, 'ask'); assert.equal(current.capabilities.harness, 'pi');
    await assert.rejects(runtime.call('profile.content.write', {botId: 'fixture', expectedVersion: 1, patch: {identitySource: '旧'}}), /配置已更新/);
  } finally { await runtime.close(); }
});

test('pending Home writes survive a process restart, and do not repeat after applying', async () => {
  const directory = await root();
  const store = new OneBotProfileStore(directory);
  store.create(input);
  assert.ok(store.pendingContent('fixture')); store.close();
  const runtime = createOneBotRuntime({directory});
  try {
    const current = await runtime.call('profiles.get', {botId: 'fixture'});
    assert.equal(current.identitySource, input.description);
    await fs.writeFile(path.join(directory, 'bots/fixture/SOUL.md'), '后续文件编辑');
    assert.equal((await runtime.call('profiles.get', {botId: 'fixture'})).identitySource, '后续文件编辑');
  } finally { await runtime.close(); }
});

test('unsupported harness/remote routes fail before creating a profile or Home', async () => {
  const directory = await root();
  const runtime = createOneBotRuntime({directory});
  try {
    for (const capabilities of [{harness: 'codex'}, {harness: 'pi', agentDeviceId: 'another-device'},
      {harness: 'pi', modelChain: [{harness: 'pi', providerId: 'model-route:["remote","provider"]', model: 'model'}]}]) {
      await assert.rejects(runtime.call('profiles.create', {input: {...input, capabilities}}), /仅使用 Pi|本机运行|远程模型路线/);
    }
    assert.deepEqual(await runtime.call('profiles.list'), []);
    await assert.rejects(fs.access(path.join(directory, 'bots/fixture')));
    await assert.rejects(runtime.call('cloud.connect'), /不支持/);
    await assert.rejects(runtime.call('profiles.create', {input: {...input, id: 'con'}}), /标识无效/);
  } finally { await runtime.close(); }
});

test('the runtime is the sole writer, releases ownership on close, and rejects a second live instance', async () => {
  const directory = await root();
  const first = createOneBotRuntime({directory});
  try {
    assert.throws(() => createOneBotRuntime({directory}), /已有 OneBot 服务运行/);
    await first.call('profiles.create', {input});
    await first.call('memory.write', {botId: 'fixture', input: {type: 'user', name: 'preference', title: '偏好', description: '沟通偏好', body: '简明表达'}});
  } finally { await first.close(); }
  const second = createOneBotRuntime({directory});
  try { assert.equal((await second.call('memory.list', {botId: 'fixture'})).length, 1); }
  finally { await second.close(); }
});

test('unknown profile directories are preserved and are not adopted by create', async () => {
  const directory = await root();
  await fs.mkdir(path.join(directory, 'bots/fixture'), {recursive: true});
  await fs.writeFile(path.join(directory, 'bots/fixture/SOUL.md'), '用户资料');
  const runtime = createOneBotRuntime({directory});
  try {
    await assert.rejects(runtime.call('profiles.create', {input}), /未覆盖/);
    assert.deepEqual(await runtime.call('profiles.list'), []);
    assert.equal(await fs.readFile(path.join(directory, 'bots/fixture/SOUL.md'), 'utf8'), '用户资料');
  } finally { await runtime.close(); }
});
