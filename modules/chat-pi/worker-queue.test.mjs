const NL=String.fromCharCode(10);
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';

// Only the model transport is a fixture. Actions use the real durable inbox.
test('queued inputs can be held, edited, cancelled, reordered and steered',{timeout:30000},async()=>{
 const folder=process.env.WINONE_CHAT_TEST_DIRECTORY;assert.ok(folder,'Use an isolated test directory');
 const root=await fs.mkdtemp(path.join(folder,'queue-'));await fs.writeFile(path.join(root,'proof.txt'),'proof');
 const bodies=[],events=[],pending=new Map();let release,buffer='',ready=false,stderr='';
 const server=http.createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=chunk;bodies.push(JSON.parse(raw));const first=bodies.length===1;if(first)await new Promise(resolve=>release=resolve);
  res.writeHead(200,{'Content-Type':'text/event-stream'});const chunk=(delta,finish_reason=null)=>res.write('data: '+JSON.stringify({id:'queue',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason}]})+NL+NL);
  if(first){chunk({tool_calls:[{index:0,id:'read',type:'function',function:{name:'read',arguments:JSON.stringify({path:'proof.txt'})}}]});chunk({},'tool_calls');}else{chunk({content:'Done'});chunk({},'stop');}res.end('data: [DONE]'+NL+NL);
 });await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 let child;const launch=()=>{ready=false;buffer='';child=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const end=buffer.indexOf(NL);if(end<0)break;const value=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
 };launch();
 const until=async check=>{const end=Date.now()+12000;while(!check()){if(Date.now()>end)throw Error('Worker timeout: '+stderr);await new Promise(resolve=>setTimeout(resolve,10));}};
 let n=0;const send=(method,params={},id='command-'+(++n))=>new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+NL);});
 try{
  const config={cwd:root,sessionDirectory:path.join(root,'sessions'),agentDir:path.join(root,'agent'),baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-session-token',model:{id:'fixture',name:'Fixture'},permission:'ask',runtimeSettings:{autoRetry:false,backgroundCompaction:false}};
  await until(()=>ready);assert.equal((await send('init',config)).ok,true);
  await send('prompt',{text:'Read proof'},'original');await until(()=>bodies.length===1);
  for(const id of ['edit-me','cancel-me','steer-me','last'])assert.equal((await send('prompt',{text:id},id)).ok,true);
  const queue=async(action,id,extra={})=>{const result=await send('queue',{action,id,...extra});assert.equal(result.ok,true,JSON.stringify(result));return result.value;};
  await queue('pause');
  await queue('lock','edit-me',{locked:true});
  await queue('update','edit-me',{text:'Edited queued input',attachments:[]});
  await queue('cancel','cancel-me');
  await queue('reorder','last',{targetIndex:0});
  await queue('steer','steer-me');
  release();await until(()=>events.some(event=>event.type==='input_saved'&&event.id==='steer-me'));
  await until(()=>events.some(event=>event.type==='settled'));
  let state=(await send('state')).value;
  assert.equal(state.messages.find(message=>message.requestId==='edit-me')?.pending,true,'editing must prevent dispatch');
  assert.ok(!state.messages.some(message=>message.requestId==='cancel-me'));
  assert.equal(state.queuePaused,true,'pause state survives a settled snapshot');
  const priorCalls=bodies.length,sessionFile=state.file,closed=once(child,'exit');await send('close');await closed;
  launch();await until(()=>ready);const opened=await send('init',{...config,sessionFile});assert.equal(opened.ok,true,JSON.stringify(opened));
  state=(await send('state')).value;assert.equal(state.queuePaused,true);assert.equal(state.streaming,false,'reopening a paused queue must stay idle');
  assert.equal(bodies.length,priorCalls,'reopen must not dispatch the paused queue');

  await queue('lock','edit-me',{locked:false});await queue('resume');await until(()=>events.some(event=>event.type==='input_saved'&&event.id==='edit-me'));await until(()=>events.filter(event=>event.type==='settled').length>=2);
  state=(await send('state')).value;
  assert.deepEqual(state.messages.filter(message=>message.role==='user').map(message=>message.content),['Read proof','steer-me','last','Edited queued input']);
  assert.equal((await send('queue',{action:'cancel',id:'edit-me'})).ok,false,'consumed inputs cannot report a false cancellation');
  assert.ok(!bodies.some(body=>body.messages.some(message=>message.role==='user'&&JSON.stringify(message.content).includes('cancel-me'))));
 }finally{release?.();if(child.exitCode===null&&child.signalCode===null){const ended=once(child,'exit');child.kill();await ended;}server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
