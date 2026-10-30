import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {Database} from 'bun:sqlite';
import {createIMSecrets} from './im-secrets.mjs';
import {createOneBotPaths} from './content-service.mjs';
import {createOneBotIMService} from './im-service.mjs';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
const key=Buffer.alloc(32,7).toString('base64');
const root=()=>fs.mkdtemp(path.join(base,'im-host-'));
// These transport-only fixtures have no Chat sessions; lane behavior is tested
// with the real original service and SQLite in im-lanes.test.mjs.
const emptyLanes=async method=>{assert.equal(method,'retire');return {archivedSessionIds:[]};};

test('IM encrypted KV preserves originals and reports decryption failure rather than missing credentials',async()=>{
 const directory=await root(),paths=createOneBotPaths(directory),warnings=[];
 let vault=createIMSecrets(paths,key,{warn:message=>warnings.push(message)});
 assert.equal(vault.write('fixture-token','private-fixture-token'),true);
 assert.deepEqual(vault.readResult('fixture-token'),{kind:'value',value:'private-fixture-token'});
 assert.deepEqual(vault.readResult('missing'),{kind:'missing'});
 const [file]=await fs.readdir(path.join(directory,'im-secrets'));
 const bytes=await fs.readFile(path.join(directory,'im-secrets',file));assert.equal(bytes.includes(Buffer.from('private-fixture-token')),false);
 vault.close();assert.deepEqual(vault.readResult('fixture-token'),{kind:'error'});
 vault=createIMSecrets(paths,key,{warn:()=>{}});assert.equal(vault.read('fixture-token'),'private-fixture-token');vault.close();
 const wrong=createIMSecrets(paths,Buffer.alloc(32,8).toString('base64'),{warn:()=>{}});assert.deepEqual(wrong.readResult('fixture-token'),{kind:'error'});wrong.close();
});

function fakeFactory(instances){return(host,record)=>{
 let status={kind:'idle'};const prefix='bot-im-'+record.id+'-';
 const statusHandlers=new Set(),messageHandlers=new Set();
 const update=value=>{status=value;for(const listener of statusHandlers)listener(value);};
 const im={
  onMessage:listener=>{messageHandlers.add(listener);return()=>messageHandlers.delete(listener);},
  onStatusChange:listener=>{statusHandlers.add(listener);return()=>statusHandlers.delete(listener);},
  getStatus:()=>status,getConfigState:()=>({status,botUsername:'fixture',ownerUserId:'10000'}),
  async setConfig({token,ownerUserId}){host.secrets.write(prefix+'telegram-bot-token',token);host.secrets.write(prefix+'telegram-owner-user-id',ownerUserId);update(token==='bad'?{kind:'error',reason:'invalid-token'}:{kind:'connected',appId:'fixture-app'});return {status};},
  async init(){if(host.secrets.read(prefix+'telegram-bot-token'))update({kind:'connected',appId:'fixture-app'});},
  async dispose(){update({kind:'idle'});},async disconnect(){update({kind:'idle'});},async sendText(){return {messageId:'reply'};},
  async sendMarkdownText(_id,text){im.sent.push(text);return {messageId:'reply'};},sent:[],
  receive:message=>{for(const listener of messageHandlers)listener(message);},
 };instances.push(im);return im;
};}

test('Cindy connection records retain encrypted identities, reject duplicates and roll back failed setup',async()=>{
 const directory=await root(),instances=[];
 const create=()=>createOneBotIMService({directory,masterKey:key,readBot:async()=>({status:'active'}),dispatch:async()=>({text:'done'}),lanes:emptyLanes,factory:fakeFactory(instances)});
 let hub=create();
 try{
  await assert.rejects(hub.call('add',{botId:'alpha',channel:'telegram',token:'bad',ownerUserId:'10000'}),/invalid-token/);
  assert.equal((await hub.call('list')).connections.length,0);
  const result=await hub.call('add',{botId:'alpha',channel:'telegram',token:'good',ownerUserId:'10000'});
  assert.equal(result.connection.status.kind,'connected');
  assert.equal(JSON.stringify(result).includes('good'),false);
  await assert.rejects(hub.call('add',{botId:'beta',channel:'telegram',token:'good',ownerUserId:'10000'}),/已连接/);
  await hub.close();hub=create();await hub.startSaved();
  const saved=(await hub.call('list')).connections[0];assert.equal(saved.status.kind,'connected');assert.equal(saved.id,result.connection.id);
  await hub.call('remove',{id:saved.id});assert.equal((await hub.call('list')).connections.length,0);
 }finally{await hub.close();}
});

test('lifecycle pauses only its Bot, resumes saved connections and removes only its credentials',async()=>{
 const directory=await root(),instances=[];
 const hub=createOneBotIMService({directory,masterKey:key,readBot:async()=>({status:'active'}),dispatch:async()=>({text:'done'}),lanes:emptyLanes,factory:fakeFactory(instances)});
 try{
  await hub.call('add',{botId:'alpha',channel:'telegram',token:'alpha-token',ownerUserId:'10000'});
  await hub.call('add',{botId:'beta',channel:'telegram',token:'beta-token',ownerUserId:'10000'});
  await hub.pauseBot('alpha');
  let connections=(await hub.call('list')).connections;
  assert.equal(connections.find(c=>c.botId==='alpha').status.kind,'idle');
  assert.equal(connections.find(c=>c.botId==='beta').status.kind,'connected');
  await hub.resumeBot('alpha');
  assert.equal((await hub.call('list')).connections.find(c=>c.botId==='alpha').status.kind,'connected');
  await hub.removeBot('alpha');
  connections=(await hub.call('list')).connections;assert.equal(connections.length,1);assert.equal(connections[0].botId,'beta');
  // Its old token can be configured again only after deletion removed the key.
  await hub.call('add',{botId:'gamma',channel:'telegram',token:'alpha-token',ownerUserId:'10000'});
 }finally{await hub.close();}
});

test('protected IM attachments are discarded after execution and never enter the durable queue',async()=>{
 const directory=await root(),instances=[];let transportHost;
 const create=fakeFactory(instances);
 const hub=createOneBotIMService({directory,masterKey:key,readBot:async()=>({status:'active'}),lanes:emptyLanes,
  factory:(host,record)=>{transportHost=host;return create(host,record);},dispatch:async({message})=>{
   assert.equal(await fs.readFile(message.attachments[0].absPath,'utf8'),'PRIVATE_PROTECTED_MEDIA');return {text:'processed'};
  }});
 try{
  await hub.call('add',{botId:'alpha',channel:'telegram',token:'protected-token',ownerUserId:'10000'});
  const file=await transportHost.files.stage({buffer:Buffer.from('PRIVATE_PROTECTED_MEDIA'),fileName:'private.txt'});
  instances[0].receive({senderId:'10000',messageId:'protected-one',text:'protected',protectedContent:true,attachments:[{absPath:file.absPath}],unsupported:[]});
  for(let attempt=0;attempt<100&&instances[0].sent.length===0;attempt++)await new Promise(resolve=>setTimeout(resolve,5));
  assert.deepEqual(instances[0].sent,['processed']);
  await assert.rejects(fs.stat(file.absPath),error=>error.code==='ENOENT');
  const database=new Database(path.join(directory,'im-inbox.db'),{readonly:true});
  try{assert.equal(database.query('SELECT count(*) AS count FROM inbox').get().count,0);}finally{database.close();}
 }finally{await hub.close();}
});

test('IM adapter sends the actual dispatch result and keeps channel lanes serialized',async()=>{
 const directory=await root(),instances=[],order=[];
 const hub=createOneBotIMService({directory,masterKey:key,readBot:async()=>({status:'active'}),lanes:emptyLanes,factory:fakeFactory(instances),dispatch:async({message})=>{order.push(message.text);return {text:'reply:'+message.text};}});
 try{
  await hub.call('add',{botId:'alpha',channel:'telegram',token:'good',ownerUserId:'10000'});
  const im=instances[0];
  for(const text of ['one','two'])im.receive({senderId:'10000',text,attachments:[],unsupported:[],messageId:text});
  for(let attempt=0;attempt<100&&im.sent.length<2;attempt++)await new Promise(resolve=>setTimeout(resolve,5));
  assert.deepEqual(order,['one','two']);assert.deepEqual(im.sent,['reply:one','reply:two']);
 }finally{await hub.close();}
});
