import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createOneBotRuntime} from './runtime.mjs';
import {createWinOneSession} from '../session.mjs';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');

test('a real Pi turn receives Cindy identity and uses session-bound memory, Skill and project tools with approval',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'pi-onebot-'));
 const runtime=createOneBotRuntime({directory});
 const id='ABCDEFGHIJKLMNOPQRSTUVWXYZ';
 const requests=[],approvals=[];
 const project=path.join(directory,'registered-project');await fs.mkdir(project);
 const catalog=[{path:project,name:'Registered project',exists:true}];
 const replies=[{tool:'memory_write',args:{type:'user',name:'language',title:'语言偏好',description:'用户偏好中文',body:'使用简体中文。'}},{tool:'save_teammate_skill',args:{slug:'check-report',name:'check-report',description:'核对报告',body:'先读取报告再核查数据。'}},{text:'已保存。'}];
 const server=http.createServer(async(req,res)=>{
  try{
   let bytes='';for await(const chunk of req)bytes+=chunk;
   const body=JSON.parse(bytes);
   if(req.url==='/onebot/tool'){
    assert.equal(req.headers.authorization,'Bearer fixture-onebot-tool-token');
    const value=await runtime.call('session.tool',{sessionId:id,method:body.method,args:body.args,catalog,ownerAuthorized:true});
    res.setHeader('Content-Type','application/json');res.end(JSON.stringify(value));return;
   }
   assert.equal(req.headers.authorization,'Bearer fixture-onebot-model-token');
   requests.push(body);const next=replies.shift();assert.ok(next);
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   const emit=(delta,finish=null)=>res.write(`data: ${JSON.stringify({id:'fixture',object:'chat.completion.chunk',created:1,model:'fixture/model',choices:[{index:0,delta,finish_reason:finish}]})}\n\n`);
   emit({role:'assistant'});
   if(next.tool){emit({tool_calls:[{index:0,id:'call-'+requests.length,type:'function',function:{name:next.tool,arguments:JSON.stringify(next.args)}}]});emit({},'tool_calls');}
   else{emit({content:next.text});emit({},'stop');}
   res.end('data: [DONE]\n\n');
  }catch(error){res.writeHead(500);res.end(error.message);}
 });
 let agent;
 try{
  await runtime.call('profiles.create',{input:{id:'pi-fixture',name:'资料助手',identitySource:'ONEBOT_IDENTITY_FIXTURE'}});
  await runtime.call('sessions.bind',{botId:'pi-fixture',sessionId:id});
  const [option]=await runtime.call('projects.query',{botId:'pi-fixture',catalog});
  replies.splice(0,0,{tool:'list_teammate_projects',args:{include_available:true}},{tool:'set_teammate_project',args:{project_id:option.id,linked:true}});
  const context=await runtime.call('runtime.context',{sessionId:id});
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const address=`http://127.0.0.1:${server.address().port}`;
  agent=await createWinOneSession({cwd:context.workspace,agentDir:path.join(directory,'agent'),sessionDirectory:path.join(directory,'sessions'),baseUrl:address+'/v1',key:'fixture-onebot-model-token',model:{id:'fixture/model',name:'Fixture'},permission:'ask',skillDirectories:context.skillDirectories,
   onebot:{...context,baseUrl:address+'/onebot',key:'fixture-onebot-tool-token'}},{authorize:async request=>{approvals.push(request.tool);return true;}});
  await agent.session.prompt('关联登记项目，记住我的语言偏好，并将经过验证的流程保存为技能。');
  assert.ok(JSON.stringify(requests[0].messages).includes('ONEBOT_IDENTITY_FIXTURE'));
  assert.deepEqual(approvals,['list_teammate_projects','set_teammate_project','memory_write','save_teammate_skill']);
  assert.equal((await runtime.call('projects.query',{botId:'pi-fixture',catalog}))[0].linked,true);
  assert.equal((await runtime.call('memory.read',{botId:'pi-fixture',filename:'user_language.md'})).body,'使用简体中文。');
  assert.equal((await runtime.call('skills.read',{botId:'pi-fixture',slug:'check-report'})).body,'先读取报告再核查数据。');
  await assert.rejects(runtime.call('session.tool',{sessionId:'not-owned',method:'memory.list',args:{}}),/不属于 OneBot/);
  const refreshed=await runtime.call('runtime.context',{sessionId:id});
  assert.ok(refreshed.systemPrompt.includes('user_language.md'));
  assert.equal(refreshed.skillDirectories.length,1);
 }finally{await agent?.close();await runtime.close();await new Promise(resolve=>server.close(resolve));}
});
