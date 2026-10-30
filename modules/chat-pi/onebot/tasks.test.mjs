import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {Database} from 'bun:sqlite';
import {createOneBotRuntime} from './runtime.mjs';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');

test('independent task uses its selected model and retains the frozen choice after profile changes',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'tasks-')),parent='ABCDEFGHIJKLMNOPQRSTUVWXYZ',dispatches=[];
 const host={
  assertTaskAuthority:async request=>request.sessionId===parent,
  readSession:async id=>({id,model:'fallback',providerId:'fixture',agentKind:'pi',effort:'deep',fastMode:true,permissionMode:'ask',workingDir:directory,status:'active',source:'bot'}),
  resolveTaskModelSelection:async selection=>{assert.equal(selection.id,'fixture/explicit');return {harness:'pi',providerId:'fixture',model:'explicit',effort:selection.effort,fastMode:false};},
  validateTaskModel:async route=>route.providerId==='fixture'&&['explicit','profile','fallback'].includes(route.model),
  dispatch:async input=>{dispatches.push(input);await input.onAccepted?.();return {ok:true,targetSessionId:input.targetSessionId,wakeKind:'created'};},
  abortSession:async()=>{},
  persistTimelineMessage:async()=>{},
  captureRequest:async()=>({message:'Owner request',autoReviewUserText:{kind:'agent-request',request:{userText:'Do the work',sessionId:parent,clientId:'owner-input'}}}),
 };
 let runtime=createOneBotRuntime({directory,tasks:host});
 try{
  const profile=await runtime.call('profiles.create',{input:{id:'task-owner',name:'Task owner',capabilities:{taskModelOverride:{harness:'pi',providerId:'fixture',model:'profile',effort:'balanced',fastMode:true}}}});
  await runtime.call('sessions.bind',{botId:profile.id,sessionId:parent});
  const started=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Write the requested report',modelSelection:{id:'fixture/explicit',effort:'light'},ownerAuthorized:true});
  assert.equal(started.ok,true,JSON.stringify(started));
  assert.equal(started.status,'running');
  assert.deepEqual(started.modelRoute,{harness:'pi',providerId:'fixture',model:'explicit',effort:'light',fastMode:false});
  assert.equal(dispatches.length,1);
  await runtime.call('profiles.update',{input:{id:profile.id,expectedVersion:profile.currentVersion,capabilities:{taskModelOverride:null}}});
  const task=await runtime.call('tasks.get',{callerSessionId:parent,delegationId:started.delegationId});
  assert.equal(task.ok,true,JSON.stringify(task));
  assert.equal(task.task.session_id,started.childSessionId);
  await runtime.close();
  runtime=createOneBotRuntime({directory,tasks:host});
  const reopened=await runtime.call('tasks.list',{callerSessionId:parent});
  assert.equal(reopened.delegations.length,1);
  assert.deepEqual(reopened.delegations[0].permissionSnapshot.taskModelOverride,{harness:'pi',providerId:'fixture',model:'explicit',effort:'light',fastMode:false});
  const stored=new Database(path.join(directory,'tasks.db'),{readonly:true});
  try{
   assert.deepEqual(stored.query('SELECT model,effort,fast_mode,source FROM sessions WHERE id=?').get(started.childSessionId),{model:'explicit',effort:'light',fast_mode:0,source:'desktop'});
  }finally{stored.close(true);}
 }finally{await runtime.close();}
});

async function taskFixture(){
 const directory=await fs.mkdtemp(path.join(base,'task-routes-')),parent='ABCDEFGHIJKLMNOPQRSTUVWXYZ',sent=[];
 const host={
  assertTaskAuthority:async request=>request.sessionId===parent,
  readSession:async id=>({id,title:'Parent',model:'fallback',providerId:'fixture',agentKind:'pi',effort:'deep',fastMode:true,permissionMode:'ask',workingDir:directory,status:'active',source:'bot'}),
  validateTaskModel:async route=>route.harness==='pi'&&route.providerId==='fixture'&&['profile','fallback'].includes(route.model),
  resolveTaskModelSelection:async()=>{throw Error('Selected model is unavailable');},
  dispatch:async input=>{sent.push(input);await input.onAccepted?.();return {ok:true,targetSessionId:input.targetSessionId,wakeKind:'created'};},
  abortSession:async()=>{},persistTimelineMessage:async()=>{},
 };
 const runtime=createOneBotRuntime({directory,tasks:host});
 const profile=await runtime.call('profiles.create',{input:{id:'routes',name:'Routes',capabilities:{taskModelOverride:{harness:'pi',providerId:'fixture',model:'profile',effort:'balanced',fastMode:false}}}});
 await runtime.call('sessions.bind',{botId:profile.id,sessionId:parent});
 return {runtime,host,parent,profile,sent};
}

test('task profile route wins over the live fallback; following inherits its exact parameters',async()=>{
 const {runtime,parent,profile}=await taskFixture();
 try{
  const configured=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Use task settings',ownerAuthorized:true});
  assert.equal(configured.ok,true,JSON.stringify(configured));
  assert.deepEqual(configured.modelRoute,{harness:'pi',providerId:'fixture',model:'profile',effort:'balanced',fastMode:false});
  await runtime.call('profiles.update',{input:{id:profile.id,expectedVersion:profile.currentVersion,capabilities:{taskModelOverride:null}}});
  const followed=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Use current session',ownerAuthorized:true});
  assert.equal(followed.ok,true,JSON.stringify(followed));
  assert.deepEqual(followed.modelRoute,{harness:'pi',providerId:'fixture',model:'fallback',effort:'deep',fastMode:true});
 }finally{await runtime.close();}
});

test('unavailable task selection and non-owner callers never create or dispatch work',async()=>{
 const {runtime,parent,sent}=await taskFixture();
 try{
  const invalid=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Unavailable choice',modelSelection:{id:'missing'},ownerAuthorized:true});
  assert.equal(invalid.errorCode,'TASK_MODEL_UNAVAILABLE');
  await assert.rejects(runtime.call('tasks.start',{callerSessionId:parent,objective:'Unapproved work',ownerAuthorized:false}),error=>error.code==='OWNER_ONLY');
  const rows=await runtime.call('tasks.list',{callerSessionId:parent});
  assert.deepEqual(rows.delegations,[]);assert.equal(sent.length,0);
 }finally{await runtime.close();}
});

test('a task cannot start without its visible timeline anchor',async()=>{
 const {runtime,host,parent,sent}=await taskFixture();
 host.persistTimelineMessage=async()=>{throw Error('Timeline unavailable');};
 try{
  const result=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Keep a visible task',ownerAuthorized:true});
  assert.equal(result.errorCode,'PARENT_TIMELINE_PERSIST_FAILED');assert.equal(sent.length,0);
  const rows=await runtime.call('tasks.list',{callerSessionId:parent});
  assert.equal(rows.delegations[0].status,'failed');
 }finally{await runtime.close();}
});

test('permission changes during model admission revoke task creation before dispatch',async()=>{
 const {runtime,host,parent,profile,sent}=await taskFixture();
 let enter,release;const entered=new Promise(resolve=>{enter=resolve;}),gate=new Promise(resolve=>{release=resolve;});
 host.resolveTaskModelSelection=async()=>{enter();await gate;return {harness:'pi',providerId:'fixture',model:'profile',effort:'balanced',fastMode:false};};
 try{
  const pending=runtime.call('tasks.start',{callerSessionId:parent,objective:'Pending admission',modelSelection:{id:'profile'},ownerAuthorized:true});
  await entered;
  await runtime.call('profiles.update',{input:{id:profile.id,expectedVersion:profile.currentVersion,capabilities:{permissions:'ask'}}});
  release();
  const result=await pending;
  assert.equal(result.ok,false,JSON.stringify(result));
  assert.equal(result.errorCode,'TASK_ACTION_AUTHORIZATION_EXPIRED');
  assert.equal(sent.length,0);
  assert.deepEqual((await runtime.call('tasks.list',{callerSessionId:parent})).delegations,[]);
 }finally{release();await runtime.close();}
});

test('only the accepted native execution can finish a task, and duplicate completion is not redelivered',async()=>{
 const {runtime,host,parent,sent}=await taskFixture();
 const execution={instanceId:'native-instance',generation:1};
 host.readSessionExecution=()=>execution;
 try{
  const started=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Complete once',ownerAuthorized:true});
  assert.equal(started.ok,true,JSON.stringify(started));
  await runtime.call('tasks.settle',{childSessionId:started.childSessionId,outcome:'done',execution:{...execution,generation:2},resultText:'Wrong generation'});
  assert.equal((await runtime.call('tasks.get',{callerSessionId:parent,delegationId:started.delegationId})).task.status,'running');
  const completed={childSessionId:started.childSessionId,outcome:'done',execution,resultText:'Verified task result'};
  await runtime.call('tasks.settle',completed);
  const task=await runtime.call('tasks.get',{callerSessionId:parent,delegationId:started.delegationId});
  assert.equal(task.task.status,'completed');assert.equal(task.task.result,'Verified task result');
  const delivered=sent.filter(input=>input.targetSessionId===parent).length;
  assert.equal(delivered,1);
  await runtime.call('tasks.settle',completed);
  assert.equal(sent.filter(input=>input.targetSessionId===parent).length,1);
  assert.equal(task.task.session_status,'active');
 }finally{await runtime.close();}
});

test('on its own initiative a teammate may advance or stop only its own task, never approve or start one',async()=>{
 const {runtime,parent,sent}=await taskFixture();
 try{
  const started=await runtime.call('tasks.start',{callerSessionId:parent,objective:'Owner started work',ownerAuthorized:true});
  assert.equal(started.ok,true,JSON.stringify(started));
  const autonomous={callerSessionId:parent,ownerAuthorized:false,principal:'autonomous'};
  // Cindy NEEDS_OWNER_REQUEST: a new task, a permission decision or resuming a pause.
  for(const request of [['tasks.start',{objective:'Self-started work'}],['tasks.message',{delegationId:started.delegationId,input:{kind:'approve'}}],
   ['tasks.message',{delegationId:started.delegationId,input:{kind:'deny',reason:'no'}}],['tasks.message',{delegationId:started.delegationId,input:{kind:'resume'}}],
   ['tasks.stop',{delegationId:'someone-elses-task'}]]){
   const result=await runtime.call(request[0],{...autonomous,...request[1]});
   assert.equal(result.errorCode,'NEEDS_OWNER_REQUEST',JSON.stringify(result));
  }
  const queued=await runtime.call('tasks.message',{...autonomous,delegationId:started.delegationId,input:{kind:'message',text:'Continue toward the original goal'}});
  assert.equal(queued.ok,true,JSON.stringify(queued));
  const stopped=await runtime.call('tasks.stop',{...autonomous,delegationId:started.delegationId});
  assert.equal(stopped.ok,true,JSON.stringify(stopped));
  // Without a Host principal a non-owner turn is still refused outright.
  await assert.rejects(runtime.call('tasks.message',{callerSessionId:parent,ownerAuthorized:false,delegationId:started.delegationId,input:{kind:'message',text:'x'}}),error=>error.code==='OWNER_ONLY');
  assert.equal(sent.filter(input=>input.message?.includes?.('Self-started')).length,0);
 }finally{await runtime.close();}
});
