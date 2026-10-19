import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

// WinOne's production worker must run the durable runtime and expose the
// input-box controls: thinking level, fast mode, usage events and subagent
// control all reach the real Pi execution path.
const LF=String.fromCharCode(10);
const SSE=['data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"Fixture reasoning"},"finish_reason":null}]}','','data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Done"},"finish_reason":null}]}','','data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}','','data: [DONE]','',''].join(LF);
test('worker runs the durable runtime and forwards thinking, fast and usage state',{timeout:25000},async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'controls-'));
 const workspace=path.join(root,'workspace'),sessions=path.join(root,'sessions');await fs.mkdir(workspace);
 const bodies=[];
 const server=http.createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=chunk;bodies.push(JSON.parse(raw));res.writeHead(200,{'Content-Type':'text/event-stream'});res.end(SSE);});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const child=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 let buffer='',ready=false,stderr='';const pending=new Map(),events=[];
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const i=buffer.indexOf(LF);if(i<0)break;const value=JSON.parse(buffer.slice(0,i));buffer=buffer.slice(i+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
 const until=async predicate=>{const end=Date.now()+15000;while(!predicate()){if(Date.now()>end)throw Error('Worker timeout: '+stderr);await new Promise(resolve=>setTimeout(resolve,10));}};
 const send=(id,method,params={})=>new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+LF);});
 try{
  await until(()=>ready);
  const init=await send('init','init',{cwd:workspace,sessionDirectory:sessions,agentDir:path.join(root,'agent'),baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-controls-token',model:{id:'fixture',name:'Fixture'},permission:'ask'});
  assert.equal(init.ok,true,init.error);assert.equal(init.value.runtime,'pi-durable');
  const thinking=await send('thinking','thinking',{value:'high'});assert.equal(thinking.ok,true,thinking.error);assert.equal(thinking.value.thinkingLevel,'high');
  const invalid=await send('thinking-bad','thinking',{value:'ultra'});assert.equal(invalid.ok,false);assert.match(invalid.error,/thinking level/i);
  const fast=await send('fast','fast',{value:true});assert.equal(fast.ok,true,fast.error);assert.equal(fast.value.fastMode,true);
  assert.equal((await send('ask','prompt',{text:'Configured request'})).ok,true);
  await until(()=>events.some(event=>event.type==='settled'));
  assert.equal(bodies.at(-1).reasoning_effort,'high');assert.equal(bodies.at(-1).service_tier,'priority');
  assert.ok(events.some(event=>event.type==='usage'&&event.contextUsage?.tokens>0),'durable usage events must reach the host');
  const agents=await send('agents','control',{action:'list'});assert.equal(agents.ok,true,agents.error);assert.deepEqual(agents.value,[]);
  const state=(await send('state','state')).value;assert.equal(state.thinkingLevel,'high');assert.equal(state.fastMode,true);
  assert.ok(JSON.stringify(state.messages).includes('thinkingDurationMs'),'instant answers must still persist thinking timing');
  assert.ok((await fs.readFile(state.file,'utf8')).includes('winone.thinking'),'thinking timing must persist with the saved history');
  assert.equal((await send('close','close')).ok,true);
 }finally{child.kill();await new Promise(resolve=>server.close(resolve));}
});
