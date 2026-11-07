// Actual Cindy task/Orca kernels on real SQLite; the native I/O seam is
// controlled here. Full sandbox -> Host -> Pi worker execution has its own
// integration test and is never inferred from these boundary checks.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';
import {fileURLToPath} from 'node:url';
import {createGhostTasks} from './platform-tasks.mjs';
import {setGhostDataRoot} from './seams/app-session.ts';

const repo=fileURLToPath(new URL('../../../',import.meta.url));
async function fixture(t){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(repo,'_temp/ghost-task-tests');await fs.mkdir(base,{recursive:true});
 const directory=await fs.mkdtemp(path.join(base,'teams-'));setGhostDataRoot(directory);
 const route={agentKind:'pi',providerId:'vendor',model:'model',effort:'high',fastMode:false},rows=new Map(),sent=[];
 const ghost={enabled:true,taskCapabilityApproved:true,approval:{state:'approved',revision:'original'},namespace:null,manifest:{id:'fixture',name:'Fixture',agent:{tasks:true}}};
 const tasks=createGhostTasks({root:directory,find:()=>ghost,current:()=>true,isRunning:()=>true,sendToGhost:()=>true,manager:{list:()=>[ghost]},push:()=>{},ui:async()=>null,agent:{setRunner(){}},errand:{setRunner(){}},cards:{},kv:{},log:{info(){},warn(){}},
  host:async(_method,input,signal)=>{
   if(input.op==='plugin-native'){
    if(input.method==='resolve-route')return input.route??route;
    if(input.method==='create'){const row={id:input.taskId,taskId:input.taskId,title:input.title,status:'active',revision:1,workingDir:directory,directory,permissionMode:input.permissionMode,resolvedConfig:input.route,orcaRole:'',workerPermissionMode:'',hasMessages:false};rows.set(row.id,row);return row;}
    if(input.method==='get')return rows.get(input.taskId);
    if(input.method==='inspect')return {instanceId:'native',running:false,pending:[],approvals:[],questions:[]};
    if(input.method==='assert-unstarted')return true;
   }
   if(input.op==='team-native'){
    if(input.method==='settings')return {subagents:true,maxSubagents:2};
    if(input.method==='models')return [{id:'vendor',name:'Vendor',models:['model'],modelMetadata:{model:{efforts:['high','deep']}}}];
    if(input.method==='get')return rows.get(input.taskId);
    if(input.method==='changed')return true;
    if(input.method==='role'){const row=rows.get(input.taskId);row.orcaRole=input.role;row.orcaTeamId=input.teamId;row.workerPermissionMode=input.permissionMode;row.revision++;return row;}
    if(input.method==='create'){
     const opts=input.options,row={id:opts.id,taskId:opts.id,title:opts.title,status:'active',revision:1,directory:opts.workingDir,workingDir:opts.workingDir,permissionMode:opts.permissionMode,orcaRole:'worker',orcaTeamId:opts.vendorOptions.orcaWorkflowId,orcaLeadId:opts.vendorOptions.orcaLeadSessionId,orcaWorkerId:opts.vendorOptions.orcaWorkerId,resolvedConfig:{agentKind:'pi',providerId:opts.providerId,model:opts.model,effort:opts.effort??'',fastMode:opts.fastMode}};rows.set(row.id,row);return row;
    }
    if(input.method==='facts')return {row:rows.get(input.taskId),live:false,activity:{phase:'idle'},state:{running:false,pending:[],approvals:[],questions:[]}};
    if(input.method==='directory')return {directory:input.params.directory};
    if(input.method==='send'){sent.push(input);return {accepted:true,queued:false,delivery:input.delivery,execution:{instanceId:'native',generation:1}};}
    if(input.method==='wait'){await delay(60_000,undefined,{signal});return {cursor:1};}
    if(input.method==='archive'){rows.get(input.taskId).status='archived';return true;}
   }
   throw Error('Unexpected native operation '+JSON.stringify({op:input.op,method:input.method}));
  }});
 tasks.writePreferences('fixture',{permissionMode:'auto'});t.after(()=>tasks.close());
 const created=await tasks.tasks('fixture',{type:'tasks-request',kind:'create',requestKey:'root',title:'Coordinator'});assert.equal(created.ok,true,JSON.stringify(created));
 return {tasks,directory,rows,sent,taskId:created.data.taskId};
}

test('starting a plugin Orca team creates no Worker or model input and preserves Auto defaults',async t=>{
 const f=await fixture(t);
 const first=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'startTeam',taskId:f.taskId});assert.equal(first.ok,true,JSON.stringify(first));assert.equal(first.data.workerPermissionMode,'auto');
 const again=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'startTeam',taskId:f.taskId});assert.equal(again.data.teamId,first.data.teamId);assert.equal(again.data.reused,true);
 const team=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'getTeam',taskId:f.taskId});assert.equal(team.data.workers.length,0);assert.equal(f.sent.length,0);assert.equal(f.rows.size,1);
 f.tasks.writePreferences('fixture',{permissionMode:'ask'});
 assert.equal((await f.tasks.tasks('fixture',{type:'tasks-request',kind:'startTeam',taskId:f.taskId})).error.code,'PERMISSION_DENIED');
});

test('original Orca creation keeps ordinary Worker ownership, selected effort and a real idle state',async t=>{
 const f=await fixture(t);await f.tasks.tasks('fixture',{type:'tasks-request',kind:'startTeam',taskId:f.taskId});
 const result=await f.tasks.teamOperation({sessionId:f.taskId,method:'createWorker',params:{agent:'pi',model:'model',providerId:'vendor',effort:'deep',role:'reviewer',label:'Review'}});
 assert.equal(result.ok,true,JSON.stringify(result));assert.equal(result.resolved.effort,'deep');assert.equal(result.resolved.label,'review');
 const worker=f.rows.get(result.workerSessionId);assert.equal(worker.permissionMode,'auto');assert.equal(worker.orcaRole,'worker');assert.equal(worker.pluginId,undefined);
 const team=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'getTeam',taskId:f.taskId});assert.equal(team.data.workers.length,1);assert.equal(team.data.workers[0].completedAt,null,'ready is not successful work');
 const owned=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'get',taskId:worker.id});assert.equal(owned.error.code,'TASK_NOT_FOUND');
 const released=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'releaseWorker',taskId:f.taskId,workerId:result.workerId,completedAt:100});assert.equal(released.ok,false);assert.equal(worker.status,'active');
 const duplicate=await f.tasks.teamOperation({sessionId:f.taskId,method:'createWorker',params:{agent:'pi',model:'model',providerId:'vendor',role:'reviewer',label:'review'}});assert.equal(duplicate.ok,false);assert.equal(duplicate.errorCode,'DUPLICATE_LABEL');
});

test('registered team plans use the original directory grant and reject unrelated workspaces',async t=>{
 const f=await fixture(t),inside=path.join(f.directory,'owned');await fs.mkdir(inside);
 const plan={concurrency:1,task:'Read the owned repository',items:[{label:'reader',workingDir:inside,route:{agentKind:'pi',providerId:'vendor',model:'model',effort:'high',fastMode:false},task:'Read only'}]};
 const saved=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'setTeamPlan',taskId:f.taskId,plan});assert.equal(saved.ok,true,JSON.stringify(saved));
 const other=path.join(path.dirname(f.directory),'outside-'+Date.now());await fs.mkdir(other);t.after(()=>fs.rm(other,{recursive:true}));
 const rejected=await f.tasks.tasks('fixture',{type:'tasks-request',kind:'setTeamPlan',taskId:f.taskId,plan:{...plan,items:[{...plan.items[0],workingDir:other}]}});assert.equal(rejected.ok,false);assert.equal(rejected.error.code,'PERMISSION_DENIED');
});
