import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('Run through build/test-onebot.mjs');
test('creation preview writes nothing until the same owner invites the draft',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'creation-draft-'));
 const runtime=createOneBotRuntime({directory,masterKey:Buffer.alloc(32,3).toString('base64'),host:{
  voiceResources:async()=>({providers:[{id:'fixture',name:'Fixture',baseUrl:'https://fixture.invalid/v1',models:[{id:'model',name:'Model'}]}]}),
  creationCapabilities:async()=>({skill:[],mcp:[],toolset:[]}),
  auxiliaryText:async()=>({ok:true,text:JSON.stringify({name:'Planner',description:'I help plan the work.',background:'An experienced project planner.',conversationStyle:'Help with evidence-based plans.',avatarPrompt:'A friendly planner portrait.',skills:[],skillRefs:[],mcpRefs:[],toolsetRefs:[]})}),
 }});
 try{
  const draft=await runtime.call('bots.generateDraft',{request:{prompt:'Help with projects',modelRoute:{agentKind:'pi',providerId:'fixture',model:'model'}}});
  assert.equal(draft.name,'Planner');assert.ok(draft.token);
  assert.deepEqual(await runtime.call('profiles.list'),[]);
  const created=await runtime.call('profiles.create',{input:{creationDraftToken:draft.token,name:'My Planner',prepareInvitation:true}});
  assert.equal(created.name,'My Planner');
  const replay=await runtime.call('profiles.create',{input:{creationDraftToken:draft.token,name:'My Planner',prepareInvitation:true}});
  assert.equal(replay.id,created.id);
 }finally{await runtime.close();}
});
test('the market reads all ten pinned packages and retains an invitation receipt',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'market-'));
 const runtime=createOneBotRuntime({directory,masterKey:Buffer.alloc(32,3).toString('base64')});
 try{
  const entries=await runtime.call('market.list');assert.equal(entries.length,10);
  const selected=entries.find(entry=>entry.packageId==='engineer-bot');assert.ok(selected);
  let result=await runtime.call('market.invite',{input:{packageId:selected.packageId,packageVersion:selected.packageVersion,packageHash:selected.packageHash,locale:'en'}});
  const deadline=Date.now()+5000;
  while(result.status==='running'&&Date.now()<deadline){await new Promise(resolve=>setTimeout(resolve,20));result=await runtime.call('market.status',{requestId:result.requestId});}
  assert.equal(result.saved,true,JSON.stringify(result));assert.equal(result.status,'complete',JSON.stringify(result));
  const replay=await runtime.call('market.invite',{input:{packageId:selected.packageId,packageVersion:selected.packageVersion,packageHash:selected.packageHash,locale:'en'}});
  assert.equal(replay.requestId,result.requestId);assert.equal(replay.botId,result.botId);
  const profile=await runtime.call('profiles.get',{botId:result.botId});assert.ok(profile.name);
  assert.ok((await runtime.call('skills.list',{botId:result.botId})).length>0);
 }finally{await runtime.close();}
});
