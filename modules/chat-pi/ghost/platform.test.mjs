// Real packages through the real manager; sandbox runs are selected explicitly by
// build/test-native-ghost-checks.ps1, as they start hidden browser processes.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import JSZip from 'jszip';
import {ghostAgentTools} from './agent-tools.mjs';
import http from 'node:http';
import {once} from 'node:events';
import {createGhostHost} from './host.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const sandboxed = {skip: !process.env.WINONE_GHOST_SANDBOX};

async function fixture(t, hostHandler = async () => null) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/ghost-platform-tests');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'platform-'));
  const events = [];
  const options={directory,
    executable: process.env.WINONE_GHOST_SANDBOX ?? path.join(directory, 'unused-helper.exe'),
    sdkFile: process.env.WINONE_GHOST_SDK ?? path.join(directory, 'unused-sdk.js'),
    mailSDKFile: fileURLToPath(new URL('../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)),
    snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT, runtimeExecutable: process.execPath, nodeWorker: fileURLToPath(new URL('./node-worker.mjs', import.meta.url)),
    masterKey: Buffer.alloc(32, 7).toString('base64'),
    host: hostHandler, event: event => { trace(JSON.stringify(event).slice(0, 300)); events.push(event); },
  };
  const host=createGhostHost(options);
  t.after(() => host.close());
  async function install(manifest, files) {
    const zip = new JSZip();
    zip.file('ghost.json', JSON.stringify({schemaVersion: 3, minCindyVersion: '0.0.0', id: 'fixture', name: 'Fixture', version: '1.0.0', description: 'Fixture plugin', entry: 'main.js', kind: 'chip', ...manifest}));
    for (const [name, content] of Object.entries(files)) zip.file(name, content);
    const file = path.join(directory, 'fixture.cindy');
    await fs.writeFile(file, await zip.generateAsync({type: 'nodebuffer'}));
    const inspected = await host.call('inspect', {path: file});
    return host.call('install', {token: inspected.token, confirmed: true});
  }
  return {directory, host, events, install, async reopen(){
    await host.close();
    const reopened=createGhostHost(options);
    t.after(()=>reopened.close());
    return reopened;
  }};
}

const trace = process.env.GHOST_TRACE ? console.error : () => {};
const session = directory => ({id: 'conversation-1', workingDir: directory, approved: false});
const echo = `cindy.onHostMessage(async message => {
  if (message.type !== 'tool-call') return;
  const reply = result => cindy.send({type: 'tool-result', callId: message.callId, ok: true, result});
  if (message.tool === 'echo') return reply({said: message.args.text});
  if (message.tool === 'notify') { await cindy.send({type: 'notify', text: 'hello from plugin'}); return reply({ok: true}); }
  if (message.tool === 'confirm') { const answer = await cindy.confirm({body: 'Proceed?', confirmText: 'Yes', cancelText: 'No'}); return reply(answer); }
  if (message.tool === 'write') { const written = await cindy.fs({op: 'write', root: 'data', path: 'note.txt', content: 'stored'}); const read = await cindy.fs({op: 'read', root: 'data', path: 'note.txt'}); const work = await cindy.fs({op: 'write', root: 'workdir', path: 'out/note.txt', content: 'to workdir', callId: message.callId}); return reply({written, read, work}); }
  if (message.tool === 'library') { const put = await cindy.library({op: 'write', path: 'a/b.txt', content: 'library text'}); const got = await cindy.library({op: 'read', path: 'a/b.txt'}); await cindy.library({op: 'db.open', dbPath: 'library.sqlite'}); const sql = await cindy.library({op: 'db.exec', dbPath: 'library.sqlite', sql: 'CREATE TABLE IF NOT EXISTS t(v TEXT)'}); return reply({put, got, sql}); }
  if (message.tool === 'slow') { await new Promise(resolve => setTimeout(resolve, 3000)); return reply({}); }
  return cindy.send({type: 'tool-result', callId: message.callId, ok: false, errorCode: 'GHOST_ERROR', message: 'unknown tool'});
});`;
const tools = ['echo', 'notify', 'confirm', 'write', 'library', 'slow'].map(name => ({name, description: 'Fixture ' + name, parameters: {type: 'object', properties: {text: {type: 'string'}}}}));

test('tools and manuals are discoverable, and a missing plugin or tool is a structured error', async t => {
  const f = await fixture(t);
  await f.install({tools, manual: {items: [{name: 'guide', description: 'How to', dir: 'manual/guide'}]}}, {'main.js': echo, 'manual/guide/MANUAL.md': '---\nname: guide\ndescription: How to\n---\nRead me.\n'});
  const listed = await f.host.call('agent-list', {session: session(f.directory)});
  assert.equal(listed[0].id, 'fixture');
  assert.deepEqual(listed[0].tools.map(tool => tool.name), tools.map(tool => tool.name));
  assert.equal((await f.host.call('agent-info', {ghostId: 'fixture', session: session(f.directory)})).ok, true);
  assert.equal((await f.host.call('agent-info', {ghostId: 'absent', session: session(f.directory)})).errorCode, 'GHOST_NOT_FOUND');
  assert.match((await f.host.call('agent-manual', {ghostId: 'fixture', path: 'guide', session: session(f.directory)})).content, /Read me/);
  assert.equal((await f.host.call('agent-call', {ghostId: 'fixture', tool: 'absent', session: session(f.directory)})).errorCode, 'TOOL_NOT_FOUND');
});

test('workspace plugin overrides persist and gate roster, info, manual, calls and commands',async t=>{
 const f=await fixture(t),installed=await f.install({tools,command:'fixture'}, {'main.js':echo});
 assert.equal((await f.host.call('agent-roster',{session:session(f.directory)})).length,1);
 await f.host.call('set-workdir-preference',{workingDir:f.directory,pluginId:installed.pluginId,disabled:true});
 assert.deepEqual(await f.host.call('agent-list',{session:session(f.directory)}),[]);
 assert.deepEqual(await f.host.call('agent-roster',{session:session(f.directory)}),[]);
 for(const method of ['agent-info','agent-manual','agent-call']){
  const result=await f.host.call(method,{ghostId:'fixture',tool:'echo',session:session(f.directory)});assert.equal(result.errorCode,'GHOST_DISABLED_IN_WORKDIR');
 }
 assert.equal((await f.host.call('expand-command',{text:'$fixture hello',workingDir:f.directory})).text,'$fixture hello');
 const reopened=await f.reopen();assert.equal((await reopened.call('agent-info',{ghostId:'fixture',session:session(f.directory)})).errorCode,'GHOST_DISABLED_IN_WORKDIR');
 await reopened.call('set-workdir-preference',{workingDir:f.directory,pluginId:installed.pluginId,disabled:false});
 assert.equal((await reopened.call('agent-roster',{session:session(f.directory)})).length,1);
});

test('Agent Skills load only the verified approval snapshot, never a mutable package path', async t => {
  const f=await fixture(t);
  const skill='---\nname: fixture-skill\ndescription: Approved fixture\n---\nAPPROVED_SKILL_BODY\n';
  await f.install({skill:{items:[{name:'fixture-skill',description:'Approved fixture',dir:'skills/fixture-skill'}]}},{'main.js':'cindy.onHostMessage(()=>{});','skills/fixture-skill/SKILL.md':skill});
  const directories=await f.host.call('agent-skills',{});
  assert.equal(directories.length,1);
  assert.match(directories[0],/skill-snapshots/);
  assert.equal(await fs.readFile(path.join(directories[0],'SKILL.md'),'utf8'),skill);
  await fs.writeFile(path.join(directories[0],'SKILL.md'),skill+'UNAPPROVED_CHANGE');
  assert.deepEqual(await f.host.call('agent-skills',{}),[],'a changed snapshot must not be silently trusted or replaced by package files');
});

test('unconfigured required secret opens Host setup; cancellation never starts the business sandbox',async t=>{
  const prompts=[];
  const f=await fixture(t,async(method,params)=>{if(method==='ui')prompts.push(params);return null;});
  await f.install({tools, settingsHtml: 'settings.html', network: {hosts: ['example.com'], secrets: [{key: 'token', label: 'Token', required: true, inject: {header: 'Authorization', format: 'Bearer {value}', hosts: ['example.com']}}]}}, {'main.js': echo, 'settings.html': '<p>x</p>'});
  const result = await f.host.call('agent-call', {ghostId: 'fixture', tool: 'echo', args: {}, session: session(f.directory)});
  assert.equal(result.errorCode,'SETUP_CANCELLED');
  assert.ok(prompts.some(prompt=>prompt.kind==='plugin-setup'&&prompt.request.kind==='plugin_setup'));
  assert.ok(!f.events.some(event => event.type === 'state' && event.state === 'running'));
});

test('a real sandbox answers a tool call and host slots reach their seams', sandboxed, async t => {
  const prompts = [];
  const f = await fixture(t, async (method, params) => {
    prompts.push([method, params]);
    if (method === 'ui' && params.kind === 'confirm') return {confirmed: true};
    return null;
  });
  await f.install({tools, notify: true, confirm: true, fs: true}, {'main.js': echo});
  const call = (tool, args = {}) => f.host.call('agent-call', {ghostId: 'fixture', tool, args, session: session(f.directory)});
  assert.deepEqual((await call('echo', {text: 'hi'})).result, {said: 'hi'});
  assert.equal((await call('notify')).ok, true);
  assert.ok(f.events.some(event => event.type === 'push' && event.channel === 'notify' && event.payload.text === 'hello from plugin'));
  assert.equal((await call('confirm')).result.confirmed, true, 'confirm');
  assert.ok(prompts.some(([method, params]) => method === 'ui' && params.kind === 'confirm' && params.body === 'Proceed?'));
  const written = await call('write');
  assert.equal(written.ok, true, JSON.stringify(written));
  assert.equal(written.result.read.content, 'stored', JSON.stringify(written.result));
});

test('native plugin media deposits are not limited to one megabyte by the Host transport', sandboxed, async t => {
  const f=await fixture(t);
  const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l1sAAAAASUVORK5CYII=','base64');
  // A real PNG with a large, legal tEXt chunk exercises the declared media
  // allowance without needing a large image decoder or an external asset.
  const body=Buffer.from('fixture\0'+'A'.repeat(2<<20)),chunk=Buffer.alloc(body.length+12);
  chunk.writeUInt32BE(body.length);chunk.write('tEXt',4);body.copy(chunk,8);
  let crc=0xffffffff;for(const byte of chunk.subarray(4,-4)){crc^=byte;for(let bit=0;bit<8;bit++)crc=(crc>>>1)^((crc&1)?0xedb88320:0);}
  chunk.writeUInt32BE((crc^0xffffffff)>>>0,chunk.length-4);
  const image=Buffer.concat([png.subarray(0,-12),chunk,png.subarray(-12)]);
  const main="cindy.onHostMessage(async message=>{if(message.type!=='tool-call')return;const blob=await(await fetch('large.png')).blob();const data=await new Promise((resolve,reject)=>{const r=new FileReader();r.onload=()=>resolve(r.result.split(',')[1]);r.onerror=reject;r.readAsDataURL(blob);});const result=await cindy.send({type:'cindy-request',kind:'deposit_media',data});await cindy.send({type:'tool-result',callId:message.callId,ok:true,result});});";
  await f.install({cindy:{media:['deposit']},tools:[{name:'deposit',description:'Deposit fixture media',parameters:{type:'object',properties:{}}}]},{'main.js':main,'large.png':image});
  const result=await f.host.call('agent-call',{ghostId:'fixture',tool:'deposit',args:{},session:session(f.directory)});
  assert.equal(result.ok,true,JSON.stringify(result));
  assert.equal(result.result.ok,true,JSON.stringify(result));
  assert.equal(result.result.bytes,image.length);
  const media=await f.host.call('media-path',{url:result.result.url});
  assert.deepEqual(await fs.readFile(media.path),image);
});

test('task views retain the requested task and schedule requests produce unsaved automation drafts', sandboxed, async t => {
  const f=await fixture(t,async(method,input)=>method==='session-snapshot'?{workingDir:'workspace',model:'fixture'}:null);
  const main="cindy.onHostMessage(async message=>{if(message.type!=='tool-call')return;const view=await cindy.taskView({sessionId:'task-a'});const draft=await cindy.agent.requestSchedule({name:'Reminder',prompt:'Check the status',intervalMs:1000});await cindy.send({type:'tool-result',callId:message.callId,ok:true,result:{view,draft}});});";
  const installed=await f.install({agent:{schedule:true},taskView:{html:'task.html',title:'Task tools'},tools:[{name:'open',description:'Open',parameters:{type:'object',properties:{}}}]},{'main.js':main,'task.html':'<html><head></head><body>Task tools</body></html>'});
  const result=await f.host.call('agent-call',{ghostId:'fixture',tool:'open',session:session(f.directory)});
  assert.equal(result.ok,true,JSON.stringify(result));
  assert.deepEqual(result.result.view,{ok:true,sessionId:'task-a'});
  assert.equal(result.result.draft.ok,true);
  const open=f.events.find(event=>event.channel==='task-view');
  assert.equal(open.payload.instanceId,installed.pluginId);assert.equal(open.payload.sessionId,'task-a');
  const draft=f.events.find(event=>event.channel==='schedule-draft').payload;
  assert.equal(draft.name,'Reminder');assert.equal(draft.intervalMs,30*60_000);assert.match(draft.prompt,/Check the status/);
  const page=await f.host.call('page-session',{pluginId:installed.pluginId,page:'task',sessionId:'task-a'});
  assert.equal(new URL(page.url).searchParams.get('session'),'task-a');
  assert.match(await (await fetch(page.url)).text(),/Task tools/);
});

test('a stopped task cancels its plugin call', sandboxed, async t => {
  const f = await fixture(t);
  await f.install({tools}, {'main.js': echo});
  const running = f.host.call('agent-call', {ghostId: 'fixture', tool: 'slow', args: {}, cancelKey: 'k1', session: session(f.directory)});
  await new Promise(resolve => setTimeout(resolve, 2000));
  await f.host.call('agent-cancel', {cancelKey: 'k1'});
  const result = await running;
  assert.equal(result.ok, false);
  assert.match(result.errorCode, /CANCEL|ABORT/);
});

test('a bundled Node worker answers JSON-RPC and stops with the plugin', sandboxed, async t => {
  const f = await fixture(t);
  const worker = "const readline = require('node:readline');readline.createInterface({input: process.stdin}).on('line', line => {const request = JSON.parse(line);process.stdout.write(JSON.stringify({jsonrpc: '2.0', id: request.id, result: {pid: process.pid, echo: request.params.value, runtime: typeof process.parentPort}}) + '\\n');});";
  const main = "cindy.onHostMessage(async message => {if (message.type !== 'tool-call') return;const response = await cindy.node.request({method: 'test/echo', callId: message.callId, params: {value: message.args.text}});return cindy.send({type: 'tool-result', callId: message.callId, ok: true, result: response});});";
  const installed = await f.install({tools: [{name: 'echo', description: 'Echo', parameters: {type: 'object', properties: {text: {type: 'string'}}}}], node: {entry: 'node/worker.cjs', protocol: 'json-rpc-stdio'}}, {'main.js': main, 'node/worker.cjs': worker});
  const result = await f.host.call('agent-call', {ghostId: 'fixture', tool: 'echo', args: {text: 'from node'}, session: session(f.directory)});
  assert.equal(result.ok, true, JSON.stringify(result));
  assert.equal(result.result.ok, true, JSON.stringify(result.result));
  assert.equal(result.result.result.echo, 'from node');
  assert.equal(result.result.result.runtime, 'undefined', 'plugin code must not reach the host channel');
  const pid = result.result.result.pid;
  await f.host.call('enabled', {pluginId: installed.pluginId, enabled: false});
  assert.throws(() => process.kill(pid, 0), 'disabling must await the actual Node process exit');
  await f.host.call('enabled', {pluginId: installed.pluginId, enabled: true});
  const restarted = await f.host.call('agent-call', {ghostId:'fixture', tool:'echo', args:{text:'again'}, session:session(f.directory)});
  assert.equal(restarted.result.result.echo, 'again');
  assert.notEqual(restarted.result.result.pid, pid);
});

test('a card is sanitized, published to the window and resumes through its action', sandboxed, async t => {
  const f = await fixture(t);
  const main = "cindy.onHostMessage(async message => {if (message.type === 'tool-call') { await cindy.send({type: 'card-update', callId: message.callId, html: '<div>card<script>alert(1)</script><button data-ghost-action=\"go\">Go</button></div>', state: 'done'}); return cindy.send({type: 'tool-result', callId: message.callId, ok: true, result: {done: true}}); } if (message.type === 'event' && message.name === 'card-action') { await cindy.send({type: 'card-update', callId: message.callId, html: '<div>clicked</div>', state: 'done'}); }});";
  await f.install({tools: [{name: 'card', description: 'Card', parameters: {type: 'object', properties: {}}}], card: {version: 2}}, {'main.js': main});
  const result = await f.host.call('agent-call', {ghostId: 'fixture', tool: 'card', args: {}, session: session(f.directory)});
  assert.equal(result.ok, true, JSON.stringify(result));
  const card = f.events.find(event => event.type === 'push' && event.channel === 'card');
  assert.ok(card, 'card pushed');
  assert.doesNotMatch(card.payload.html, /script/);
  assert.match(card.payload.html, /card/);
  const dispatched=await f.host.call('card-action',{callId:card.payload.callId,action:'go'});
  assert.equal(dispatched.ok,true,JSON.stringify(dispatched));
  const deadline=Date.now()+5000;
  while(!f.events.some(event=>event.channel==='card'&&event.payload.html.includes('clicked'))&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,10));
  assert.ok(f.events.some(event=>event.channel==='card'&&event.payload.html.includes('clicked')),'the real plugin did not receive its native card action');
});

test('a page reaches the logic through the channel relay and its own endpoints', sandboxed, async t => {
  const f = await fixture(t);
  const main = "const c = new BroadcastChannel('fx'); c.onmessage = e => c.postMessage({answer: e.data.ask + 1}); cindy.onHostMessage(() => {});";
  const installed = await f.install({panel: {html: 'panel.html', title: 'Panel'}}, {'main.js': main, 'panel.html': '<!doctype html><html><head></head><body>panel</body></html>'});
  const {url} = await f.host.call('page-session', {pluginId: installed.pluginId, page: 'panel'});
  const html = await (await fetch(url)).text();
  assert.match(html, /__page_shim__/);
  assert.match(html, /panel/);
  const root = new URL(url).origin + new URL(url).pathname.split('/').slice(0, 2).join('/');
  assert.match(await (await fetch(root + '/__page_shim__')).text(), /BroadcastChannel/);
  const stream = await fetch(root + '/__bc/stream');
  const reader = stream.body.getReader();
  await new Promise(resolve => setTimeout(resolve, 500));
  assert.equal((await fetch(root + '/__bc/post', {method: 'POST', body: JSON.stringify({channel: 'fx', data: {ask: 41}})})).status, 204);
  let text = '';
  const deadline = Date.now() + 10000;
  while (!text.includes('"answer":42') && Date.now() < deadline) {
    const {value, done} = await Promise.race([reader.read(), new Promise(resolve => setTimeout(() => resolve({done: true}), Math.max(deadline - Date.now(), 1)))]);
    if (done) break;
    text += Buffer.from(value).toString();
  }
  await reader.cancel();
  assert.match(text, /"answer":42/);
});

test('a will-user-message hook can rewrite or block, and a $command gains its directive', sandboxed, async t => {
  const f = await fixture(t);
  const main = "cindy.onHostMessage(async m => { if (m.type !== 'event') return; const text = m.data.text; await cindy.send({type: 'event-verdict', hookId: m.hookId, action: text.includes('stop') ? 'block' : text.includes('swap') ? 'rewrite' : 'allow', reason: 'fixture', text: 'swapped'}); });";
  await f.install({tools, command: 'fix', launch: 'resident', subscribe: {hooks: ['will-user-message']}}, {'main.js': main});
  assert.equal((await f.host.call('screen-user', {sessionId: 's', text: 'hello'})).action, 'allow');
  const rewritten = await f.host.call('screen-user', {sessionId: 's', text: 'please swap'});
  assert.equal(rewritten.action, 'rewrite');
  assert.equal(rewritten.text, 'swapped');
  assert.equal((await f.host.call('screen-user', {sessionId: 's', text: 'stop now'})).action, 'block');
  assert.match((await f.host.call('expand-command', {text: '$fix do it'})).text, /插件指令/);
});

test('the upstream MCP switchboard reaches an installed plugin through the real worker adapter',sandboxed,async t=>{
 const f=await fixture(t);
 await f.install({tools},{'main.js':echo});
 const server=http.createServer(async(req,res)=>{
  const chunks=[];for await(const part of req)chunks.push(part);
  const {method,params}=JSON.parse(Buffer.concat(chunks).toString());
  if(method==='agent-call')assert.equal(params.ghostId,'fixture',JSON.stringify(params));
  const result=await f.host.call(method,{...params,session:session(f.directory)});
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify(result));
 }).listen(0,'127.0.0.1');await once(server,'listening');t.after(()=>server.close());
 const {tools:agentTools}=await ghostAgentTools({ghostBridge:{baseUrl:`http://127.0.0.1:${server.address().port}`,key:'x'.repeat(32)},model:{}},async()=>{});
 const result=await agentTools.find(t=>t.name==='ghost_call').execute('call-actual',{ghost_id:'fixture',tool:'echo',args:{text:'via switchboard'}});
 assert.match(JSON.stringify(result),/via switchboard/);
});

test('a plugin installed before host restart remains callable through fresh discovery', sandboxed, async t => {
 const f=await fixture(t);
 await f.install({tools},{'main.js':echo});
 const reopened=await f.reopen();
 assert.equal((await reopened.call('agent-roster',{}))[0].id,'fixture');
 const result=await reopened.call('agent-call',{ghostId:'fixture',tool:'echo',args:{text:'after restart'},session:session(f.directory)});
 assert.equal(result.ok,true,JSON.stringify(result));
 assert.equal(result.result.said,'after restart');
});

test('the native model picker resolves the real plugin request to the exact provider', sandboxed, async t => {
 const f=await fixture(t,async(method,params)=>{
  if(method==='models')return [{id:'same-model',providerId:'chosen-provider'}];
  if(method==='ui'&&params.kind==='model-picker')return {route:{agentKind:'pi',providerId:'chosen-provider',model:'same-model',effort:'',fastMode:false}};
  return null;
 });
 await f.install({tools:[{name:'pick',description:'Choose',parameters:{type:'object',properties:{}}}]},{'main.js':"cindy.onHostMessage(async m=>{if(m.type==='tool-call'){const result=await cindy.pickModel();await cindy.send({type:'tool-result',callId:m.callId,ok:true,result});}})"});
 const result=await f.host.call('agent-call',{ghostId:'fixture',tool:'pick',args:{},session:session(f.directory)});
 assert.equal(result.ok,true,JSON.stringify(result));
 assert.equal(result.result.ok,true,JSON.stringify(result.result));
 assert.equal(result.result.route.providerId,'chosen-provider');
});

test('the library stores files and rows under its own root', sandboxed, async t => {
  const f = await fixture(t);
  await f.install({tools, library: true}, {'main.js': echo});
  const result = await f.host.call('agent-call', {ghostId: 'fixture', tool: 'library', args: {}, session: session(f.directory)});
  assert.equal(result.ok, true, JSON.stringify(result));
  assert.equal(result.result.put.ok, true, JSON.stringify(result.result));
  assert.equal(result.result.got.ok, true, JSON.stringify(result.result));
  assert.equal(result.result.sql.ok, true, JSON.stringify(result.result));
});
