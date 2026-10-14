import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';

test('aborting image preflight cannot start the cancelled prompt or clear a later user turn',{timeout:20000},async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'preflight-cancel-')),workspace=path.join(root,'workspace'),sessions=path.join(root,'sessions');
 const image=path.join(sessions,'attachments','one','blocked.png');await fs.mkdir(path.dirname(image),{recursive:true});await fs.mkdir(workspace);await fs.copyFile(new URL('../../internal/branding/chat-welcome.png',import.meta.url),image);
 let requests=0,release;const server=http.createServer(async(req,res)=>{for await(const ignored of req){}requests++;await new Promise(resolve=>{release=resolve;});res.writeHead(200,{'Content-Type':'text/event-stream'});res.end('data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Done"},"finish_reason":null}]}\n\ndata: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n');});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const source=`import fileIO from 'node:fs/promises';const original=fileIO.readFile;fileIO.readFile=async(...args)=>{if(String(args[0]).endsWith('blocked.png')){console.error('PREFLIGHT_STARTED');await new Promise(resolve=>setTimeout(resolve,500));}return original(...args);};await import(${JSON.stringify(new URL('./worker.mjs',import.meta.url).href)});`;
 const child=spawn(process.execPath,['--eval',source],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 let buffer='',ready=false,preparing=false;const replies=new Map(),events=[];
 child.stdout.on('data',chunk=>{buffer+=chunk.toString();for(;;){const end=buffer.indexOf('\n');if(end<0)break;const message=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);if(message.type==='ready')ready=true;else if(message.type==='response'){replies.get(message.id)?.(message);replies.delete(message.id);}else events.push(message);}});
 child.stderr.on('data',chunk=>{if(chunk.toString().includes('PREFLIGHT_STARTED'))preparing=true;});
 const until=async(predicate)=>{const timeout=Date.now()+10000;while(!predicate()){if(Date.now()>timeout)throw Error('Timed out waiting for the worker');await new Promise(resolve=>setTimeout(resolve,10));}};
 const send=(id,method,params)=>new Promise(resolve=>{replies.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+'\n');});
 try{
  await until(()=>ready);assert.equal((await send('init','init',{cwd:workspace,sessionDirectory:sessions,agentDir:path.join(root,'agent'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-session-token',model:{id:'fixture',name:'Fixture',vision:true},permission:'ask'})).ok,true);
  const cancelled=send('cancelled','prompt',{text:'Old image request',attachments:[image]});await until(()=>preparing);assert.equal((await send('abort','abort')).ok,true);
  assert.equal((await send('new-user','prompt',{text:'New user request'})).ok,true);await until(()=>requests===1);
  const old=await cancelled;assert.equal(old.ok,false,'the cancelled preflight must never be acknowledged as accepted');
  const state=await send('state','state');assert.equal(state.value.streaming,true,'the old preflight cannot clear the newer running turn');assert.equal(requests,1);
  release();await until(()=>events.some(event=>event.type==='settled'&&event.status==='completed'));
 }finally{release?.();if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
