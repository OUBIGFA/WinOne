import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';
import {applyChatEvent,emptyChatState} from '../../apps/desktop/frontend/src/features/chat-state.ts';
import {projectChatProcess} from '../../apps/desktop/frontend/src/features/chat-process.ts';

// A local SSE provider exercises the actual Pi loop and persisted receipts.
test('Pi consumes multiple follow-ups once and retries saved history without duplicating its user message',{timeout:25000},async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'followup-'));
 const workspace=path.join(root,'workspace'),sessions=path.join(root,'sessions');await fs.mkdir(workspace);
 let calls=0,release,releaseFollowup;const bodies=[];
 const server=http.createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);bodies.push(body);calls++;if(calls===1)await new Promise(resolve=>release=resolve);else if(calls===2)await new Promise(resolve=>releaseFollowup=resolve);res.writeHead(200,{'Content-Type':'text/event-stream'});res.end('data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Done"},"finish_reason":null}]}\n\ndata: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n');});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const child=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 let buffer='',ready=false,stderr='';const pending=new Map(),events=[];
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const i=buffer.indexOf('\n');if(i<0)break;const value=JSON.parse(buffer.slice(0,i));buffer=buffer.slice(i+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
 const until=async predicate=>{const end=Date.now()+12000;while(!predicate()){if(Date.now()>end)throw Error('Worker timeout: '+stderr);await new Promise(resolve=>setTimeout(resolve,10));}};
 const send=(id,method,params={})=>new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+'\n');});
 try{
  await until(()=>ready);assert.equal((await send('init','init',{cwd:workspace,sessionDirectory:sessions,agentDir:path.join(root,'agent'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-session-token',model:{id:'fixture',name:'Fixture'},permission:'ask'})).ok,true);
  assert.equal((await send('original','prompt',{text:'Original request'})).ok,true);await until(()=>calls===1);
  for(const id of ['follow-one','follow-two'])assert.equal((await send(id,'prompt',{text:id})).ok,true,'mid-turn input must reach Pi');
  release();
  // Force the first reply to reach the renderer while the next run is still
  // executing. Execution becoming idle must not outrun committed event delivery.
  await until(()=>calls>=2&&events.some(event=>event.type==='pi_event'&&event.eventType==='message_end'&&event.message?.role==='assistant'));
  assert.ok(!events.some(event=>event.type==='settled'),'queued follow-ups are still running');
  releaseFollowup();await until(()=>events.some(event=>event.type==='settled'));
  for(const id of ['original','follow-one','follow-two']){
   const liveRows=events.filter(event=>event.type==='pi_event'&&event.message?.role==='user'&&event.message?.turnId===id);
   assert.equal(liveRows.length,2,'each input emits start and end ('+id+') '+stderr+' '+JSON.stringify(events.filter(event=>event.type==='settled'||event.type==='notice').map(event=>event.type+':'+(event.status??event.text??''))));
   assert.ok(liveRows.every(event=>event.message.requestId===id),'live messages must carry the same receipt as the optimistic and saved rows');
  }
  const state=(await send('state','state')).value;assert.deepEqual(state.messages.filter(message=>message.role==='user').map(message=>message.content),['Original request','follow-one','follow-two']);
  const displayed=events.reduce((value,event)=>applyChatEvent(value,event),emptyChatState());
  const answers=messages=>messages.filter(message=>message.role==='assistant').map(message=>({turnId:message.turnId,content:message.content}));
  assert.deepEqual(answers(displayed.messages),answers(state.messages),'live events must show every follow-up reply before stop or history refresh');
  const rendered=projectChatProcess(displayed.messages,displayed.tools,displayed.running,displayed.operations).filter(item=>item.kind==='response'&&item.answer).map(item=>item.answer.turnId);
  assert.deepEqual(rendered,state.messages.filter(message=>message.role==='assistant').map(message=>message.turnId),'follow-up answers must survive the work-group projection');
  assert.ok(calls>=2);for(const id of ['follow-one','follow-two'])assert.equal((await send('status-'+id,'input_status',{id,text:id})).value.saved,true);
  const user=state.messages.findLast(message=>message.role==='user'),before=calls;const retiredAnswerCount=state.messages.filter(message=>message.role==='assistant').length;events.length=0;
  assert.equal((await send('retry-turn','retry',{entry:user.id})).ok,true);await until(()=>events.some(event=>event.type==='settled'));assert.equal(calls,before+1);
  const reset=events.findIndex(event=>event.type==='branch_reset'),firstReply=events.findIndex(event=>event.type==='delta'||event.type==='pi_event'&&event.message?.role==='assistant');
  assert.ok(reset>=0&&reset<firstReply,'replacement branch and user must reach the renderer before the first response event');
  let liveState={...emptyChatState(),...state};
  const historicalAnswers=state.messages.slice(0,state.messages.indexOf(user)).filter(message=>message.role==='assistant');
  for(const event of events){
   liveState=applyChatEvent(liveState,event);const items=projectChatProcess(liveState.messages,liveState.tools,liveState.running,liveState.operations);
   // Re-importing the selected branch is context setup, not a new assistant
   // turn. A duplicate during generation pushes every user row off screen even
   // when the final settled snapshot and relative user/reply order are correct.
   for(const previous of historicalAnswers){
    const rows=liveState.messages.filter(message=>message.role==='assistant'&&message.timestamp===previous.timestamp);
    assert.equal(rows.length,1,'a historical answer was replayed during '+event.type+': '+JSON.stringify(event));
    assert.equal(rows[0].turnId,previous.turnId,'a historical answer moved into the retry turn');
   }
   const answer=items.findIndex(item=>item.kind==='response'&&item.key==='retry-turn');
   if(answer>=0)assert.ok(items.slice(0,answer).some(item=>item.kind==='user'&&item.key==='retry-turn'),'a streaming replacement rendered above its input');
  }
  assert.deepEqual(bodies.at(-1).messages.filter(message=>message.role==='user').map(message=>typeof message.content==='string'?message.content:message.content.map(part=>part.text??'').join('')),['Original request','follow-one','follow-two'],'the actual model context must follow the replacement branch');
  assert.ok(bodies.at(-1).messages.filter(message=>message.role==='assistant'&&message.content).length<retiredAnswerCount,'the model must not see the answer being retried');
  const retried=(await send('retried-state','state')).value;assert.deepEqual(retried.messages.filter(message=>message.role==='user').map(message=>message.content),['Original request','follow-one','follow-two']);
  // The replacement branch keeps the earlier answer available. Reopening must
  // not restore the retired queued receipt as a second pending prompt.
  await send('close','close');
  const reopened=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
  let reopenedBuffer='',reopenedReady=false,reopenedStderr='';const reopenedPending=new Map();
  reopened.stderr.on('data',chunk=>reopenedStderr+=chunk);
  reopened.stdout.on('data',chunk=>{reopenedBuffer+=chunk;for(;;){const i=reopenedBuffer.indexOf('\n');if(i<0)break;const value=JSON.parse(reopenedBuffer.slice(0,i));reopenedBuffer=reopenedBuffer.slice(i+1);if(value.type==='ready')reopenedReady=true;else if(value.type==='response'){reopenedPending.get(value.id)?.(value);reopenedPending.delete(value.id);}}});
  const sendReopened=(id,method,params={})=>new Promise(resolve=>{reopenedPending.set(id,resolve);reopened.stdin.write(JSON.stringify({id,method,params})+'\n');});
  try{
   const reopenedDeadline=Date.now()+12000;while(!reopenedReady){if(Date.now()>reopenedDeadline)throw Error('Reopened worker timeout: '+reopenedStderr);await new Promise(resolve=>setTimeout(resolve,10));}
   await sendReopened('init','init',{cwd:workspace,sessionDirectory:sessions,sessionFile:retried.file,agentDir:path.join(root,'agent'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-session-token',model:{id:'fixture',name:'Fixture'},permission:'ask'});
   await new Promise(resolve=>setTimeout(resolve,250));
   assert.equal(calls,before+1,'a retired receipt must not run again after reopen');
   const reopenedState=(await sendReopened('reopened-state','state')).value;assert.deepEqual(reopenedState.messages.filter(message=>message.role==='user').map(message=>message.content),['Original request','follow-one','follow-two']);
   const editEntry=reopenedState.messages.findLast(message=>message.role==='user');
   assert.equal((await sendReopened('edit-turn','retry',{entry:editEntry.id,text:'Edited follow-up'})).ok,true);
   const edited=(await sendReopened('edited-state','state')).value;
   assert.deepEqual(edited.messages.filter(message=>message.role==='user').map(message=>message.content),['Original request','follow-one','Edited follow-up']);
  }finally{if(reopened.exitCode===null&&reopened.signalCode===null){const exited=once(reopened,'exit');reopened.kill();await exited;}}
 }finally{release?.();releaseFollowup?.();if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});

// A tool boundary distinguishes steer from followUp: both remain ordinary
// durable inputs, but the correction must reach request #2 before the queue.
test('same-turn insertion overtakes the queue without swapping identical-text receipts',{timeout:25000},async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'steer-'));
 const cwd=path.join(root,'workspace');await fs.mkdir(cwd);await fs.writeFile(path.join(cwd,'proof.txt'),'READ_PROOF');
 const bodies=[],events=[],pending=new Map();let releaseFirst,releaseSecond,buffer='',ready=false,stderr='';
 const server=http.createServer(async(req,res)=>{
  let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);bodies.push(body);const call=bodies.length;
  if(call===1)await new Promise(resolve=>releaseFirst=resolve);if(call===2)await new Promise(resolve=>releaseSecond=resolve);
  res.writeHead(200,{'Content-Type':'text/event-stream'});
  const chunk=(delta,finish_reason=null)=>res.write('data: '+JSON.stringify({id:'steer-fixture',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason}]})+'\n\n');
  if(call===1){chunk({tool_calls:[{index:0,id:'read-proof',type:'function',function:{name:'read',arguments:JSON.stringify({path:'proof.txt'})}}]});chunk({},'tool_calls');}
  else{chunk({content:call===2?'Inserted reply':'Queued reply'});chunk({},'stop');}res.end('data: [DONE]\n\n');
 });await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const child=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const end=buffer.indexOf('\n');if(end<0)break;const value=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
 const until=async check=>{const deadline=Date.now()+12000;while(!check()){if(Date.now()>deadline)throw Error('Steer fixture timeout: '+stderr);await new Promise(resolve=>setTimeout(resolve,10));}};
 const send=(id,method,params={})=>new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+'\n');});
 try{
  await until(()=>ready);assert.equal((await send('init','init',{cwd,sessionDirectory:path.join(root,'sessions'),agentDir:path.join(root,'agent'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-session-token',model:{id:'fixture',name:'Fixture'},permission:'ask',runtimeSettings:{autoRetry:false,backgroundCompaction:false}})).ok,true);
  assert.equal((await send('original','prompt',{text:'Read the proof'})).ok,true);await until(()=>bodies.length===1);
  assert.equal((await send('queued','prompt',{text:'Same message',deliveryMode:'queue'})).ok,true);
  assert.equal((await send('inserted','prompt',{text:'Same message',deliveryMode:'steer'})).ok,true);
  releaseFirst();await until(()=>bodies.length===2&&events.some(event=>event.type==='input_saved'&&event.id==='inserted'));
  assert.equal(bodies[1].messages.filter(message=>message.role==='user').length,2,'only steer may enter at a tool boundary');
  assert.deepEqual(events.filter(event=>event.type==='input_saved').map(event=>event.id),['original','inserted'],'identical content must not exchange receipt IDs');
  assert.equal((await send('status-insert','input_status',{id:'inserted',text:'Same message',deliveryMode:'steer'})).value.saved,true);
  releaseSecond();await until(()=>events.some(event=>event.type==='settled'));
  const state=(await send('state','state')).value;
  assert.deepEqual(state.messages.filter(message=>message.role==='user').map(message=>message.requestId),['original','inserted','queued']);
  assert.equal(bodies.length,3,'steer must not abort and restart the unfinished request');
  const rendered=events.reduce((state,event)=>applyChatEvent(state,event),emptyChatState());
  assert.deepEqual(rendered.messages.filter(message=>message.role==='user').map(message=>message.requestId),['original','inserted','queued']);
 }finally{releaseFirst?.();releaseSecond?.();if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
