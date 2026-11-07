// The ordinary Chat harness uses the same immutable input proof as OneBot;
// delivery judgement must never borrow a later user entry's identity.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createDurableSession} from '../durable-session.mjs';
import {BACKGROUND_CONTEXT as context} from '@earendil-works/chord/context';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('Use build/test-onebot.mjs with an isolated output');

for(const nested of [false,true])test(`task delivery ${nested?'through Code Mode':'directly'} keeps the original Chat input`,async()=>{
 const directory=await fs.mkdtemp(path.join(base,'task-delivery-')),requests=[],reports=[];
 let release,started;
 const gate=new Promise(resolve=>{release=resolve;}),first=new Promise(resolve=>{started=resolve;});
 const server=http.createServer(async(req,res)=>{
  try {
   let raw='';for await(const part of req)raw+=part;const body=JSON.parse(raw);
   if(req.url==='/v1/task/delivery'){
    reports.push(body);res.setHeader('Content-Type','application/json');res.end(JSON.stringify({ok:true}));return;
   }
   requests.push(body);
   if(requests.length===1){started();await gate;}
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   const emit=(delta,finish_reason=null)=>res.write(`data: ${JSON.stringify({choices:[{index:0,delta,finish_reason}]})}\n\n`);
   if(requests.length===1){
    const name=nested?'codemode':'report_task_delivery';
    const args=nested?{code:'text(await tools.report_task_delivery({final_delivery:true}));'}:{final_delivery:true};
    emit({tool_calls:[{index:0,id:'delivery',type:'function',function:{name,arguments:JSON.stringify(args)}}]});emit({},'tool_calls');
   }else {emit({content:'Delivered'});emit({},'stop');}
   res.end('data: [DONE]\n\n');
  }catch(error){res.writeHead(500);res.end(String(error));}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 let agent;
 try {
  agent=await createDurableSession({cwd:directory,agentDir:path.join(directory,'agent'),sessionDirectory:path.join(directory,'sessions'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-only-delivery-key',model:{id:'fixture',name:'Fixture'},permission:'bypassPermissions',taskDelivery:true,runtimeSettings:{autoRetry:false,backgroundCompaction:false}});
  const running=agent.session.prompt('Deliver the result.',{requestId:'original',inputReceipt:{id:'original',text:'Deliver the result.'}});
  await first;
  await agent.root.commit(tx=>tx.appendEntry(agent.root.id,{kind:'fixture.later',model:[{role:'user',content:'Unrelated input',timestamp:Date.now()}],data:{receipt:{id:'later'}}}),context);
  release();await running;
  assert.deepEqual(reports,[{requestId:'original',finalDelivery:true}]);
  const lastTool = requests.at(-1).messages.findLast(message => message.role === 'tool');
  assert.ok(lastTool, 'the model must see the delivery receipt');
  assert.match(typeof lastTool.content === 'string' ? lastTool.content : JSON.stringify(lastTool.content), /ok/);
 } finally {release();await agent?.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
