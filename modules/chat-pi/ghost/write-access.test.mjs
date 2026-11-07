import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createGhostTaskService} from './platform-task-service.mjs';
import {setGhostDataRoot} from './seams/app-session.ts';
import {getLocale,setGhostLanguage} from './seams/i18n.ts';

const repo=fileURLToPath(new URL('../../../',import.meta.url));
async function fixture(t){
 // Each fixture owns its language; another SDK test may deliberately switch
 // the process to English before this permission dialog is constructed.
 const language=getLocale();setGhostLanguage('zh-CN');t.after(()=>setGhostLanguage(language));
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(repo,'_temp/ghost-task-tests');await fs.mkdir(base,{recursive:true});
 const root=await fs.mkdtemp(path.join(base,'write-'));setGhostDataRoot(root);
 const ghost={enabled:true,taskCapabilityApproved:true,namespace:null,approval:{state:'approved',revision:'one'},manifest:{id:'fixture',name:'Fixture',agent:{tasks:true}}},rows=new Map(),prompts=[];
 let confirmed=false;
 const service=createGhostTaskService({root,find:()=>ghost,current:()=>true,cards:{},log:{warn(){}},ui:async(_kind,request)=>{prompts.push(request);return {confirmed};},host:async(_kind,input)=>{
  if(input.method==='resolve-route')return input.route??{agentKind:'pi',providerId:'vendor',model:'model',effort:'',fastMode:false};
  if(input.method==='create'){const row={taskId:input.taskId,title:input.title,status:'active',revision:1,permissionMode:input.permissionMode,workingDir:root,resolvedConfig:input.route};rows.set(row.taskId,row);return row;}
  if(input.method==='get')return rows.get(input.taskId);
  if(input.method==='inspect')return {running:false,pending:[],approvals:[],questions:[]};
  if(input.method==='assert-idle'||input.method==='assert-unstarted')return true;
  if(input.method==='permission'||input.method==='permission-restore'){const row=rows.get(input.taskId);assert.equal(input.expectedRevision,row.revision);row.permissionMode=input.mode;row.revision++;return {...row};}
  throw Error('Unexpected native operation: '+input.method);
 }});
 t.after(()=>service.close());
 const task=await service.handle('fixture',{kind:'create',requestKey:'create',title:'My task'});
 return {service,task,prompts,ghost,rows,confirm:value=>{confirmed=value;}};
}

test('a refusal does not reopen on mode changes; native owner retry restores the original mode',async t=>{
 const f=await fixture(t);
 assert.deepEqual(await f.service.handle('fixture',{kind:'requestWriteAccess',taskId:f.task.taskId,mode:'acceptEdits'}),{granted:false});
 assert.deepEqual(await f.service.handle('fixture',{kind:'requestWriteAccess',taskId:f.task.taskId,mode:'auto'}),{granted:false});
 assert.equal(f.prompts.length,1);assert.match(f.prompts[0].body,/允许/);
 f.confirm(true);
 const retry=await f.service.retryWriteAccess('fixture',f.task.taskId);
 assert.equal(retry.granted,true);assert.equal(retry.task.permissionMode,'acceptEdits');assert.equal(f.prompts.length,2);
 assert.equal(f.service.preferences('fixture').permissionMode,'acceptEdits');
});

test('opting out of the Host task card grants only the target task and preserves prior defaults',async t=>{
 const f=await fixture(t);f.service.writePreferences('fixture',{permissionMode:'ask'});
 f.ghost.manifest.showTaskPreferences=false;f.confirm(true);
 const granted=await f.service.handle('fixture',{kind:'requestWriteAccess',taskId:f.task.taskId,mode:'auto'});
 assert.equal(granted.granted,true);assert.equal(granted.task.permissionMode,'auto');assert.deepEqual(f.service.preferenceDetails('fixture').taskPreferences,{});
 f.ghost.manifest.showTaskPreferences=true;
 assert.equal(f.service.preferences('fixture').permissionMode,'ask','the hidden card was not read or rewritten');
});
