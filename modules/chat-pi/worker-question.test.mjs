import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

// Exercise the production JSONL boundary, including failed validation,
// cancellation and a stopped run. A default option is never an answer.
test('worker awaits actual question answers and clears cancelled requests',{timeout:45000},async t=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'questions-'));
 const cwd=path.join(root,'workspace');await fs.mkdir(cwd);
 const requests=[];let calls=0;
 const LF=String.fromCharCode(10);
 const server=http.createServer(async(req,res)=>{
  let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);requests.push(body);
  const answered=body.messages.at(-1)?.role==='tool';
  const delta=answered?{content:'Answer received'}:{tool_calls:[{index:0,id:'question-'+(++calls),type:'function',function:{name:'ask_user_question',arguments:JSON.stringify({questions:[{question:'Which direction?',options:['A','B']}]})}}]};
  const chunks=[{id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason:null}]},{id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta:{},finish_reason:answered?'stop':'tool_calls'}]}];
  res.writeHead(200,{'Content-Type':'text/event-stream'});res.end(chunks.map(value=>'data: '+JSON.stringify(value)+LF+LF).join('')+'data: [DONE]'+LF+LF);
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const child=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 let buffer='',ready=false,stderr='';const pending=new Map(),events=[];
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const at=buffer.indexOf(LF);if(at<0)break;const value=JSON.parse(buffer.slice(0,at));buffer=buffer.slice(at+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
 const until=async predicate=>{const end=Date.now()+10000;while(!predicate()){if(Date.now()>end)throw Error('Question boundary timeout: '+stderr+JSON.stringify(events.slice(-3)));await new Promise(resolve=>setTimeout(resolve,10));}};
 const send=(id,method,params={})=>new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+LF);});
 t.after(async()=>{child.kill();await new Promise(resolve=>server.close(resolve));});
 await until(()=>ready);
 const initialized=await send('init','init',{cwd,sessionDirectory:path.join(root,'sessions'),agentDir:path.join(root,'agent'),baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-question-session-key',model:{id:'fixture',name:'Fixture'},permission:'ask'});
 assert.equal(initialized.ok,true,initialized.error);
 assert.equal((await send('prompt','prompt',{text:'Ask me a question'})).ok,true);
 await until(()=>events.some(value=>value.type==='approval'&&value.request.kind==='question'));
 const request=events.find(value=>value.type==='approval'&&value.request.kind==='question').request;
 assert.equal(request.questions[0].question,'Which direction?');assert.equal(requests.length,1);
 const invalid=await send('invalid','question',{id:request.id,answers:[]});assert.equal(invalid.ok,false);assert.equal(requests.length,1);
 assert.equal((await send('answer','question',{id:request.id,answers:['B']})).ok,true);
 await until(()=>events.some(value=>value.type==='settled'));
 assert.ok(requests.at(-1).messages.some(value=>value.role==='tool'&&value.content.includes('"answer":"B"')));
 assert.equal((await send('duplicate','question',{id:request.id,answers:['A']})).ok,false);
 const before=events.length;assert.equal((await send('prompt2','prompt',{text:'Ask again'})).ok,true);
 await until(()=>events.slice(before).some(value=>value.type==='approval'&&value.request.kind==='question'));
 const cancelled=events.slice(before).find(value=>value.type==='approval').request;
 assert.equal((await send('cancel','question',{id:cancelled.id,cancel:true})).ok,true);
 await until(()=>events.slice(before).some(value=>value.type==='settled'));
 assert.ok(events.slice(before).some(value=>value.type==='approval_cancelled'&&value.id===cancelled.id));
 const afterCancel=events.length;assert.equal((await send('prompt3','prompt',{text:'Wait for me'})).ok,true);
 await until(()=>events.slice(afterCancel).some(value=>value.type==='approval'&&value.request.kind==='question'));
 const stopped=events.slice(afterCancel).find(value=>value.type==='approval').request;
 assert.equal((await send('stop','abort')).ok,true);
 assert.equal((await send('late','question',{id:stopped.id,answers:['A']})).ok,false);
 assert.equal((await send('close','close')).ok,true);
});
