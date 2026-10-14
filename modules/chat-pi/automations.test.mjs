import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {createAutomationRuntime} from './automations.mjs';

const definition={name:'Check project',prompt:'Inspect current files',kind:'cron',cronExpr:'0 9 * * *',timezone:'Asia/Shanghai',recurring:true,manual:false,agentKind:'pi',modelAgentKind:'pi',model:'fixture',providerId:'service',workspaceKind:'dialogue',useWorktree:false,permissionMode:'ask',notify:{desktop:false,feishu:false}};
test('all built-in templates and parameterized user templates remain editable without scheduling',async()=>{
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,dispatch:async()=>{throw Error('must not execute a template')}});
 try{
  const catalog=await runtime.call('templates');assert.equal(catalog.templates.length,6);assert.equal(catalog.categories.length,3);assert.equal((await runtime.call('templates',{language:'en-US'})).templates[0].name,'Nightly test self-healing');
  const user=await runtime.call('template.save',{template:{name:'Parameterized',prompt:'Review {{project}}',parameters:[{key:'project',label:'Project',type:'string',required:true}],category:'user'}});
  await assert.rejects(runtime.call('template.apply',{id:user.id}),/required/);
  assert.equal((await runtime.call('template.apply',{id:user.id,parameters:{project:'Fixture'}})).prompt,'Review Fixture');assert.equal((await runtime.call('list')).length,0);
  await runtime.call('template.delete',{id:user.id});assert.equal((await runtime.call('templates')).templates.length,6);
 }finally{await runtime.close();}
});
test('project configuration promotes, edits, reloads, clones and demotes without losing pause state',async()=>{
 const root=await directory(),project=await directory();const runtime=await createAutomationRuntime({directory:root,passive:true,dispatch:async()=>({sessionId:'fixture'})});
 try{
  const user=await runtime.call('create',{input:{...definition,workingDir:project,workspaceKind:'project',manual:true}});
  await runtime.call('project.promote',{id:user.id});let [task]=await runtime.call('list');assert.equal(task.source,'project');assert.notEqual(task.id,user.id);
  await runtime.call('pause',{id:task.id});await runtime.call('update',{id:task.id,patch:{name:'Project edit',preRunHook:{command:'node check.mjs'}}});
  let file=await runtime.call('project.read',{workingDir:project});assert.equal(file.config.schedules[0].name,'Project edit');assert.equal(file.config.schedules[0].preRunHook.command,'node check.mjs');
  await runtime.call('project.reconcile',{workingDir:project});assert.equal((await runtime.call('get',{id:task.id})).status,'paused');
  const clone=await runtime.call('project.clone',{id:task.id});assert.equal(clone.source,'user');assert.notEqual(clone.id,task.id);
  await runtime.call('project.demote',{id:task.id});assert.ok((await runtime.call('list')).every(value=>value.source==='user'));file=await runtime.call('project.read',{workingDir:project});assert.equal(file.config.schedules.length,0);
 }finally{await runtime.close();}
});
test('run policy binds to the accepted turn and quiet runs can explicitly request notification',async()=>{
 const notifications=[];let instance,requestNotify=false;
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,notify:async(schedule,run)=>notifications.push({schedule,run}),dispatch:async(schedule,ctx)=>{
  ctx.onTurnActive('session');await assert.rejects(instance.call('policy',{sessionId:'session',requestId:'wrong',action:'notify'}),/not executing/);
  if(requestNotify)assert.equal((await instance.call('policy',{sessionId:'session',requestId:ctx.runId,action:'notify'})).applied,true);
  return {sessionId:'session',resultText:'Result'};
 }});instance=runtime;
 try{const task=await runtime.call('create',{input:{...definition,silentWhenIdle:true}});await runtime.call('run',{id:task.id});await runtime.settled();assert.equal(notifications.length,0);requestNotify=true;await runtime.call('run',{id:task.id});await runtime.settled();assert.equal(notifications.length,1);assert.equal(notifications[0].run.status,'success');}
 finally{await runtime.close();}
});
test('notification failures are visible on the successful run without losing its result',async()=>{
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,notify:async()=>{throw Error('connector unavailable')},dispatch:async()=>({sessionId:'session',resultText:'Saved result'})});
 try{const task=await runtime.call('create',{input:definition});await runtime.call('run',{id:task.id});await runtime.settled();const [run]=await runtime.call('runs',{id:task.id});assert.equal(run.status,'success');assert.equal(run.resultText,'Saved result');assert.match(run.notificationError,/connector unavailable/);}
 finally{await runtime.close();}
});
async function directory(){return fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'automation-runtime-'));}

test('run-now returns its real run id before execution finishes, then history records the result',async()=>{
 let finish,dispatched;const started=new Promise(resolve=>dispatched=resolve),completion=new Promise(resolve=>finish=resolve);
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,dispatch:async(schedule,context)=>{await context.onSessionBound('chat-fixture');dispatched();await completion;return {sessionId:'chat-fixture',resultText:'Saved report'};}});
 try{
  const task=await runtime.call('create',{input:definition});const accepted=await runtime.call('run',{id:task.id});await started;
  assert.ok(accepted.runId);assert.equal((await runtime.call('runs',{id:task.id}))[0].status,'running');
  finish();await runtime.settled();const result=(await runtime.call('runs',{id:task.id}))[0];assert.equal(result.status,'success');assert.equal(result.resultText,'Saved report');
  assert.equal((await runtime.call('get',{id:task.id})).unreadRunCount,1);await runtime.call('markRead',{runId:result.id});assert.equal((await runtime.call('get',{id:task.id})).unreadRunCount,0);
 }finally{finish();await runtime.close();}
});

test('one-time dates remain exact and nullable fields clear through the JSON boundary',async()=>{
 const now=Date.parse('2026-10-10T00:00:00Z'),onceAt=Date.parse('2027-11-12T02:30:00Z');
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,clock:{now:()=>now},dispatch:async()=>({sessionId:'one'})});
 try{const task=await runtime.call('create',{input:{...definition,onceAt,recurring:false}});assert.equal(task.nextFireAt,onceAt);const updated=await runtime.call('update',{id:task.id,patch:{onceAt:null,intervalMs:null,targetSessionId:null,recurring:true}});assert.equal(updated.onceAt,undefined);assert.equal(updated.intervalMs,undefined);assert.ok(updated.nextFireAt<onceAt);}
 finally{await runtime.close();}
});

test('pausing a running automation cancels its actual execution and preserves history',async()=>{
 let aborted=false;const runtime=await createAutomationRuntime({directory:await directory(),passive:true,dispatch:async(schedule,ctx)=>new Promise((resolve,reject)=>{ctx.signal.addEventListener('abort',()=>{aborted=true;reject(new DOMException('aborted','AbortError'));},{once:true});})});
 try{const task=await runtime.call('create',{input:definition});await runtime.call('run',{id:task.id});await runtime.call('pause',{id:task.id});await runtime.settled();assert.equal(aborted,true);assert.equal((await runtime.call('runs',{id:task.id}))[0].status,'aborted');assert.equal((await runtime.call('get',{id:task.id})).status,'paused');}
 finally{await runtime.close();}
});

test('resuming a one-time task preserves its calendar instant and expires a missed date',async()=>{
 let now=Date.parse('2026-10-10T00:00:00Z');const onceAt=now+60000;
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,clock:{now:()=>now},dispatch:async()=>{throw Error('A paused or expired one-time task must not run');}});
 try{const task=await runtime.call('create',{input:{...definition,onceAt,recurring:false}});await runtime.call('pause',{id:task.id});now+=30000;
  const resumed=await runtime.call('resume',{id:task.id});assert.equal(resumed.nextFireAt,onceAt);assert.equal(resumed.onceAt,onceAt);
  await runtime.call('pause',{id:task.id});now=onceAt+1;
  const expired=await runtime.call('resume',{id:task.id});assert.equal(expired.status,'expired');assert.equal(expired.nextFireAt,undefined);assert.equal((await runtime.call('runs',{id:task.id})).length,0);
 }finally{await runtime.close();}
});

test('deleting a task archives its runs and restoring keeps the history without automatically rerunning',async()=>{
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,dispatch:async()=>({sessionId:'retained-chat',resultText:'Retained result'})});
 try{const task=await runtime.call('create',{input:definition});await runtime.call('run',{id:task.id});await runtime.settled();await runtime.call('delete',{id:task.id});
  assert.equal((await runtime.call('runs',{id:task.id})).length,0);assert.equal((await runtime.call('deleted')).length,1);
  const restored=await runtime.call('restore',{id:task.id});assert.equal(restored.status,'paused');assert.equal((await runtime.call('runs',{id:task.id}))[0].resultText,'Retained result');assert.equal((await runtime.call('deleted')).length,0);
 }finally{await runtime.close();}
});

test('an unavailable explicitly bound conversation pauses the task after recording its failure',async()=>{
 const runtime=await createAutomationRuntime({directory:await directory(),passive:true,dispatch:async()=>{const error=Error('Bound conversation was archived');error.code='target_unavailable';throw error;}});
 try{const task=await runtime.call('create',{input:{...definition,targetSessionId:'old-chat',persistentSession:false}});await runtime.call('run',{id:task.id});await runtime.settled();assert.equal((await runtime.call('get',{id:task.id})).status,'paused');assert.equal((await runtime.call('runs',{id:task.id}))[0].status,'failed');}
 finally{await runtime.close();}
});

test('a second real scheduler process cannot replan a live claim and takes over after its owner exits',{timeout:20000},async()=>{
 const root=await directory();let taskId='',started=false,output='';
 const source=`import {createAutomationRuntime} from ${JSON.stringify(new URL('./automations.mjs',import.meta.url).href)};
 const runtime=await createAutomationRuntime({directory:${JSON.stringify(root)},dispatch:async(schedule,context)=>{console.log(JSON.stringify({started:true,id:schedule.id}));return new Promise((resolve,reject)=>context.signal.addEventListener('abort',()=>reject(new DOMException('aborted','AbortError')),{once:true}));}});
 const task=await runtime.call('create',{input:${JSON.stringify({...definition,intervalMs:1000})}});console.log(JSON.stringify({task:task.id}));`;
 const child=spawn(process.execPath,['--eval',source],{stdio:['ignore','pipe','pipe'],windowsHide:true});
 const errors=[];child.stderr.on('data',chunk=>errors.push(chunk.toString()));
 child.stdout.on('data',chunk=>{output+=chunk.toString();for(;;){const end=output.indexOf('\n');if(end<0)break;const value=JSON.parse(output.slice(0,end));output=output.slice(end+1);if(value.task)taskId=value.task;if(value.started)started=true;}});
 const until=async(predicate,timeout=8000)=>{const end=Date.now()+timeout;while(!predicate()){if(Date.now()>end)throw Error('Timed out waiting for scheduler evidence: '+errors.join(''));await new Promise(resolve=>setTimeout(resolve,25));}};
 let second,dispatches=0;
 try{await until(()=>started);second=await createAutomationRuntime({directory:root,dispatch:async()=>{dispatches++;return {sessionId:'replacement',resultText:'Recovered'};}});
  await new Promise(resolve=>setTimeout(resolve,1800));assert.equal(dispatches,0,'startup must not overwrite another process execution claim');
  const exited=once(child,'exit');child.kill();await exited;
  await until(()=>dispatches>0);await second.settled();
  const runs=await second.call('runs',{id:taskId});assert.ok(runs.some(run=>run.status==='interrupted'));assert.ok(runs.some(run=>run.status==='success'));
 }finally{if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}await second?.close();}
});
