// Actual Bun HTTP streams and the official Pi Harness, not a mocked retry loop.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';
import {applyChatEvent,emptyChatState} from '../../apps/desktop/frontend/src/features/chat-state.ts';

async function workerFixture(t,respond){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.resolve('_temp/chat-cindy-fixes/worker-tests');await fs.mkdir(base,{recursive:true});
 const root=await fs.mkdtemp(path.join(base,'transport-')),cwd=path.join(root,'workspace');await fs.mkdir(cwd);
 const requests=[],events=[];let ready=false,buffer='',stderr='';const pending=new Map();
 const server=http.createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);requests.push(body);await respond(body,res,requests.length);});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const child=spawn(process.env.WINONE_CHAT_BUN_EXE??process.execPath,[process.env.WINONE_CHAT_WORKER??fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const end=buffer.indexOf('\n');if(end<0)break;const value=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
 t.after(async()=>{if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}server.closeAllConnections();await new Promise(resolve=>server.close(resolve));});
 const until=async check=>{const deadline=Date.now()+25000;while(!check()){if(Date.now()>deadline)throw Error('Worker transport fixture timed out: '+stderr);await new Promise(resolve=>setTimeout(resolve,10));}};
 const send=(id,method,params={})=>new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+'\n');});
 await until(()=>ready);
 return {cwd,root,events,requests,send,until,config:{cwd,agentDir:path.join(root,'agent'),sessionDirectory:path.join(root,'sessions'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-cindy-transport-token',model:{id:'fixture',name:'Fixture'},permission:'bypassPermissions',runtimeSettings:{backgroundCompaction:false}}};
}
function chunk(res,delta,finish_reason=null){res.write('data: '+JSON.stringify({id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason}]})+'\n\n');}
function answer(res,text){res.writeHead(200,{'Content-Type':'text/event-stream'});chunk(res,{content:text});chunk(res,{},'stop');res.end('data: [DONE]\n\n');}

test('Cindy retry budget recovers repeated socket drops after a tool without replay or manual continue',{timeout:35000},async t=>{
 const f=await workerFixture(t,async(_body,res,index)=>{
  if(index===1){res.writeHead(200,{'Content-Type':'text/event-stream'});chunk(res,{tool_calls:[{index:0,id:'once',type:'function',function:{name:'powershell',arguments:JSON.stringify({command:"Add-Content -LiteralPath 'once.txt' -Value 'ONCE'"})}}]});chunk(res,{},'tool_calls');res.end('data: [DONE]\n\n');return;}
  if(index<=4){res.writeHead(200,{'Content-Type':'text/event-stream'});chunk(res,{reasoning_content:'Connection interrupted before completion'});await new Promise(resolve=>setTimeout(resolve,150));res.destroy();return;}
  answer(res,'RECOVERED_WITHOUT_CONTINUE');
 });
 assert.equal((await f.send('init','init',f.config)).ok,true);assert.equal((await f.send('original','prompt',{text:'Run the command once and finish.'})).ok,true);
 await f.until(()=>f.events.some(event=>event.type==='settled'));
 const settled=f.events.filter(event=>event.type==='settled');assert.equal(settled.length,1);assert.equal(settled[0].status,'completed',JSON.stringify(settled[0]));
 assert.equal(f.requests.length,5,'three transient drops need more than the old two retries');
 assert.equal((await fs.readFile(path.join(f.cwd,'once.txt'),'utf8')).match(/ONCE/g)?.length,1);
 const state=f.events.reduce(applyChatEvent,emptyChatState());assert.equal(state.messages.filter(message=>message.role==='user').length,1);
 assert.match(JSON.stringify(state.messages.at(-1)),/RECOVERED_WITHOUT_CONTINUE/);
 assert.ok(f.events.some(event=>event.type==='operation'&&event.operation.kind==='retry'));
});

test('Stop during native retry backoff never schedules another model request',{timeout:15000},async t=>{
 const f=await workerFixture(t,async(_body,res)=>{res.writeHead(200,{'Content-Type':'text/event-stream'});chunk(res,{content:'Partial'});await new Promise(resolve=>setTimeout(resolve,150));res.destroy();});
 assert.equal((await f.send('init','init',f.config)).ok,true);await f.send('original','prompt',{text:'Stop on the reconnect interval.'});
 await f.until(()=>f.events.some(event=>event.type==='operation'&&event.operation.kind==='retry'));
 await f.send('stop','abort');await f.until(()=>f.events.some(event=>event.type==='settled'));
 await new Promise(resolve=>setTimeout(resolve,2200));assert.equal(f.requests.length,1);assert.equal(f.events.findLast(event=>event.type==='settled').status,'stopped');
});
