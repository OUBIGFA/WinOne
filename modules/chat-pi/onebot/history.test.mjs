// Search real Pi transcripts through the runtime's private, Host-bound scope.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {createOneBotRuntime} from './runtime.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('Run through build/test-onebot.mjs with an isolated output');
async function fixture(){
 const directory=await fs.mkdtemp(path.join(base,'history-'));
 const runtime=createOneBotRuntime({directory});
 await runtime.call('profiles.create',{input:{id:'reader',name:'Reader'}});
 await runtime.call('profiles.create',{input:{id:'other',name:'Other'}});
 const sessions=[];
 for(const [id,botId,role,title] of [['A'.repeat(26),'reader','canonical','旅行计划'],['B'.repeat(26),'reader','channel','Hidden'],['C'.repeat(26),'other','canonical','Other']]){
  const sessionDirectory=path.join(directory,'history',id);await fs.mkdir(sessionDirectory,{recursive:true});
  const manager=SessionManager.create(directory,sessionDirectory);
  manager.appendCustomEntry('winone.input',{id:'visible-input-'+id,text:'住宿预算 receipt',attachments:[]});
  manager.appendMessage({role:'user',content:[{type:'text',text:'Invisible attachment payload'}],timestamp:1});
  const messageId=manager.appendMessage({role:'assistant',content:[{type:'text',text:'已核对住宿预算 receipt。'},{type:'thinking',thinking:'private-reasoning'}],api:'openai-completions',provider:'fixture',model:'fixture',usage:{input:1,output:1,cacheRead:0,cacheWrite:0,totalTokens:2,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},stopReason:'stop',timestamp:2});
  manager.appendMessage({role:'user',content:'[UI_ACTION_TRIGGER] hidden-control',timestamp:3});
  manager.appendMessage({role:'toolResult',toolCallId:'hidden',toolName:'read',content:[{type:'text',text:'private-tool-output'}],isError:false,timestamp:4});
  await runtime.call('sessions.bind',{botId,sessionId:id,role,routeKey:role==='canonical'?'canonical':'group:fixture'});
  sessions.push({id,title,sessionDirectory,sessionFile:manager.getSessionFile(),directory,createdAt:'2026-10-26T08:00:00Z',updatedAt:'2026-10-26T09:00:00Z',archived:false,messageId});
 }
 return {runtime,sessions,directory};
}

test('history uses upstream CJK/full-text and title ranking without exposing hidden content or changing transcripts',async()=>{
 const f=await fixture();
 try{
  const session=f.sessions[0],before=await fs.readFile(session.sessionFile);
  const query=q=>f.runtime.call('history.query',{botId:'reader',query:q,sessions:[session],limit:20});
  const result=await query('住宿预算');
  assert.equal(result.results.length,1);
  assert.equal(result.results[0].session.id,session.id);
  assert.equal(result.results[0].contentHits.length,2);
  assert.equal(result.results[0].session._count.messages,2);
  assert.ok(result.results[0].contentHits.every(hit=>hit.messageClientId&&hit.messageId));
  assert.equal((await query('旅行')).results[0].matchKind,'title');
  for(const hidden of ['Invisible','private-reasoning','hidden-control','private-tool-output'])assert.equal((await query(hidden)).results.length,0,hidden);
  assert.deepEqual(await fs.readFile(session.sessionFile),before);
  assert.equal(result.vectorUsed,false);
  assert.match(result.vectorSkipReason,/未启用/);
 }finally{await f.runtime.close();}
});

test('history refuses another Bot, hidden channels, duplicate sessions and paths outside the registered transcript directory',async()=>{
 const f=await fixture();
 try{
  for(const sessions of [[f.sessions[1]],[f.sessions[2]],[f.sessions[0],f.sessions[0]]]){
   await assert.rejects(f.runtime.call('history.query',{botId:'reader',query:'receipt',sessions}),error=>error.code==='INVALID_SCOPE');
  }
  const bad={...f.sessions[0],sessionFile:f.sessions[2].sessionFile};
  await assert.rejects(f.runtime.call('history.query',{botId:'reader',query:'receipt',sessions:[bad]}),error=>error.code==='INVALID_PATH');
  await assert.rejects(f.runtime.call('history.query',{botId:'missing',query:'receipt',sessions:[]}),/不存在/);
 }finally{await f.runtime.close();}
});

test('history reports incomplete transcripts and sees edits on the next search',async()=>{
 const f=await fixture();
 try{
  const session=f.sessions[0],before=await fs.readFile(session.sessionFile,'utf8');
  await fs.appendFile(session.sessionFile,'{"type":');
  await assert.rejects(f.runtime.call('history.query',{botId:'reader',query:'receipt',sessions:[session]}),error=>error.code==='HISTORY_UNAVAILABLE');
  await fs.writeFile(session.sessionFile,before,'utf8');
  assert.equal((await f.runtime.call('history.query',{botId:'reader',query:'receipt',sessions:[session]})).results.length,1);
 }finally{await f.runtime.close();}
});
