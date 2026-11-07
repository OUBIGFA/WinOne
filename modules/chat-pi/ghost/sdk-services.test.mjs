// Adapter regressions use the real vault, endpoint dispatchers, SQLite and
// downloader. Browser answers are controlled only at the trusted Host boundary.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {once} from 'node:events';
import {Readable} from 'node:stream';
import {createHash} from 'node:crypto';
import {createGhostAccounts} from './platform-accounts.mjs';
import {BunSqliteDatabase} from './library-sqlite.mjs';
import {createLibraryDbCore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/libraryDbCore.ts';
import {createOneBotPaths} from '../onebot/content-service.mjs';
import {createIMSecrets} from '../onebot/im-secrets.mjs';
import {GhostSetupChangeBus} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupChangeBus.ts';
import {createDownloader} from '../cindy/ghost/apps/desktop/src/main/downloader/index.ts';
import {PluginDownloadSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/downloadSlot.ts';
import {createGhostPageContext,readWinOneMessageHookContext,setGhostContextReader} from './context.mjs';
import {createGhostLibrary} from './platform-library.mjs';

async function directory(t) {
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.resolve('_temp/plugin-task-close/sdk-worker/profiles');
 await fs.mkdir(base,{recursive:true});return fs.mkdtemp(path.join(base,'service-'));
}
const log={debug(){},info(){},warn(){}};
async function accountsFixture(t) {
 const root=await directory(t),secrets=createIMSecrets(createOneBotPaths(root),Buffer.alloc(32,23).toString('base64'),log);
 const decl={authorizeUrl:'https://accounts.example.com/auth',tokenUrl:'https://accounts.example.com/token',scopes:['read','write']};
 const ghost={manifest:{id:'service-fixture',name:'Service fixture',network:{hosts:['accounts.example.com'],secrets:[{key:'acct',source:'oauth',oauth:decl}]}},enabled:true,namespace:null,approval:{state:'approved',revision:1}};
 const secretName=(id,key)=>id+'-'+key,changeBus=new GhostSetupChangeBus();
 const accounts=createGhostAccounts({root,secrets,secretName,find:id=>id==='service-fixture'?ghost:null,host:async()=>null,ui:async()=>null,kv:{read:()=>({})},changeBus,log,ownerScope:{isStable:()=>true}});
 t.after(()=>{accounts.close();secrets.close()});
 async function request(route,method='GET',body) {
  const req=Readable.from(body===undefined?[]:[Buffer.from(JSON.stringify(body))]);req.method=method;
  let status=200,text='';const res={setHeader(){},writeHead(value){status=value},end(value){text=value??''}};
  assert.equal(await accounts.endpoint(ghost,route,req,res),true);return {status,text,json:()=>JSON.parse(text)};
 }
 return {root,secrets,secretName,ghost,accounts,request,changeBus};
}

test('SDK OAuth endpoint persists client overrides, scope evidence and defaults instead of silent lock no-ops',async t=>{
 const f=await accountsFixture(t);
 assert.equal((await f.request('/oauth')).json()[0].clientConfigured,false);
 assert.equal((await f.request('/oauth/acct/client','PUT',{clientId:'private-client-id',clientSecret:'private-client-secret'})).status,204);
 assert.equal((await f.request('/oauth')).json()[0].clientConfigured,true);
 assert.equal(f.secrets.read(f.secretName('service-fixture','acct-client-id')),'private-client-id');
 assert.doesNotMatch((await f.request('/oauth')).text,/private-client/);
 f.secrets.write(f.secretName('service-fixture','acct-accounts'),JSON.stringify({defaultAccountId:'a',accounts:[{id:'a',label:'A',status:'connected',createdAt:1},{id:'b',label:'B',status:'connected',createdAt:2}]}));
 const revision=f.changeBus.currentRevision('service-fixture');
 assert.equal((await f.request('/oauth/acct/default','POST',{accountId:'missing'})).status,404);
 assert.equal((await f.request('/oauth/acct/default','POST',{accountId:'b'})).status,204);
 assert.equal((await f.request('/oauth')).json()[0].accounts.find(a=>a.id==='b').isDefault,true);
 assert.equal((await f.request('/oauth/acct/insufficient-scopes','POST',{scopes:['write']})).status,204);
 assert.equal((await f.request('/oauth')).json()[0].accounts.find(a=>a.id==='b').scopeStale,true);
 assert.ok(f.changeBus.currentRevision('service-fixture')>revision);
 const same=f.changeBus.currentRevision('service-fixture');
 await f.request('/oauth/acct/insufficient-scopes','POST',{scopes:['write']});assert.equal(f.changeBus.currentRevision('service-fixture'),same);
 assert.equal((await f.request('/oauth/acct/insufficient-scopes','POST',{scopes:['outside']})).status,400);
 await f.request('/oauth/acct/accounts/b','DELETE');assert.deepEqual((await f.request('/oauth')).json()[0].accounts.map(a=>a.id),['a']);
 await f.request('/oauth/acct/client','DELETE');assert.equal((await f.request('/oauth')).json()[0].clientConfigured,false);
 f.accounts.remove('service-fixture',f.ghost.manifest);assert.equal(f.secrets.read(f.secretName('service-fixture','acct-accounts')),null);
});

test('SDK SQLite rejects statement tails but accepts quoted semicolons, comments and triggers',async t=>{
 const file=path.join(await directory(t),'data.sqlite'),core=createLibraryDbCore({DatabaseCtor:BunSqliteDatabase});t.after(()=>core.closeAll());
 assert.equal((await core.openDatabase(file)).ok,true);
 assert.equal(core.execStatement(file,'CREATE TABLE t(id INTEGER PRIMARY KEY,v TEXT)').ok,true);
 assert.equal(core.execStatement(file,'SELECT 1; DELETE FROM t').ok,false);
 assert.equal(core.execStatement(file,"SELECT 'a;b' AS v; /* SELECT 2 */ -- tail").ok,true);
 assert.equal(core.execStatement(file,"CREATE TRIGGER tr AFTER INSERT ON t BEGIN UPDATE t SET v='trigger;value' WHERE id=NEW.id; END;").ok,true);
 assert.equal(core.execStatement(file,"INSERT INTO t VALUES(:id,:v)",{id:1,v:'named'}).ok,true);
 assert.deepEqual(core.execStatement(file,'SELECT * FROM t').rows,[{id:1,v:'trigger;value'}]);
 assert.equal(core.execStatement(file,'SELECT 1; /* harmless */ ATTACH DATABASE \'elsewhere\' AS extra').ok,false);
});

test('SDK trusted downloader streams HTTP with manual redirect, hash validation, progress and cache reuse',async t=>{
 const root=await directory(t),bytes=Buffer.alloc(256<<10,65),sha256=createHash('sha256').update(bytes).digest('hex');let requests=0;
 const server=http.createServer((req,res)=>{requests++;if(req.url==='/redirect'){res.writeHead(302,{Location:'/file'});res.end();return}res.writeHead(200,{'Content-Length':bytes.length});res.end(bytes)}).listen(0,'127.0.0.1');await once(server,'listening');t.after(()=>new Promise(r=>server.close(r)));
 const url=`http://127.0.0.1:${server.address().port}/redirect`,phases=[];
 const options={url,targetPath:path.join(root,'archive'),sha256,expectedSize:bytes.length,onProgress:p=>phases.push(p.loaded),isUrlAllowed:u=>new URL(u).origin===new URL(url).origin};
 const download=createDownloader();const result=await download(options);assert.equal(result.size,bytes.length);assert.equal(result.sha256,sha256);assert.ok(phases.length);assert.deepEqual(await fs.readFile(options.targetPath),bytes);
 const before=requests;assert.equal((await download(options)).fromCache,true);assert.equal(requests,before);
 await assert.rejects(()=>download({...options,targetPath:path.join(root,'denied'),redirect:'error'}),error=>error.code==='URL_POLICY');
});

test('SDK plugin download permission uses the actual single-hop SSRF gate, never the trusted HTTP default',async t=>{
 const root=await directory(t),events=[],ghost={enabled:true,approval:{state:'approved'},manifest:{node:{entry:'node.cjs'},network:{hosts:['127.0.0.1']}}};
 const slot=new PluginDownloadSlot({getGhost:()=>ghost,root:()=>path.join(root,'cache','plugin'),scope:()=> 'owner',send:(_id,e)=>events.push(e),download:createDownloader()});t.after(()=>slot.stopAndWait());
 const started=performance.now();
 const result=await slot.handle('plugin',{type:'download-request',kind:'start',id:'ssrf',url:'https://127.0.0.1/file',sha256:'a'.repeat(64),bytes:1});assert.equal(result.ok,false);assert.ok(events.some(e=>e.data.phase==='failed'));assert.ok(performance.now()-started<5000,'a deterministic SSRF refusal must not enter network retry backoff');
 assert.equal((await slot.withNodeDownloads('plugin',{downloadTokens:{archive:'forged'}},async()=>({ok:true}))).ok,false);
});

function pageFixture() {
 let clock=10000,answer={confirmed:true},ghost={enabled:true,approval:{revision:'original'},manifest:{name:'Page fixture',network:{secrets:[{url:'https://accounts.example.com/key'}]},node:{secretBindings:[{url:'https://worker.example.com/key'}]}}};
 const opened=[],prompts=[],warnings=[];
 const pages=createGhostPageContext({find:()=>ghost,current:captured=>ghost.enabled&&captured.approval.revision===ghost.approval.revision,
  now:()=>clock,host:async(method,input)=>{assert.equal(method,'open-external');opened.push(input.url);},ui:async(_kind,input)=>{prompts.push(input);return typeof answer==='function'?answer():answer;},log:{debug(){},warn:(...args)=>warnings.push(args)}});
 const attach=()=>pages.attach({pageId:'native-page',pluginId:'page-fixture'});
 attach();
 return {pages,opened,prompts,warnings,attach,tick(){clock+=1001},answer(value){answer=value},replace(value){ghost={...ghost,...value}},navigate:(url,focused=true)=>pages.navigation({pageId:'native-page',url,focused})};
}

test('SDK native plugin page keeps focus, declared URL and normalized external-link confirmation semantics',async()=>{
 const f=pageFixture();
 await f.navigate('https://accounts.example.com/key',false);assert.deepEqual(f.opened,[]);assert.deepEqual(f.prompts,[]);
 await f.navigate('https://accounts.example.com/key');assert.deepEqual(f.opened,['https://accounts.example.com/key']);assert.equal(f.prompts.length,0);
 f.tick();await f.navigate('https://worker.example.com/key');assert.equal(f.prompts.length,0);
 f.tick();await f.navigate('https://nested.xd.com/path');assert.equal(f.prompts.length,0);
 f.tick();await f.navigate('https://evilxd.com/a b');assert.equal(f.prompts[0].body,'https://evilxd.com/a%20b');assert.equal(f.opened.at(-1),'https://evilxd.com/a%20b');
 const count=f.opened.length;
 for(const url of ['https://user:password@example.com/','http://example.com/','javascript:alert(1)','not a URL']){f.tick();await f.navigate(url);}
 assert.equal(f.opened.length,count);assert.equal(f.prompts.length,1);
 f.tick();f.answer({confirmed:false});await f.navigate('https://example.com/cancel');assert.equal(f.opened.length,count);
 f.pages.close();
});

test('SDK native plugin page discards a late confirmation after close, replacement or disable and never opens twice',async()=>{
 for(const change of ['close','replace','disable']){
  const f=pageFixture();let resolve;f.answer(()=>new Promise(done=>resolve=done));
  const pending=f.navigate('https://example.com/one');
  await new Promise(resolve=>setImmediate(resolve));assert.equal(f.prompts.length,1);
  f.tick();await f.navigate('https://example.com/two');assert.equal(f.prompts.length,1,'one native confirmation per plugin');
  if(change==='close')f.pages.detach({pageId:'native-page'});
  else if(change==='replace')f.replace({approval:{revision:'replacement'}});
  else f.replace({enabled:false});
  resolve({confirmed:true});await pending;assert.deepEqual(f.opened,[]);
  f.pages.close();
 }
 const f=pageFixture();await f.navigate('https://accounts.example.com/key');await f.navigate('https://accounts.example.com/key');assert.equal(f.opened.length,1,'rate limit is shared with native page attempts');f.pages.close();
});

test('SDK message hook context contains only the Host model and no paths, identity or credentials',async()=>{
 setGhostContextReader(async id=>id==='active'?'vendor/model':undefined);
 assert.deepEqual(await readWinOneMessageHookContext('active'),{model:'vendor/model'});
 assert.deepEqual(await readWinOneMessageHookContext('unknown'),{});
});

test('SDK Library reports native read-only grants honestly and only the current opener can retain the shared slot',async t=>{
 const root=await directory(t),calls=[],ghosts={a:{enabled:true,approval:{state:'approved',revision:1},manifest:{id:'a',name:'A',library:true}},b:{enabled:true,approval:{state:'approved',revision:1},manifest:{id:'b',name:'B',library:true}}};
 let grant='not-granted';
 const library=createGhostLibrary({root,find:id=>ghosts[id],current:ghost=>ghost.enabled,sendToGhost:()=>true,showSave:async()=>null,log,mediaStore:{},stopNode:async()=>{},
  host:async(method,input)=>{assert.equal(method,'library-read-root');calls.push(input);return grant;}});
 t.after(()=>library.close());
 const call=(id,op)=>library.request(id,{op});
 const initial=await call('a','open');assert.equal(initial.ok,true,JSON.stringify(initial));assert.equal(initial.authorizedReadonly,false);assert.doesNotMatch(JSON.stringify(initial),/libraryRoot|\\\\libraries/);
 grant='granted';assert.equal((await call('a','status')).authorizedReadonly,true);
 assert.equal((await call('b','status')).authorizedReadonly,false,'status from another plugin does not take the slot');
 assert.equal((await call('b','open')).authorizedReadonly,true);
 assert.equal((await call('a','status')).authorizedReadonly,false);
 const before=calls.length;await library.stop('a');assert.equal(calls.length,before,'stopping the old opener cannot revoke the new owner');
 await library.stop('b');assert.deepEqual(calls.at(-1),{ghostId:'b',root:null});
 assert.equal((await call('b','status')).authorizedReadonly,false,'status after stop cannot silently regain a revoked slot');
});
