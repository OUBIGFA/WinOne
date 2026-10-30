import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
async function fixture(){
 const directory=await fs.mkdtemp(path.join(base,'projects-'));
 const project=path.join(directory,'registered');
 await fs.mkdir(project);
 const runtime=createOneBotRuntime({directory});
 for(const id of ['reader','other'])await runtime.call('profiles.create',{input:{id,name:id,capabilities:{permissions:'ask',mcpMode:'allowlist',mcpServers:[]}}});
 return {directory,project,runtime,catalog:[{path:project,name:'Registered project',exists:true}]};
}

test('linked projects use the original workbench, retain legacy tasks and never grant tool access',async()=>{
 const f=await fixture();let runtime=f.runtime;
 const file=path.join(f.directory,'bots','reader','workbench.json');
 try{
  await fs.writeFile(file,JSON.stringify({directories:[],tasks:{'idea:follow-up':{title:'Keep this legacy judgment',verdict:'idea',next:null,project:f.project,updatedAt:'2026-10-27T00:00:00Z'}}}));
  const before=await runtime.call('profiles.get',{botId:'reader'});
  const options=await runtime.call('projects.query',{botId:'reader',catalog:f.catalog});
  assert.equal(options.length,1);assert.equal(options[0].name,'Registered project');assert.equal(options[0].linked,false);
  const linked=await runtime.call('projects.link',{botId:'reader',id:options[0].id,linked:true,catalog:f.catalog});
  assert.equal(linked[0].linked,true);
  const stored=JSON.parse(await fs.readFile(file,'utf8'));
  assert.equal(stored.directories[0].path,f.project);
  assert.ok(stored.tasks['idea:follow-up']);
  const after=await runtime.call('profiles.get',{botId:'reader'});
  assert.deepEqual(after.capabilities,before.capabilities);
  assert.equal((await runtime.call('projects.query',{botId:'other',catalog:f.catalog}))[0].linked,false);
  await runtime.close();runtime=createOneBotRuntime({directory:f.directory});
  await fs.rename(f.project,f.project+'-offline');
  const missing=await runtime.call('projects.query',{botId:'reader',catalog:[]});
  assert.equal(missing[0].id,options[0].id);assert.equal(missing[0].linked,true);assert.equal(missing[0].available,false);
  assert.deepEqual(await runtime.call('projects.link',{botId:'reader',id:missing[0].id,linked:false,catalog:[]}),[]);
 }finally{await runtime.close();}
});

test('project changes reject unknown choices and preserve a damaged workbench for repair',async()=>{
 const f=await fixture();
 try{
  await assert.rejects(f.runtime.call('projects.link',{botId:'reader',id:'not-a-listed-choice',linked:true,catalog:f.catalog}),/unavailable|NOT_FOUND/);
  const file=path.join(f.directory,'bots','reader','workbench.json');
  await fs.writeFile(file,'{damaged');
  await assert.rejects(f.runtime.call('projects.query',{botId:'reader',catalog:f.catalog}));
  await assert.rejects(f.runtime.call('projects.link',{botId:'reader',id:'unknown',linked:true,catalog:f.catalog}));
  assert.equal(await fs.readFile(file,'utf8'),'{damaged');
 }finally{await f.runtime.close();}
});

test('a linked workbench file cannot redirect project writes outside the Bot Home',async()=>{
 const f=await fixture();
 try{
  const other=path.join(f.directory,'unrelated.json');
  const original=JSON.stringify({directories:[],tasks:{}});
  await fs.writeFile(other,original);
  await fs.link(other,path.join(f.directory,'bots','reader','workbench.json'));
  await assert.rejects(f.runtime.call('projects.query',{botId:'reader',catalog:f.catalog}),/链接|UNSAFE_PATH/);
  assert.equal(await fs.readFile(other,'utf8'),original);
 }finally{await f.runtime.close();}
});

test('project tools are bound to a visible owner session and require the current owner for changes',async()=>{
 const f=await fixture();
 try{
  const main='ABCDEFGHIJKLMNOPQRSTUVWXYZ',channel='BCDEFGHIJKLMNOPQRSTUVWXYZA';
  await f.runtime.call('sessions.bind',{botId:'reader',sessionId:main});
  await f.runtime.call('sessions.bind',{botId:'reader',sessionId:channel,role:'channel',routeKey:'channel'});
  const [option]=await f.runtime.call('projects.query',{botId:'reader',catalog:f.catalog});
  await assert.rejects(f.runtime.call('session.tool',{sessionId:channel,method:'projects.list',args:{include_available:true},catalog:f.catalog,ownerAuthorized:true}),/主任务|MAIN_TASK/);
  await assert.rejects(f.runtime.call('session.tool',{sessionId:main,method:'projects.set',args:{project_id:option.id,linked:true},catalog:f.catalog,ownerAuthorized:false}),/主人|OWNER/);
  const result=await f.runtime.call('session.tool',{sessionId:main,method:'projects.set',args:{project_id:option.id,linked:true,botId:'other'},catalog:f.catalog,ownerAuthorized:true});
  assert.equal(result.projects[0].project_id,option.id);
  assert.equal(result.projects[0].working_dir,f.project);
  assert.equal(result.projects[0].linked,true);
  assert.deepEqual(result.projects[0].context_locations,['AGENTS.md','CLAUDE.md','README.md','.cindy/project-knowledge/TOC.md']);
  assert.equal((await f.runtime.call('projects.query',{botId:'other',catalog:f.catalog}))[0].linked,false);
 }finally{await f.runtime.close();}
});
