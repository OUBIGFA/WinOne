import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

// Exercise the process boundary and durable storage, including a complete
// Worker restart. A paused unsafe command must never be executed twice.
for(const codeMode of [false,true])test('worker pause survives process restart and resumes only on explicit request'+(codeMode?' through a nested Code Mode command':''),{timeout:30000},async t=>{
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.resolve('_temp/chat-pause-tests');await fs.mkdir(base,{recursive:true});
 const directory=await fs.mkdtemp(path.join(base,'pause-')),cwd=path.join(directory,'workspace');await fs.mkdir(cwd);
 const requests=[],LF=String.fromCharCode(10);
 const server=http.createServer(async(req,res)=>{
  let raw='';for await(const chunk of req)raw+=chunk;requests.push(JSON.parse(raw));
  const command={command:"Add-Content -LiteralPath 'once.txt' -Value 'ONCE'; Start-Sleep -Seconds 60"};
  const operation=codeMode?{name:'codemode',arguments:JSON.stringify({code:'await tools.powershell('+JSON.stringify(command)+'); text("Nested command returned");'})}:{name:'powershell',arguments:JSON.stringify(command)};
  const delta=requests.length===1?{tool_calls:[{index:0,id:'once',type:'function',function:operation}]}:{content:'Recovered without repeating the command.'};
  const chunks=[{id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason:null}]},{id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta:{},finish_reason:requests.length===1?'tool_calls':'stop'}]}];
  res.writeHead(200,{'Content-Type':'text/event-stream'});res.end(chunks.map(value=>'data: '+JSON.stringify(value)+LF+LF).join('')+'data: [DONE]'+LF+LF);
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));t.after(()=>new Promise(resolve=>server.close(resolve)));
 const config={cwd,sessionDirectory:path.join(directory,'sessions'),agentDir:path.join(directory,'agent'),baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-pause-key',model:{id:'fixture',name:'Fixture'},permission:'ask'};
 let sequence=0;
 async function open(sessionFile){
  const child=spawn(process.env.WINONE_CHAT_BUN_EXE??process.execPath,[process.env.WINONE_CHAT_WORKER??fileURLToPath(new URL('./worker.mjs',import.meta.url))],{cwd,stdio:['pipe','pipe','pipe'],windowsHide:true});
  t.after(()=>child.kill());let buffer='',ready=false,stderr='';const pending=new Map(),events=[];
  child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const at=buffer.indexOf(LF);if(at<0)break;const value=JSON.parse(buffer.slice(0,at));buffer=buffer.slice(at+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
  async function until(predicate){const end=Date.now()+10000;while(!await predicate()){if(Date.now()>end)throw Error('Pause boundary timeout: '+stderr+JSON.stringify(events.slice(-3)));await new Promise(resolve=>setTimeout(resolve,10));}}
  const send=(method,params={})=>new Promise(resolve=>{const id=String(++sequence);pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+LF);});
  await until(()=>ready);const init=await send('init',{...config,...(sessionFile?{sessionFile}:{})});assert.equal(init.ok,true,init.error);
  return {send,until,events,child,initial:init.value};
 }
 let worker=await open();assert.equal((await worker.send('thinking',{value:'high'})).ok,true);assert.equal((await worker.send('prompt',{text:'Run the command once.'})).ok,true);
 await worker.until(()=>worker.events.some(value=>value.type==='approval'));
 const approval=worker.events.find(value=>value.type==='approval');assert.equal((await worker.send('permission',{id:approval.request.id,allow:true})).ok,true);
 await worker.until(async()=>{try{return (await fs.readFile(path.join(cwd,'once.txt'),'utf8')).includes('ONCE');}catch{return false;}});
 assert.equal((await worker.send('prompt',{text:'Keep this accepted follow-up.'})).ok,true);
 const pause=await worker.send('pause');assert.equal(pause.ok,true,pause.error);assert.equal(pause.value.paused,true);assert.equal(pause.value.streaming,false);
 const file=pause.value.file;
 const tasks=await worker.send('control',{action:'tasks'});assert.equal(tasks.ok,true,tasks.error);assert.equal(tasks.value.scheduling,'paused');
 assert.ok(pause.value.taskGraph.tasks.length>0);assert.ok(!JSON.stringify(pause.value.taskGraph).includes('Add-Content'));
 assert.ok(!worker.events.some(value=>value.type==='settled'&&value.status==='failed'),'pausing is not a failed prompt');
 for(const method of ['prompt','compact','thinking','retry'])assert.equal((await worker.send(method,{text:'Must wait',value:'high'})).ok,false,method+' must wait for resume');
 assert.equal((await worker.send('close')).ok,true);await worker.until(()=>worker.child.exitCode!==null);
 worker=await open(file);assert.equal(worker.initial.paused,true);assert.equal(worker.initial.streaming,false);
 await new Promise(resolve=>setTimeout(resolve,100));assert.equal(requests.length,1,'opening paused history must not send a model request');
 const resume=await worker.send('resume');assert.equal(resume.ok,true,resume.error);assert.equal(resume.value.paused,false);
 await worker.until(()=>requests.length>=2&&worker.events.some(value=>value.type==='settled'&&value.status==='completed'));
 assert.ok(!worker.events.some(value=>value.type==='approval'),'unsafe command must not request execution again');
 assert.equal((await fs.readFile(path.join(cwd,'once.txt'),'utf8')).match(/ONCE/g).length,1);
 assert.ok(requests.flatMap(request=>request.messages).some(message=>message.role==='user'&&JSON.stringify(message.content).includes('Keep this accepted follow-up.')),'accepted follow-up must survive pause and restart');
 // An idle pause resumes in the same process through a newly opened harness.
 assert.equal((await worker.send('thinking',{value:'low'})).ok,true);
 assert.equal((await worker.send('fast',{value:true})).ok,true);
 assert.equal((await worker.send('pause')).ok,true);
 const again=await worker.send('resume');assert.equal(again.ok,true,again.error);assert.equal(again.value.thinkingLevel,'low');assert.equal(again.value.fastMode,true);
 const before=requests.length;assert.equal((await worker.send('prompt',{text:'Continue normally.'})).ok,true);
 await worker.until(()=>requests.length>before&&worker.events.filter(value=>value.type==='settled'&&value.status==='completed').length>=2);
 assert.equal(requests.at(-1).reasoning_effort,'low');assert.equal(requests.at(-1).service_tier,'priority');
 assert.equal((await worker.send('close')).ok,true);await worker.until(()=>worker.child.exitCode!==null);assert.equal(worker.child.exitCode,0);
});
