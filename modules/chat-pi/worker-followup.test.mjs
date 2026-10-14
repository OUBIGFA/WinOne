import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';

// A local SSE provider exercises the actual Pi loop and persisted receipts.
test('Pi consumes multiple follow-ups once and retries saved history without duplicating its user message',{timeout:25000},async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'followup-'));
 const workspace=path.join(root,'workspace'),sessions=path.join(root,'sessions');await fs.mkdir(workspace);
 let calls=0,release;const bodies=[];
 const server=http.createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);bodies.push(body);calls++;if(calls===1)await new Promise(resolve=>release=resolve);res.writeHead(200,{'Content-Type':'text/event-stream'});res.end('data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Done"},"finish_reason":null}]}\n\ndata: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n');});
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
  release();await until(()=>events.some(event=>event.type==='settled'));
  for(const id of ['original','follow-one','follow-two']){
   const liveRows=events.filter(event=>event.type==='pi_event'&&event.message?.role==='user'&&event.message?.turnId===id);
   assert.equal(liveRows.length,2,'each input emits start and end');
   assert.ok(liveRows.every(event=>event.message.requestId===id),'live messages must carry the same receipt as the optimistic and saved rows');
  }
  const state=(await send('state','state')).value;assert.deepEqual(state.messages.filter(message=>message.role==='user').map(message=>message.content),['Original request','follow-one','follow-two']);
  assert.ok(calls>=2);for(const id of ['follow-one','follow-two'])assert.equal((await send('status-'+id,'input_status',{id,text:id})).value.saved,true);
  const user=state.messages.findLast(message=>message.role==='user'),before=calls;const retiredAnswerCount=state.messages.filter(message=>message.role==='assistant').length;events.length=0;
  assert.equal((await send('retry-turn','retry',{entry:user.id})).ok,true);await until(()=>events.some(event=>event.type==='settled'));assert.equal(calls,before+1);
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
 }finally{release?.();if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
