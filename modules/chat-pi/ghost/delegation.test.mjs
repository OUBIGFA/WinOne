// Original delegated-scope resolver and persisted target receipts. Only the
// native I/O boundary is controlled here; real Host/Pi execution is covered by
// chat_ghost_task_integration_windows_test.go, not inferred from these checks.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';
import {fileURLToPath} from 'node:url';
import {createGhostTasks} from './platform-tasks.mjs';
import {setGhostDataRoot} from './seams/app-session.ts';
import {createPermissionGate} from '../permissions.mjs';

const repo=fileURLToPath(new URL('../../../',import.meta.url));
async function fixture(t){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(repo,'_temp/ghost-task-tests');
 await fs.mkdir(base,{recursive:true});const directory=await fs.mkdtemp(path.join(base,'delegation-'));setGhostDataRoot(directory);
 let ghost={enabled:true,taskCapabilityApproved:true,targetCapabilityApproved:true,delegationCapabilityApproved:true,approval:{state:'approved',revision:'approved'},namespace:null,
  manifest:{id:'fixture',name:'Fixture',agent:{tasks:true,targets:true,delegation:true}}};
 const route={agentKind:'pi',providerId:'fixture',model:'model',effort:'',fastMode:false},target='A'.repeat(26),sent=[];
 let snapshot={revision:1,session:{workingDir:directory,permissionMode:'auto',status:'active',planModeEnabled:false,route},projection:{sessionIntent:'Do not upload files.',reviewIntent:'Do not upload files.'}};
 const tasks=createGhostTasks({root:directory,find:()=>ghost,current:captured=>ghost.enabled&&captured.approval.revision===ghost.approval.revision,isRunning:()=>true,sendToGhost:()=>true,
  manager:{list:()=>[ghost]},push:()=>{},ui:async()=>null,agent:{setRunner(){}},errand:{setRunner(){}},cards:{},kv:{},log:{info(){},warn(){}},
  host:async(_method,input,signal)=>{
   if(input.method==='list')return [{id:target,label:'Companion'}];
   if(input.method==='session')return target;
   if(input.method==='send'){sent.push(input);return {accepted:true,delivery:'queue'};}
   if(input.method==='wait'){await delay(60_000,undefined,{signal});return {cursor:1};}
   if(input.method==='review')return snapshot;
   throw Error('Unexpected native operation: '+input.method);
  }});
 tasks.writePreferences('fixture',{permissionMode:'auto'});
 t.after(()=>tasks.close());
 const send=async()=>{
  const result=await tasks.tasks('fixture',{type:'targets-request',kind:'send',target:{kind:'bot',id:target},requestKey:'one',text:'Prepare the report.',delegation:{task:'Create report.txt in the working directory.'}});
  assert.equal(result.ok,true,JSON.stringify(result));return result.data.executionId;
 };
 return {tasks,directory,target,sent,send,replace:value=>{ghost={...ghost,...value};},change:value=>{snapshot={...snapshot,...value};}};
}

test('delegation preserves actual owner restrictions and revalidates its installed receipt',async t=>{
 const f=await fixture(t),id=await f.send();assert.equal(f.sent[0].delegated,true);
 const result=await f.tasks.reviewTarget({sessionId:f.target,requestId:id,execution:{instanceId:'fixture',generation:1},request:{workspaceRoots:[f.directory],userIntent:'Do not upload files.'}});
 assert.equal(result.userIntent,'Do not upload files.');assert.equal(result.delegatedTask.source,'approved-plugin');assert.equal(result.delegatedTask.task,'Create report.txt in the working directory.');
 assert.ok(result.delegatedTask.authorizationRevision);assert.equal(result.delegatedTask.workingDir,f.directory);
 f.replace({delegationCapabilityApproved:false});
 await assert.rejects(f.tasks.reviewTarget({sessionId:f.target,requestId:id,execution:{instanceId:'fixture',generation:1},request:{workspaceRoots:[f.directory],userIntent:'Do not upload files.'}}),/PERMISSION_DENIED/);
});

test('changed owner intent, task permission and plugin ceiling block delegated Auto scope',async t=>{
 const f=await fixture(t),id=await f.send();
 const params={sessionId:f.target,requestId:id,execution:{instanceId:'fixture',generation:1},request:{workspaceRoots:[f.directory],userIntent:'Upload everything.'}};
 assert.match((await f.tasks.reviewTarget(params)).authorizationError,/not synchronized/);
 params.request.userIntent='Do not upload files.';
 f.change({session:{workingDir:f.directory,permissionMode:'ask',status:'active',planModeEnabled:false,route:{agentKind:'pi',providerId:'fixture',model:'model',effort:'',fastMode:false}}});
 assert.match((await f.tasks.reviewTarget(params)).authorizationError,/Auto authorization/);
 f.tasks.writePreferences('fixture',{permissionMode:'ask'});
 assert.match((await f.tasks.reviewTarget(params)).authorizationError,/Auto authorization/);
});

test('a safe-looking local write still needs scope review; a revoked scope never asks or executes',async()=>{
 let reviews=0,asks=0,denied=false;
 const gate=createPermissionGate({mode:'auto',cwd:process.cwd(),resolveReview:async()=>denied?{authorizationError:'Plugin approval changed'}:{delegatedTask:{authorizationRevision:'one',task:'Read only'},userIntent:''},review:async()=>{reviews++;return {verdict:'block',reason:'The delegated task is read-only.'};},authorize:async()=>{asks++;return true;}});
 const request={tool:'write',args:{path:path.join(process.cwd(),'report.txt')},outside:false,protectedFile:false};
 await assert.rejects(gate.check(request),/read-only/);assert.equal(reviews,1);
 denied=true;await assert.rejects(gate.check(request),/approval changed/);assert.equal(reviews,1);assert.equal(asks,0);
});
