// Actual upstream slots, with only the private WinOne Host boundary supplied.
// The native Host/Bun tests cover that boundary separately; these tests assert
// that async receipts and capability revocation are not converted into success.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {setGhostDataRoot} from './seams/app-session.ts';
import {createGhostTasks} from './platform-tasks.mjs';
import {createGhostPreferences} from './platform-preferences.mjs';
import {GhostAgentSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/agentSlot.ts';
import {GhostErrandSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/errandSlot.ts';
import {recordGhostPickedDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pickGrantsStore.ts';

const root=fileURLToPath(new URL('../../../',import.meta.url));
async function fixture(t,host=async()=>null,{legacy=false,ui=async()=>null,now}={}){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(root,'_temp/ghost-task-tests');
 await fs.mkdir(base,{recursive:true});
 const directory=await fs.mkdtemp(path.join(base,'tasks-'));
 setGhostDataRoot(directory);
 let ghost={enabled:true,taskCapabilityApproved:true,targetCapabilityApproved:true,companionsCapabilityApproved:true,observeCapabilityApproved:true,
  approval:{state:'approved',revision:'original'},namespace:null,
  manifest:{id:'fixture',name:'Fixture',agent:{tasks:true,targets:true,companions:true,observe:true,...(legacy?{errand:true,background:true}:{})},...(legacy?{workspace:true}:{}),routineEvents:{events:[{type:'changed',name:'Changed'}]}}};
 const sent=[];
 const log={info(){},warn(){}};
 const agent=legacy?new GhostAgentSlot({getGhost:()=>ghost,now,log}):{setRunner(){}};
 const errand=legacy?new GhostErrandSlot({getGhost:()=>ghost,now,log}):{setRunner(){}};
 const preferences=legacy?createGhostPreferences({root:directory,find:()=>ghost,host}):undefined;
 const tasks=createGhostTasks({root:directory,find:()=>ghost,manager:{list:()=>[ghost]},
  current:captured=>ghost.enabled&&captured.approval.revision===ghost.approval.revision,
  isRunning:()=>true,sendToGhost:(id,payload)=>{sent.push({id,payload});return true;},
  host,push:()=>{},ui,agent,errand,cards:{},kv:{},log,preferences});
 t.after(()=>tasks.close());
 return {tasks,sent,directory,agent,errand,preferences,ghost:()=>ghost,replace:value=>{ghost=value;}};
}

test('routine status waits for the remote engine and does not invent an acknowledgement',async t=>{
 let release;
 const f=await fixture(t,()=>new Promise(resolve=>{release=resolve;}));
 let returned=false;
 const call=f.tasks.routine(f.ghost(),{action:'status',status:'listening'}).then(value=>{returned=true;return value;});
 await new Promise(resolve=>setTimeout(resolve,20));
 assert.equal(returned,false);
 release({registered:true});
 assert.deepEqual(await call,{ok:true});
});

test('routine registration failure is an actual failed slot result',async t=>{
 const f=await fixture(t,async()=>{throw Error('Routine service is stopped');});
 const result=await f.tasks.routine(f.ghost(),{action:'status',status:'listening'});
 assert.equal(result.ok,false);
 assert.equal(result.message,'Routine service is stopped');
});

test('companion watch subscribes, receives changes and stops its subscription',async t=>{
 const f=await fixture(t);
 const started=await f.tasks.companions('fixture',{kind:'watch'});
 assert.equal(started.ok,true,JSON.stringify(started));
 assert.ok(started.data.subscriptionId);
 f.tasks.hostEvent({type:'activity'});
 await new Promise(resolve=>setTimeout(resolve,20));
 assert.equal(f.sent.length,1);
 assert.equal(f.sent[0].payload.type,'companions-changed');
 assert.equal(f.sent[0].payload.subscriptionId,started.data.subscriptionId);
 assert.deepEqual(await f.tasks.companions('fixture',{kind:'unwatch',subscriptionId:started.data.subscriptionId}),{ok:true,data:{stopped:true}});
});

test('task creation retains original receipt identity and returns one public envelope',async t=>{
 const rows=new Map();let created=0;
 const route={agentKind:'pi',providerId:'fixture',model:'model',effort:'',fastMode:false};
 const f=await fixture(t,async(_method,input)=>{
  assert.equal(input.op,'plugin-native');
  if(input.method==='resolve-route')return input.route??route;
  if(input.method==='create'){
   created++;
   rows.set(input.taskId,{taskId:input.taskId,title:input.title,status:'active',revision:1,workingDir:'fixture',permissionMode:input.permissionMode,hidden:input.hidden,resolvedConfig:input.route});
   return rows.get(input.taskId);
  }
  if(input.method==='get')return rows.get(input.taskId);
  if(input.method==='set-model'){const row=rows.get(input.taskId);row.resolvedConfig=input.route;row.revision++;return {status:'updated',task:row};}
  throw Error('Unexpected Host method: '+input.method);
 });
 const request={type:'tasks-request',kind:'create',requestKey:'one',title:'My task'};
 const first=await f.tasks.tasks('fixture',request);
 assert.equal(first.ok,true,JSON.stringify(first));
 assert.match(first.data.taskId,/^[A-Z2-7]{26}$/);
 assert.equal(first.data.data,undefined,'The source slot already owns the success envelope');
 assert.equal((await f.tasks.tasks('fixture',request)).data.taskId,first.data.taskId);
 assert.equal(created,1);
 const conflict=await f.tasks.tasks('fixture',{...request,title:'Different task'});
 assert.equal(conflict.ok,false);
 assert.equal(conflict.error.code,'IDEMPOTENCY_CONFLICT');
 const changed=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'setModel',taskId:first.data.taskId,expectedRevision:1,route:{...route,model:'another'}});
 assert.equal(changed.ok,true,JSON.stringify(changed));
 assert.equal(changed.data.task.resolvedConfig.model,'another');
 const stale=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'setModel',taskId:first.data.taskId,expectedRevision:1,route});
 assert.equal(stale.error.code,'STALE_REVISION');
});

test('plugin defaults preserve provider effort words and a hidden Host card ignores saved authority',async t=>{
 const rows=new Map(),created=[];
 const defaultRoute={agentKind:'pi',providerId:'default',model:'default-model',effort:'',fastMode:false};
 const f=await fixture(t,async(_method,input)=>{
  if(input.method==='resolve-route')return input.route??defaultRoute;
  if(input.method==='create'){created.push(input);rows.set(input.taskId,{taskId:input.taskId,title:input.title,status:'active',revision:1,resolvedConfig:input.route,permissionMode:input.permissionMode});return rows.get(input.taskId);}
  if(input.method==='get')return rows.get(input.taskId);
  throw Error('Unexpected native call '+input.method);
 });
 const config={agentKind:'pi',providerId:'vendor',model:'model',effort:'adaptive-vendor',fastMode:true,permissionMode:'auto'};
 await f.tasks.validatePreferences('fixture',config);
 f.tasks.writePreferences('fixture',config);
 assert.deepEqual(f.tasks.preferenceDetails('fixture').taskPreferences,config);
 const first=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'create',requestKey:'configured',title:'Configured'});
 assert.equal(first.ok,true,JSON.stringify(first));
 assert.equal(created[0].route.effort,'adaptive-vendor');
 assert.equal(created[0].permissionMode,'auto');
 f.replace({...f.ghost(),manifest:{...f.ghost().manifest,showTaskPreferences:false}});
 assert.equal(f.tasks.preferenceDetails('fixture').showTaskPreferences,false);
 assert.deepEqual(f.tasks.preferenceDetails('fixture').taskPreferences,{});
 const second=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'create',requestKey:'no-card',title:'No card',permissionMode:'bypassPermissions'});
 assert.equal(second.ok,true,JSON.stringify(second));
 assert.deepEqual(created[1].route,defaultRoute);
 assert.equal(created[1].permissionMode,'bypassPermissions');
});

test('targets keep real delivery receipts, reject key conflicts and recheck approval at execution',async t=>{
 const deliveries=[];let requestId;
 const targetId='A'.repeat(26);
 const f=await fixture(t,async(_method,input)=>{
  assert.equal(input.op,'plugin-target');
  if(input.method==='list')return [{id:targetId,label:'Task',detail:'Workspace'}];
  if(input.method==='session')return targetId;
  if(input.method==='send'){deliveries.push(input);requestId=input.clientId;return {accepted:true,delivery:'queue'};}
  if(input.method==='wait')return {cursor:1,terminal:{execution:{instanceId:'native-instance',generation:1},status:'completed'}};
  if(input.method==='withdraw')return true;
  if(input.method==='settled')return {settled:true};
  throw Error('Unexpected target operation '+input.method);
 });
 const request={type:'targets-request',kind:'send',requestKey:'delivery',target:{kind:'task',id:targetId},text:'Do the task'};
 const first=await f.tasks.tasks('fixture',request);
 assert.deepEqual(first,{ok:true,data:{taskId:targetId,accepted:true,delivery:'queue'}});
 assert.deepEqual(await f.tasks.tasks('fixture',request),first);
 assert.equal(deliveries.length,1);
 const conflict=await f.tasks.tasks('fixture',{...request,text:'A different input'});
 assert.equal(conflict.error.code,'REQUEST_KEY_CONFLICT');
 await f.tasks.admitTarget({sessionId:targetId,requestId});
 f.replace({...f.ghost(),targetCapabilityApproved:false});
 await assert.rejects(f.tasks.admitTarget({sessionId:targetId,requestId}),/PERMISSION_DENIED/);
 f.replace({...f.ghost(),targetCapabilityApproved:true});
 const cancelled=await f.tasks.tasks('fixture',{type:'targets-request',kind:'cancel',requestKey:'cancel-first'});
 assert.equal(cancelled.data.cancelled,true);
 const superseded=await f.tasks.tasks('fixture',{...request,requestKey:'cancel-first'});
 assert.equal(superseded.error.code,'SUPERSEDED');
 assert.equal(deliveries.length,1);
});

test('companion responses cannot cross a changed installation approval',async t=>{
 let release;
 const f=await fixture(t,()=>new Promise(resolve=>{release=resolve;}));
 const pending=f.tasks.companions('fixture',{kind:'list'});
 f.replace({...f.ghost(),approval:{state:'approved',revision:'replacement'}});
 release([]);
 const result=await pending;
 assert.equal(result.ok,false);
 assert.equal(result.error.code,'PERMISSION_DENIED');
});

test('legacy Agent reveals a clicked new task once and never reveals a background new task',async t=>{
 const revealed=[],started=[];
 const f=await fixture(t,async(_method,input)=>{
  if(input.op==='reveal-session'){revealed.push(input.taskId);return true;}
  if(input.op==='agent-turn'){started.push(input.request);return {ok:true,sessionId:'new-'+started.length,disposition:'created'};}
  throw Error('Unexpected Agent Host operation');
 },{legacy:true});
 const base={type:'agent-request',mode:'new',promptTemplate:'Owner request: {{user_message}}',userMessage:'Continue'};
 const token=f.agent.issueUserActionToken('fixture','original');
 const clicked=await f.agent.handleRequest('fixture',{...base,userActionToken:token});
 assert.equal(clicked.ok,true,JSON.stringify(clicked));assert.deepEqual(revealed,['new-1']);
 const replay=await f.agent.handleRequest('fixture',{...base,userActionToken:token});assert.equal(replay.errorCode,'TOKEN_EXPIRED');
 const background=await f.agent.handleRequest('fixture',{...base,trigger:'background',sessionId:'original'});
 assert.equal(background.ok,true,JSON.stringify(background));assert.deepEqual(revealed,['new-1']);
 assert.equal(started[0].sourceSessionId,'original');assert.equal(JSON.parse(started[0].persistedContent).ghostAgent.userMessage,'Continue');
});

test('legacy errand uses the user directory first and checks only plugin-relayed directories against the picker ledger',async t=>{
 let clock=100000;const starts=[];
 const f=await fixture(t,async(_method,input)=>{
  if(input.op==='errand-start'){starts.push(input.request);return {ok:true,sessionId:'errand-'+starts.length,requestId:'input-'+starts.length};}
  if(input.op==='errand-wait')return {ok:true,sessionId:input.request.sessionId,text:'Exact result'};
  throw Error('Unexpected errand Host operation');
 },{legacy:true,now:()=>clock});
 f.preferences.write('fixture',{workingDir:f.directory,model:'selected',providerId:'fixture',effort:'adaptive',permissionMode:'auto'});
 const run=workingDir=>f.errand.handleRequest('fixture',{type:'agent-errand-request',kind:'run',task:'Inspect',mode:'wait',...(workingDir?{workingDir}:{})});
 assert.equal((await run(path.join(f.directory,'not-picked'))).ok,true);
 assert.equal(starts[0].workingDir,f.directory);assert.equal(starts[0].permissionMode,'auto');assert.equal(starts[0].taskConfig.effort,'adaptive');
 clock+=11000;f.preferences.write('fixture',{permissionMode:'auto'});
 assert.equal((await run(f.directory)).errorCode,'INVALID_REQUEST');assert.equal(starts.length,1);
 clock+=11000;recordGhostPickedDir('fixture',f.directory);
 assert.equal((await run(f.directory)).ok,true);assert.equal(starts[1].workingDir,f.directory);
 clock+=11000;f.preferences.write('fixture',{workingDir:f.directory,model:'selected',providerId:'fixture',permissionMode:'bypassPermissions'});
 f.replace({...f.ghost(),manifest:{...f.ghost().manifest,showTaskPreferences:false}});
 assert.equal((await run()).ok,true);assert.equal(starts[2].permissionMode,'plan');assert.equal(starts[2].workingDir,undefined);assert.deepEqual(starts[2].taskConfig,{});
});

test('legacy workspace creates an authorized draft with the captured model, not an unrelated saved directory',async t=>{
 const calls=[];let selected;
 const route={agentKind:'pi',providerId:'fixture',model:'model',effort:'adaptive',fastMode:false};
 const f=await fixture(t,async(_method,input)=>{
  calls.push(input);
  if(input.method==='resolve-route')return input.route??route;
  if(input.op==='find-by-workdir')return {};
  if(input.op==='create-draft')return {sessionId:'workspace-draft'};
  throw Error('Unexpected workspace Host operation');
 },{legacy:true,ui:async()=>({path:selected})});
 selected=f.directory;
 f.preferences.write('fixture',{workingDir:path.join(f.directory,'unrelated'),providerId:'fixture',model:'model',effort:'adaptive',permissionMode:'auto'});
 const result=await f.tasks.workspace('fixture',{kind:'ensure-session',mode:'pick'},()=>true);
 assert.equal(result.ok,true,JSON.stringify(result));assert.equal(result.created,true);
 const created=calls.find(call=>call.op==='create-draft');assert.equal(created.dirAbs,f.directory);assert.equal(created.route.effort,'adaptive');assert.equal(created.permissionMode,'auto');
});

test('legacy workspace rejects changed settings before its draft reaches the native Host',async t=>{
 let f,selected;const calls=[];
 f=await fixture(t,async(_method,input)=>{
  calls.push(input);
  if(input.op==='find-by-workdir')return {};
  if(input.method==='resolve-route'){f.preferences.write('fixture',{model:'changed',providerId:'fixture'});return input.route;}
  if(input.op==='create-draft')throw Error('A stale configuration must not create a draft');
  throw Error('Unexpected workspace Host operation');
 },{legacy:true,ui:async()=>({path:selected})});
 selected=f.directory;f.preferences.write('fixture',{model:'first',providerId:'fixture'});
 const result=await f.tasks.workspace('fixture',{kind:'ensure-session',mode:'pick'},()=>true);
 assert.equal(result.ok,false);assert.equal(calls.some(call=>call.op==='create-draft'),false);
});
