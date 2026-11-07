// Exercise real imported Cindy slots through installed packages and native sandbox.
// Only browser opening / explicit user answers are controlled at the Host boundary.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import JSZip from 'jszip';
import {BunSqliteDatabase} from './library-sqlite.mjs';
import {createLibraryDbCore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/libraryDbCore.ts';
import {libraryBlobRelPath} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/librarySlot.ts';
import {createGhostHost} from './host.mjs';

const root=fileURLToPath(new URL('../../../',import.meta.url));
const system={skip:!process.env.WINONE_GHOST_SANDBOX};
async function fixture(t,handler=async()=>null){
  const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(root,'_temp/plugin-task-close/sdk-worker/profiles');
  await fs.mkdir(base,{recursive:true});
  const directory=await fs.mkdtemp(path.join(base,'sdk-'));
  const events=[];
  const options={directory,executable:process.env.WINONE_GHOST_SANDBOX??path.join(directory,'unused.exe'),sdkFile:process.env.WINONE_GHOST_SDK??path.join(directory,'unused-sdk.js'),mailSDKFile:fileURLToPath(new URL('../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js',import.meta.url)),snapshotWorker:process.env.WINONE_GHOST_SNAPSHOT,runtimeExecutable:process.execPath,nodeWorker:fileURLToPath(new URL('./node-worker.mjs',import.meta.url)),masterKey:Buffer.alloc(32,17).toString('base64'),host:handler,event:e=>events.push(e)};
  const host=createGhostHost(options);t.after(()=>host.close());
  const session={id:'sdk-session',workingDir:directory,approved:false};
  async function install(manifest,files){
    const zip=new JSZip();zip.file('ghost.json',JSON.stringify({schemaVersion:3,minCindyVersion:'0.0.0',id:'sdk-fixture',name:'SDK fixture',version:'1.0.0',description:'SDK behavior fixture',entry:'main.js',kind:'chip',...manifest}));
    for(const [name,content] of Object.entries(files))zip.file(name,content);
    const file=path.join(directory,'sdk-fixture.cindy');await fs.writeFile(file,await zip.generateAsync({type:'nodebuffer'}));
    const inspected=await host.call('inspect',{path:file});return host.call('install',{token:inspected.token,confirmed:true});
  }
  return {host,directory,events,install,session,options,async page(id){const {url}=await host.call('settings-session',{pluginId:id});return new URL(url).origin+new URL(url).pathname.split('/').slice(0,2).join('/');},call:args=>host.call('agent-call',{ghostId:'sdk-fixture',tool:'run',args,session})};
}
const tools=[{name:'run',description:'Run SDK check',parameters:{type:'object',properties:{}}}];
const rpcMain="cindy.onHostMessage(async m=>{if(m.type!=='tool-call')return;const result=await cindy.node.request({...m.args,callId:m.callId});await cindy.send({type:'tool-result',callId:m.callId,ok:true,result});});";
const rpcWorker=body=>"const rl=require('node:readline');const send=v=>process.stdout.write(JSON.stringify(v)+'\\n');rl.createInterface({input:process.stdin}).on('line',async line=>{const m=JSON.parse(line);try{"+body+"}catch{send({jsonrpc:'2.0',id:m.id,error:{code:-32000,message:'fixture failed'}});}});";

test('SDK Node authorization opens the exact declared URL and rejects failed/cancelled opening',system,async t=>{
  const opened=[];let deny=false;
  const f=await fixture(t,async(method,input)=>{if(method==='open-external'){if(deny)throw Error('Browser unavailable');opened.push(input.url);}if(method==='ui'&&input.kind==='confirm')return {confirmed:true};return null;});
  await f.install({tools,node:{entry:'node/worker.cjs',protocol:'json-rpc-stdio'},network:{hosts:['accounts.example.com']}},{'main.js':rpcMain,'node/worker.cjs':rpcWorker("const authorize=m.method==='legacy'?globalThis.__CINDY_NODE__.bindDeviceAuthorization():globalThis.__CINDY_NODE__.bindAuthorization();const result=await authorize(m.method==='legacy'?'https://accounts.example.com/device':{kind:'browser',url:'https://accounts.example.com/login'});send({jsonrpc:'2.0',id:m.id,result:{opened:result?.kind==='opened'||m.method==='legacy'}});")});
  for(const method of ['legacy','generic']){const result=await f.call({method,cancelWithCall:true});assert.equal(result.result.ok,true,JSON.stringify(result));assert.equal(result.result.result.opened,true);}
  assert.deepEqual(opened,['https://accounts.example.com/device','https://accounts.example.com/login']);
  deny=true;const failed=await f.call({method:'generic',cancelWithCall:true});assert.equal(failed.result.ok,false,JSON.stringify(failed));
  const count=opened.length;const unbound=await f.call({method:'generic'});assert.equal(unbound.result.ok,false);assert.equal(opened.length,count,'background RPC must not open a browser');
});

test('SDK Node uses private credentials only for declared methods and supports extra entries, children and crash recovery',system,async t=>{
  const f=await fixture(t,async(method,input)=>method==='ui'&&input.kind==='secret'?{value:'private-worker-secret'}:null);
  const worker=rpcWorker("if(m.method==='crash'){process.exit(7);return;}if(m.method==='child'){const c=await globalThis.__CINDY_NODE__.spawnEntry('node/child.cjs',['fixture-arg']);let text='';c.stdout.on('data',b=>text+=b);await new Promise(r=>c.once('exit',r));send({jsonrpc:'2.0',id:m.id,result:JSON.parse(text)});return;}send({jsonrpc:'2.0',id:m.id,result:{pid:process.pid,bound:m.cindy?.secrets?.credential==='private-worker-secret',input:m.cindy?.secretInputCompleted===true,parent:typeof process.parentPort}});");
  const installed=await f.install({tools,settingsHtml:'settings.html',setup:{requires:[]},node:{entry:'node/worker.cjs',entries:['node/extra.cjs','node/child.cjs'],protocol:'json-rpc-stdio',childSpawn:true,secretBindings:[{key:'credential',label:'Credential',methods:['auth']}] }},{'main.js':rpcMain,'settings.html':'<html><body>Credentials</body></html>','node/worker.cjs':worker,'node/extra.cjs':worker,'node/child.cjs':"process.stdout.write(JSON.stringify({pid:process.pid,arg:process.argv[2],api:typeof globalThis.__CINDY_NODE__}));"});
  const auth=await f.call({method:'auth',cancelWithCall:true,promptSecrets:true});assert.equal(auth.result.ok,true,JSON.stringify(auth));assert.equal(auth.result.result.bound,true);assert.equal(auth.result.result.input,true);assert.equal(auth.result.result.parent,'undefined');
  const plain=await f.call({method:'plain',params:{cindy:{secrets:{credential:'forged'}}}});assert.equal(plain.result.result.bound,false);
  const extra=await f.call({method:'plain',entry:'node/extra.cjs'});assert.equal(extra.result.ok,true,JSON.stringify(extra));assert.notEqual(extra.result.result.pid,auth.result.result.pid);
  assert.equal((await f.call({method:'plain',entry:'node/undeclared.cjs'})).result.ok,false);
  const child=await f.call({method:'child',cancelWithCall:true});assert.equal(child.result.ok,true,JSON.stringify(child));assert.equal(child.result.result.arg,'fixture-arg');assert.equal(child.result.result.api,'undefined');assert.throws(()=>process.kill(child.result.result.pid,0));
  const crash=await f.call({method:'crash'});assert.equal(crash.result.ok,false);
  const recovered=await f.call({method:'plain'});assert.equal(recovered.result.ok,true,JSON.stringify(recovered));
  await f.host.call('enabled',{pluginId:installed.pluginId,enabled:false});for(const pid of [recovered.result.result.pid,extra.result.result.pid])assert.throws(()=>process.kill(pid,0));
  await f.host.call('enabled',{pluginId:installed.pluginId,enabled:true});assert.equal((await f.call({method:'plain'})).result.ok,true);
});

test('SDK dynamic connections keep tokens private, gate new hosts and advance setup revision',system,async t=>{
  let confirmed=false;const prompts=[];
  const f=await fixture(t,async(method,input)=>{if(method==='ui'){prompts.push(input);return {confirmed};}return null;});
  const installed=await f.install({tools,settingsHtml:'settings.html',network:{connections:[{key:'service',label:'Service',maxConnections:2,inject:{header:'Authorization',format:'Bearer {value}'}}]}},{'main.js':'cindy.onHostMessage(()=>{});','settings.html':'<html><body>Connections</body></html>'});
  const page=await f.page(installed.pluginId);
  const request=(route,input)=>fetch(page+route,{method:input?'POST':'GET',...(input?{body:JSON.stringify(input)}:{})});
  const initial=await f.host.call('setup',{pluginId:installed.pluginId});
  assert.equal((await(await request('/connections/service',{host:'api.example.com',token:'private-connection-token'})).json()).error,'CONFIRM_DENIED');
  confirmed=true;const added=await(await request('/connections/service',{host:'api.example.com',token:'private-connection-token'})).json();assert.equal(added.ok,true);
  assert.ok((await f.host.call('setup',{pluginId:installed.pluginId})).revision>initial.revision,'committed connection must invalidate setup assessment');
  const before=prompts.length;const updated=await(await request('/connections/service',{host:'API.EXAMPLE.COM',token:'changed-private-token'})).json();assert.equal(updated.connection.id,added.connection.id);assert.equal(prompts.length,before,'same host token replacement must not expand authorization');
  const body=await(await request('/connections')).text();assert.doesNotMatch(body,/private-connection-token|changed-private-token/);
  assert.equal((await request('/connections/service',{host:'127.0.0.1',token:'x'})).status,400);
  assert.equal((await fetch(page+'/connections/service/'+added.connection.id,{method:'DELETE'})).status,204);
  assert.deepEqual((await(await request('/connections')).json())[0].connections,[]);
});

test('SDK SQL rejects extra statements and preserves batch rollback and named bindings',async t=>{
  const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(root,'_temp/plugin-task-close/sdk-worker/profiles');await fs.mkdir(base,{recursive:true});
  const directory=await fs.mkdtemp(path.join(base,'sql-'));const file=path.join(directory,'library.sqlite');
  const core=createLibraryDbCore({DatabaseCtor:BunSqliteDatabase});t.after(()=>core.closeAll());
  assert.equal((await core.openDatabase(file)).ok,true);
  assert.equal(core.execStatement(file,'CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)').ok,true);
  assert.equal(core.execStatement(file,'INSERT INTO t(id,v) VALUES (:id,:v)',{id:1,v:'named'}).ok,true);
  assert.equal(core.batch(file,[{sql:'INSERT INTO t VALUES (?,?)',params:[2,'new']},{sql:'INSERT INTO t VALUES (?,?)',params:[3,'named']}]).ok,false);
  assert.deepEqual(core.execStatement(file,'SELECT id,v FROM t').rows,[{id:1,v:'named'}]);
  assert.equal(core.execStatement(file,'SELECT 1; DELETE FROM t').ok,false,'SQLite adapter must reject multiple statements, not silently execute a subset');
  assert.equal(core.execStatement(file,"ATTACH DATABASE 'other.sqlite' AS other").code,'DB_STATEMENT_REJECTED');
});

test('SDK local app context follows the Host and session context is re-minted instead of accepting plugin values',system,async t=>{
 const f=await fixture(t);
 const main="let changed; cindy.onHostMessage(async m=>{if(m.type==='host-context-changed'){changed=m.context;return;}if(m.type!=='tool-call')return;const result={app:await cindy.request({kind:'app-context'}),session:m.args.session_context,changed};await cindy.send({type:'tool-result',callId:m.callId,ok:true,result});});";
 const installed=await f.install({tools,sessionContext:true,settingsHtml:'settings.html'},{'main.js':main,'settings.html':'<html><body>Local context</body></html>'});
 const first=await f.call({session_context:{session_id:'forged',workdir:'forged',workdir_is_local:true,workdir_is_read_only:false}});
 assert.deepEqual(first.result.app,{ok:true,context:{region:'global',locale:'zh-CN',uiLocale:'zh-CN'}});
 assert.equal(first.result.session.session_id,f.session.id);assert.equal(first.result.session.workdir,f.directory);
 await f.host.call('models',{language:'en-US'});
 const next=await f.host.call('agent-call',{ghostId:'sdk-fixture',tool:'run',args:{session_context:{workdir_is_read_only:false}},session:{...f.session,readOnly:true}});
 assert.equal(next.result.session.workdir_is_read_only,true);assert.equal(next.result.app.context.uiLocale,'en');assert.equal(next.result.changed.uiLocale,'en');
 const page=await f.page(installed.pluginId);
 assert.deepEqual(await(await fetch(page+'/app-context')).json(),{ok:true,context:{region:'global',locale:'en',uiLocale:'en'}});
 assert.equal((await fetch(page+'/app-context',{method:'POST',body:'{}'})).status,405);
 assert.deepEqual(Object.keys(next.result.app.context).sort(),['locale','region','uiLocale']);
});

test('SDK Library streams, staging, SQLite, private file reads and relocation survive restart',system,async t=>{
  const f=await fixture(t,async(method,input)=>method==='ui'&&input.kind==='confirm'?{confirmed:true}:null);
  const installed=await f.install({tools,library:true,settingsHtml:'settings.html'},{'main.js':"cindy.onHostMessage(async m=>{if(m.type!=='tool-call')return;const result=await cindy.library(m.args);await cindy.send({type:'tool-result',callId:m.callId,ok:true,result});});",'settings.html':'<html><body>Library</body></html>'});
  const call=async input=>{const outer=await f.call(input);assert.equal(outer.ok,true,JSON.stringify(outer));return outer.result;};
  const caps=await call({op:'capabilities'});assert.equal(caps.ok,true);assert.ok(caps.capabilities.operations.includes('staging.begin'));
  const write=await call({op:'write',path:'assets/text.txt',content:'stream-body'});assert.equal(write.ok,true,JSON.stringify(write));
  assert.equal((await call({op:'write',path:'../escape.txt',content:'bad'})).ok,false);
  const begin=await call({op:'writeBegin',path:'assets/chunks.txt',totalBytes:11,sha256:write.sha256});assert.equal(begin.ok,true,JSON.stringify(begin));
  assert.equal((await call({op:'writeChunk',streamId:begin.streamId,seq:1,content:'stream-body'})).ok,true);
  assert.equal((await call({op:'writeCommit',streamId:begin.streamId})).ok,true);
  const staged=await call({op:'staging.begin',taskId:'canvas-1',sourceRevision:'v1',totalBytes:11,sha256:write.sha256,mime:'text/plain',recovery:{canvas:'canvas-1'}});assert.equal(staged.ok,true,JSON.stringify(staged));
  assert.equal((await call({op:'staging.chunk',stagingId:staged.stagingId,seq:1,content:Buffer.from('stream-body').toString('base64'),encoding:'base64'})).ok,true);
  const committed=await call({op:'staging.commit',stagingId:staged.stagingId});assert.equal(committed.ok,true,JSON.stringify(committed));assert.equal(committed.durable,true);
  const stagedRead=await call({op:'staging.read',stagingId:staged.stagingId});assert.equal(stagedRead.ok,true,JSON.stringify(stagedRead));
  const ack=await call({op:'write',path:libraryBlobRelPath(write.sha256,'txt'),content:'stream-body'});
  const released=await call({op:'staging.release',stagingId:staged.stagingId,path:ack.path,sha256:ack.sha256,bytes:ack.bytes,libraryIdentity:ack.libraryIdentity,libraryGeneration:ack.libraryGeneration});assert.equal(released.ok,true,JSON.stringify(released));
  assert.equal((await call({op:'db.open',dbPath:'data.sqlite'})).ok,true);
  assert.equal((await call({op:'db.exec',dbPath:'data.sqlite',sql:'CREATE TABLE t(v TEXT)'})).ok,true);
  assert.equal((await call({op:'db.exec',dbPath:'data.sqlite',sql:'INSERT INTO t VALUES (?)',params:['persistent']})).ok,true);
  assert.deepEqual((await call({op:'db.exec',dbPath:'data.sqlite',sql:'SELECT * FROM t'})).rows,[{v:'persistent'}]);
  const page=await f.page(installed.pluginId);
  assert.equal(await(await fetch(page+'/library/assets/text.txt')).text(),'stream-body');
  const range=await fetch(page+'/library/assets/text.txt',{headers:{Range:'bytes=0-5'}});assert.equal(range.status,206);assert.equal(await range.text(),'stream');
  const candidate=f.directory+'-custom-library'; // Outside the host-managed profile, still under the test output.
  const migrated=await f.host.call('library-action',{pluginId:installed.pluginId,action:'relocate',target:candidate});assert.equal(migrated.ok,true,JSON.stringify(migrated));
  assert.equal((await call({op:'read',path:'assets/text.txt'})).content,'stream-body');
  assert.equal((await call({op:'db.open',dbPath:'data.sqlite'})).ok,true);
  assert.deepEqual((await call({op:'db.exec',dbPath:'data.sqlite',sql:'SELECT * FROM t'})).rows,[{v:'persistent'}]);
  await f.host.call('enabled',{pluginId:installed.pluginId,enabled:false});
  // Disabling owns and closes the sandbox's listener too. Keeping it alive
  // just to serve HTTP 410 would be a resource leak, not a stronger denial.
  await assert.rejects(()=>fetch(page+'/library/assets/text.txt'),error=>error.code==='ConnectionRefused');
  await f.host.call('enabled',{pluginId:installed.pluginId,enabled:true});assert.equal((await call({op:'read',path:'assets/text.txt'})).content,'stream-body');
  await f.host.close();const reopened=createGhostHost(f.options);t.after(()=>reopened.close());
  const restored=await reopened.call('agent-call',{ghostId:'sdk-fixture',tool:'run',args:{op:'read',path:'assets/text.txt'},session:f.session});assert.equal(restored.result.content,'stream-body');
  await reopened.call('remove',{pluginId:installed.pluginId,confirmed:true});assert.equal(await fs.readFile(path.join(candidate,installed.pluginId,'assets/text.txt'),'utf8'),'stream-body','uninstall must retain user Library');
});
