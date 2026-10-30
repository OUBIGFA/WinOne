import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createDurableSession} from '../durable-session.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
for(const mode of ['recoverable','permanent','retry-disabled','background','ack-rejected'])test(`OneBot model chain handles ${mode} errors without repeating a completed tool`,async()=>{
 const directory=await fs.mkdtemp(path.join(base,'model-fallback-')),cwd=path.join(directory,'workspace');await fs.mkdir(cwd);
 const requests=[],acknowledgements=[],childRequests=[];
 let childStarted,releaseChild;
 const started=new Promise(resolve=>{childStarted=resolve;}),released=new Promise(resolve=>{releaseChild=resolve;});
 const server=http.createServer(async(req,res)=>{
  try{
   let raw='';for await(const part of req)raw+=part;const body=JSON.parse(raw);
   if(req.url==='/onebot/runtime.model'){
    assert.equal(req.headers.authorization,'Bearer fixture-onebot');acknowledgements.push(body);
    if(mode==='ack-rejected'){res.writeHead(409);res.end('Model choice has changed');return;}
    res.setHeader('Content-Type','application/json');res.end('{}');return;
   }
   const child=body.messages.some(message=>message.role==='user'&&(typeof message.content==='string'?message.content:message.content?.map(part=>part.text??'').join(''))==='Independent background task');
   if(child){childRequests.push(body);if(childRequests.length===1){childStarted();await released;}}
   else{
    requests.push(body);
    if(requests.length===(mode==='background'?3:2)){
     if(mode==='background')await started;
     res.writeHead(mode==='permanent'?400:503,{'Content-Type':'application/json'});res.end(JSON.stringify({error:{message:mode==='permanent'?'Invalid request':'Service temporarily unavailable'}}));return;
    }
   }
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   const emit=(delta,finish=null)=>res.write(`data: ${JSON.stringify({id:'fixture',object:'chat.completion.chunk',model:body.model,choices:[{index:0,delta,finish_reason:finish}]})}\n\n`);
   emit({role:'assistant'});
   if(child&&childRequests.length===1){emit({tool_calls:[{index:0,id:'child-read',type:'function',function:{name:'read',arguments:JSON.stringify({path:'proof.txt'})}}]});emit({},'tool_calls');}
   else if(!child&&requests.length===1){emit({tool_calls:[{index:0,id:'write-proof',type:'function',function:{name:'write',arguments:JSON.stringify({path:'proof.txt',content:'Written once before the failure'})}}]});emit({},'tool_calls');}
   else if(!child&&mode==='background'&&requests.length===2){emit({tool_calls:[{index:0,id:'spawn-child',type:'function',function:{name:'subagent',arguments:JSON.stringify({action:'spawn',name:'background',task:'Independent background task',thinkingLevel:'low',notify:false})}}]});emit({},'tool_calls');}
   else{emit({content:'Continued with the configured route'});emit({},'stop');}
   res.end('data: [DONE]\n\n');
  }catch(error){res.writeHead(500);res.end(error.message);}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const address=`http://127.0.0.1:${server.address().port}`;
 const first={id:'fixture/first',provider:'fixture',name:'First',reasoning:true,efforts:['low','high']},second={id:'fixture/second',provider:'fixture',name:'Second',reasoning:true,efforts:['low','high'],tools:false};
 const config={cwd,agentDir:path.join(directory,'agent'),sessionDirectory:path.join(directory,'sessions'),baseUrl:address+'/v1',key:'fixture-model-token',model:first,models:[first,second],thinkingLevel:'low',permission:'bypassPermissions',
  runtimeSettings:{codeMode:false,subagents:mode==='background',planning:false,autoRetry:mode!=='retry-disabled',maxRetries:1},
  onebot:{botId:'fixture',homeDir:directory,baseUrl:address+'/onebot',key:'fixture-onebot',memoryEnabled:false,modelChainKey:'fixture-chain',modelChain:[
   {harness:'pi',providerId:'fixture',model:'first',effort:'low',fastMode:false},
   {harness:'pi',providerId:'missing',model:'unavailable',effort:'low',fastMode:false},
   {harness:'pi',providerId:'fixture',model:'second',effort:'high',fastMode:true},
  ]}};
 let agent;
 try{
  agent=await createDurableSession(config);
  if(mode==='recoverable'||mode==='background')await agent.session.prompt('Write a proof then continue',{requestId:'original'});
  else await assert.rejects(agent.session.prompt('Write a proof then continue',{requestId:'original'}));
  assert.equal(await fs.readFile(path.join(cwd,'proof.txt'),'utf8'),'Written once before the failure');
  const messages=agent.session.sessionManager.getBranch().filter(entry=>entry.type==='message').map(entry=>entry.message);
  assert.equal(messages.filter(message=>message.role==='toolResult'&&message.toolName==='write').length,1);
  assert.equal(messages.filter(message=>message.role==='user').length,1);
  if(mode!=='recoverable'&&mode!=='background'){assert.equal(requests.length,2);assert.equal(acknowledgements.length,mode==='ack-rejected'?1:0);return;}
  assert.deepEqual(requests.map(request=>request.model),mode==='background'?['fixture/first','fixture/first','fixture/first','fixture/second']:['fixture/first','fixture/first','fixture/second']);
  assert.equal(requests.at(-1).reasoning_effort,'high');assert.equal(requests.at(-1).service_tier,'priority');
  assert.equal(requests.at(-1).tools?.length??0,0);
  assert.deepEqual(acknowledgements.map(({modelId,previousModelId,modelKey})=>({modelId,previousModelId,modelKey})),[{modelId:'fixture/second',previousModelId:'fixture/first',modelKey:'fixture-chain'}]);
  assert.equal(agent.modelIdentity().modelName,'Second');
  if(mode==='background'){
   releaseChild();const child=await agent.control('wait',{name:'background'});
   assert.equal(child.status,'completed');
   assert.deepEqual(childRequests.map(request=>({model:request.model,effort:request.reasoning_effort,fast:request.service_tier})),[{model:'fixture/first',effort:'low',fast:undefined},{model:'fixture/first',effort:'low',fast:undefined}]);
  }
 }finally{releaseChild();await agent?.close();await new Promise(resolve=>server.close(resolve));}
});
