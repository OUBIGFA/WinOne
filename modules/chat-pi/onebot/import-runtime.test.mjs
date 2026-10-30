import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('Run through build/test-onebot.mjs');

// Use the original source readers/import transaction, real encrypted vault,
// actual command process and routine engine. No third-party user profile is read.
test('Hermes and OpenClaw imports preserve sources, expose private command tools and guard unaccepted handovers',async()=>{
 const root=await fs.mkdtemp(path.join(base,'import-runtime-'));
 const homes={HERMES_HOME:path.join(root,'hermes'),OPENCLAW_STATE_DIR:path.join(root,'openclaw'),OPENCLAW_CONFIG_PATH:path.join(root,'openclaw','openclaw.json'),OPENCLAW_WORKSPACE_DIR:path.join(root,'openclaw','workspace')};
 const previous=Object.fromEntries(Object.keys(homes).map(key=>[key,process.env[key]]));
 let runtime,sessionCount=0;
 try{
  Object.assign(process.env,homes);
  for(const directory of [homes.HERMES_HOME,homes.OPENCLAW_WORKSPACE_DIR]){
   await fs.mkdir(path.join(directory,'skills','research'),{recursive:true});
   await fs.writeFile(path.join(directory,'SOUL.md'),'An evidence-based imported partner.');
   await fs.writeFile(path.join(directory,'MEMORY.md'),'Retain the original research notes.');
   await fs.writeFile(path.join(directory,'skills','research','SKILL.md'),'---\nname: research\ndescription: Inspect evidence\n---\nRead the original evidence before drawing conclusions.\n');
  }
  await fs.writeFile(path.join(homes.HERMES_HOME,'config.yaml'),'name: Hermes fixture\n');
  await fs.writeFile(homes.OPENCLAW_CONFIG_PATH,JSON.stringify({agents:{list:[{id:'main',name:'OpenClaw fixture',workspace:homes.OPENCLAW_WORKSPACE_DIR}]}}));
  const jobs=[];
  for(const [kind,directory] of [['hermes',homes.HERMES_HOME],['openclaw',homes.OPENCLAW_STATE_DIR]]){
   await fs.writeFile(path.join(directory,'.env'),'IMPORT_TEST_SECRET=private-import-credential\n');
   await fs.mkdir(path.join(directory,'cron'),{recursive:true});
   const file=path.join(directory,'cron','jobs.json');
   const job={id:'paused-source',name:'Imported paused routine',enabled:false,schedule:{kind:'cron',expr:'0 9 * * *',tz:'UTC'},...(kind==='hermes'?{prompt:'Read research evidence.'}:{agentId:'main',payload:{kind:'agentTurn',message:'Read research evidence.'}})};
   const sourceJobs=[job,{...job,id:'active-source',name:'Imported active routine',enabled:true}];
   const bytes=JSON.stringify(kind==='hermes'?sourceJobs:{jobs:sourceJobs});await fs.writeFile(file,bytes);jobs.push({file,bytes});
  }
  runtime=createOneBotRuntime({directory:path.join(root,'owner'),masterKey:Buffer.alloc(32,7).toString('base64'),lifecycle:{closeSession:async()=>{},deleteSessions:async()=>{},recycleHome:async()=>{}},host:{
   routeAvailable:async()=>false,
   createMessage:async()=>({created:true}),
   createCanonicalSession:async input=>{const sessionId='A'.repeat(25)+String.fromCharCode(66+sessionCount++);await runtime.call('sessions.bind',{botId:input.botId,sessionId,routeKey:'canonical',role:'canonical'});return {canonicalSessionId:sessionId,created:true};},
  }});
  const sources=await runtime.call('import.sources');assert.equal(sources.length,2);
  for(const source of sources){
   const preview=await runtime.call('import.preview',{sourceId:source.id});
   assert.ok(preview.entries.some(item=>item.category==='personality'));
   assert.ok(preview.entries.some(item=>item.category==='skills'));
   assert.ok(preview.entries.some(item=>item.category==='automations'));
   assert.ok(!JSON.stringify(preview).includes('private-import-credential'));
   const selection={previewId:preview.id,requestId:'import_fixture_'+source.kind,name:'Imported '+source.kind,entryIds:preview.entries.map(item=>item.id),takeover:false,deferSetup:true};
   let result=await runtime.call('import.start',{selection});
   const deadline=Date.now()+10000;
   while(!result.saved&&Date.now()<deadline){await new Promise(resolve=>setTimeout(resolve,20));result=await runtime.call('import.status',{requestId:selection.requestId});}
   assert.equal(result.saved,true,JSON.stringify(result));
   const profile=await runtime.call('profiles.get',{botId:result.botId});
   assert.match(profile.identitySource,/evidence-based/);
   assert.ok((await runtime.call('skills.list',{botId:result.botId})).some(skill=>skill.slug==='research'));
   const tools=await runtime.call('importedTools.list',{sessionId:result.canonicalSessionId});
   assert.ok(tools.some(tool=>tool.name==='run_command'));
   const output=await runtime.call('importedTools.call',{sessionId:result.canonicalSessionId,callId:'import-command',name:'run_command',args:{command:'echo %IMPORT_TEST_SECRET%'}});
   assert.equal(output.isError,false);assert.ok(!JSON.stringify(output).includes('private-import-credential'));assert.match(JSON.stringify(output),/IMPORT_TEST_SECRET/);
   const routines=await runtime.call('routines.list',{botId:result.botId});const imported=routines.find(row=>row.name==='Imported paused routine');assert.ok(imported);assert.equal(imported.enabled,false);
   const guarded=routines.find(row=>row.name==='Imported active routine');assert.ok(guarded);assert.equal(guarded.enabled,false);
   await assert.rejects(runtime.call('routines.runNow',{botId:result.botId,id:guarded.id}),/AUTOMATION_HANDOVER_REQUIRED/);
   const replay=await runtime.call('import.start',{selection});assert.equal(replay.botId,result.botId);
   const pending=runtime.call('importedTools.call',{sessionId:result.canonicalSessionId,callId:'cancel-command',name:'run_command',args:{command:`"${process.execPath}" -e "require('fs').writeFileSync('import-pid.txt',String(process.pid));setInterval(()=>{},1000)"`}}).then(value=>({value}),error=>({error}));
   const pidFile=path.join(root,'owner','bots',result.botId,'workspace','import-pid.txt');let pid;
   for(let attempt=0;attempt<100&&!pid;attempt++){pid=await fs.readFile(pidFile,'utf8').then(Number,()=>undefined);if(!pid)await new Promise(resolve=>setTimeout(resolve,20));}
   assert.ok(pid,'imported command did not start');
   await runtime.call('importedTools.cancel',{sessionId:result.canonicalSessionId,callId:'cancel-command'});
   const cancelled=await pending;assert.ok(cancelled.error||cancelled.value?.isError,'cancelled command reported success');
   let exited=false;
   for(let attempt=0;attempt<100&&!exited;attempt++){try{process.kill(pid,0);}catch{exited=true;}if(!exited)await new Promise(resolve=>setTimeout(resolve,20));}
   assert.equal(exited,true,'imported command survived cancellation');
   await runtime.call('lifecycle.action',{botId:result.botId,action:'delete',confirmName:profile.name,keepHistory:false});
   await assert.rejects(runtime.call('profiles.get',{botId:result.botId}),error=>error.code==='NOT_FOUND');
   await runtime.call('import.start',{selection});
   await assert.rejects(runtime.call('profiles.get',{botId:result.botId}),error=>error.code==='NOT_FOUND');
  }
  for(const job of jobs)assert.equal(await fs.readFile(job.file,'utf8'),job.bytes,'unaccepted source scheduling must remain untouched');
 }finally{
  await runtime?.close();
  for(const [key,value] of Object.entries(previous)){if(value===undefined)delete process.env[key];else process.env[key]=value;}
 }
});
