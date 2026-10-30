import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createDurableSession} from '../durable-session.mjs';
import {BACKGROUND_CONTEXT as context} from '@earendil-works/chord/context';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
const until=async check=>{for(let n=0;n<400;n++){if(check())return;await new Promise(resolve=>setTimeout(resolve,10));}throw Error('fixture timed out');};

async function fixture(respond,beforePolicy=async()=>{}){
 const directory=await fs.mkdtemp(path.join(base,'receipt-')),cwd=path.join(directory,'workspace');await fs.mkdir(cwd);
 await fs.writeFile(path.join(cwd,'proof.txt'),'INPUT_SCOPE_PROOF');
 const policies=[],requests=[],events=[];
 const server=http.createServer(async(req,res)=>{
  try{
   let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);
   if(req.url==='/onebot/policy'){
    policies.push(body);await beforePolicy(body,policies.length);
    if(!body.requestId){res.writeHead(403);res.end('Missing input receipt');return;}
    res.setHeader('Content-Type','application/json');res.end(JSON.stringify({allowed:true,requestId:body.requestId,channel:'telegram',group:true}));return;
   }
   requests.push(body);const next=await respond(body,requests.length);
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   const emit=(delta,finish_reason=null)=>res.write(`data: ${JSON.stringify({id:'fixture',object:'chat.completion.chunk',model:'fixture/model',choices:[{index:0,delta,finish_reason}]})}\n\n`);
   emit({role:'assistant'});
   if(next.tool){emit({tool_calls:[{index:0,id:'same-call-id',type:'function',function:{name:next.tool,arguments:JSON.stringify(next.args)}}]});emit({},'tool_calls');}
   else{emit({content:next.text??'Done'});emit({},'stop');}
   res.end('data: [DONE]\n\n');
  }catch(error){res.writeHead(500);res.end(String(error));}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const address=`http://127.0.0.1:${server.address().port}`;
 const config={cwd,agentDir:path.join(directory,'agent'),sessionDirectory:path.join(directory,'sessions'),baseUrl:address+'/v1',key:'fixture-model-private-key',model:{id:'fixture/model',name:'Fixture'},permission:'bypassPermissions',runtimeSettings:{autoRetry:false,backgroundCompaction:false},
  onebot:{homeDir:directory,receiptScoped:true,baseUrl:address+'/onebot',key:'fixture-onebot-private-key'}};
 return {config,policies,requests,events,close:()=>new Promise(resolve=>server.close(resolve))};
}

for(const nested of [false,true])test(`OneBot ${nested?'nested Code Mode':'direct'} tool uses the generating input, not a newer transcript entry`,async()=>{
 let release;const gate=new Promise(resolve=>{release=resolve;});
 const f=await fixture(async(_body,index)=>{if(index===1){await gate;return nested?{tool:'codemode',args:{code:"text(await tools.read({path:'proof.txt'}));"}}:{tool:'read',args:{path:'proof.txt'}};}return {text:'Done'};});
 let agent;
 try{
  agent=await createDurableSession(f.config,{event:value=>f.events.push(value)});
  const running=agent.session.prompt('Read the proof.',{requestId:'guest-input',inputReceipt:{id:'guest-input',text:'Read the proof.'}});
  await until(()=>f.requests.length===1);
  // A later host write is not part of the model request already in flight.
  // This reproduces the old latest-user lookup borrowing a different owner.
  await agent.root.commit(tx=>tx.appendEntry(agent.root.id,{kind:'fixture.later',model:[{role:'user',content:'Unrelated later input',timestamp:Date.now()}],data:{receipt:{id:'owner-input'}}}),context);
  release();await running;
  assert.ok(f.policies.length);assert.ok(f.policies.every(value=>value.requestId==='guest-input'),JSON.stringify({policies:f.policies,notices:f.events.filter(value=>value.type==='notice')}));
  assert.match(JSON.stringify(f.requests.at(-1).messages),/INPUT_SCOPE_PROOF/);
 }finally{release();await agent?.close();await f.close();}
});

test('OneBot replay-safe tools retain their original input receipt after closing and reopening SQLite',async()=>{
 let release;const gate=new Promise(resolve=>{release=resolve;});
 const f=await fixture((_body,index)=>index===1?{tool:'read',args:{path:'proof.txt'}}:{text:'Recovered'},async(_body,index)=>{if(index===1)await gate;});
 let agent;
 try{
  agent=await createDurableSession(f.config);
  const running=agent.session.prompt('Read before restart.',{requestId:'restart-receipt',inputReceipt:{id:'restart-receipt',text:'Read before restart.'}}).catch(()=>{});
  await until(()=>f.policies.length===1);
  const file=agent.state().file;
  await agent.close();release();await running;
  agent=await createDurableSession({...f.config,sessionFile:file});
  await agent.resume();await agent.root.waitForIdle(context);
  assert.equal(f.policies.length,2);
  assert.ok(f.policies.every(value=>value.requestId==='restart-receipt'),JSON.stringify(f.policies));
  assert.match(JSON.stringify(f.requests.at(-1).messages),/INPUT_SCOPE_PROOF/);
 }finally{release();await agent?.close();await f.close();}
});

test('OneBot confirmations and questions carry their exact receipt, and an unbound input cannot borrow an earlier one',async()=>{
 const replies=[{tool:'write',args:{path:'first.txt',content:'ONCE'}},{tool:'ask_user_question',args:{questions:[{question:'选择',options:['甲','乙']}]}},{text:'Done'},
  {tool:'read',args:{path:'proof.txt'}},{text:'Denied'}];
 const f=await fixture(()=>replies.shift()),approvals=[],questions=[];let agent;
 try{
  agent=await createDurableSession(f.config,{authorize:async request=>{approvals.push(request);return true;},question:async request=>{questions.push(request);return ['乙'];}});
  await agent.session.prompt('Write and ask.',{requestId:'owner-first',inputReceipt:{id:'owner-first',text:'Write and ask.'}});
  assert.equal(approvals.length,1);assert.equal(approvals[0].requestId,'owner-first');assert.equal(questions[0].requestId,'owner-first');
  await agent.session.prompt('Read without a bound receipt.',{requestId:'unbound'});
  assert.equal(f.policies.at(-1).requestId,null);
  assert.match(JSON.stringify(f.requests.at(-1).messages),/Missing input receipt/);
 }finally{await agent?.close();await f.close();}
});
