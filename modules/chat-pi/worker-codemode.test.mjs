import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

// Run the shipped JSONL entry, not an import from the source tree: its separate
// VM entry and WASM must work with no node_modules beside the bundle.
test('bundled code mode runs its isolated VM and recovers conversation store',{timeout:30000},async t=>{
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.resolve('_temp/chat-codemode-tests');await fs.mkdir(base,{recursive:true});
 const directory=await fs.mkdtemp(path.join(base,'bundle-')),cwd=path.join(directory,'workspace');await fs.mkdir(cwd);
 await fs.writeFile(path.join(cwd,'proof.txt'),'BUNDLED_CODE_MODE_PROOF');
 const requests=[],replies=[
  {code:"text(await tools.read({path:'proof.txt'})); store('proof','RECOVERED_STORE');"},
  {text:'Read the file.'},{code:"text(load('proof'));"},{text:'Recovered the store.'},
 ];
 const LF=String.fromCharCode(10);
 const server=http.createServer(async(req,res)=>{
  let raw='';for await(const chunk of req)raw+=chunk;requests.push(JSON.parse(raw));
  const response=replies.shift();if(!response){res.writeHead(500);res.end('Unexpected request');return;}
  const delta=response.code?{tool_calls:[{index:0,id:'code-'+requests.length,type:'function',function:{name:'codemode',arguments:JSON.stringify({code:response.code})}}]}:{content:response.text};
  const chunks=[{id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason:null}]},{id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta:{},finish_reason:response.code?'tool_calls':'stop'}]}];
  res.writeHead(200,{'Content-Type':'text/event-stream'});res.end(chunks.map(value=>'data: '+JSON.stringify(value)+LF+LF).join('')+'data: [DONE]'+LF+LF);
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));t.after(()=>new Promise(resolve=>server.close(resolve)));
 const config={cwd,sessionDirectory:path.join(directory,'sessions'),agentDir:path.join(directory,'agent'),baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-code-mode-key',model:{id:'fixture',name:'Fixture'},permission:'ask'};
 let sequence=0;
 async function open(sessionFile){
  const child=spawn(process.env.WINONE_CHAT_BUN_EXE??process.execPath,[process.env.WINONE_CHAT_WORKER??fileURLToPath(new URL('./worker.mjs',import.meta.url))],{cwd,stdio:['pipe','pipe','pipe'],windowsHide:true});
  t.after(()=>child.kill());let buffer='',ready=false,stderr='';const pending=new Map(),events=[];
  child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const at=buffer.indexOf(LF);if(at<0)break;const value=JSON.parse(buffer.slice(0,at));buffer=buffer.slice(at+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}else events.push(value);}});
  async function until(predicate){const end=Date.now()+8000;while(!predicate()){if(Date.now()>end)throw Error('Code mode boundary timeout: '+stderr+JSON.stringify(events.slice(-3)));await new Promise(resolve=>setTimeout(resolve,10));}}
  const send=(method,params={})=>new Promise(resolve=>{const id=String(++sequence);pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+LF);});
  await until(()=>ready);const init=await send('init',{...config,...(sessionFile?{sessionFile}:{})});assert.equal(init.ok,true,init.error);
  return {send,until,events,child};
 }
 let worker=await open();assert.equal((await worker.send('prompt',{text:'Read and store through code mode.'})).ok,true);
 await worker.until(()=>requests.length>=2&&worker.events.some(value=>value.type==='settled'));
 assert.ok(requests.at(-1).messages.some(value=>value.role==='tool'&&value.content.includes('BUNDLED_CODE_MODE_PROOF')),JSON.stringify(requests.at(-1).messages));
 const state=await worker.send('state'),file=state.value.file;assert.ok(file);
 assert.equal((await worker.send('close')).ok,true);
 worker=await open(file);assert.equal((await worker.send('prompt',{text:'Recall the stored value.'})).ok,true);
 await worker.until(()=>requests.length>=4&&worker.events.some(value=>value.type==='settled'));
 assert.ok(requests.at(-1).messages.some(value=>value.role==='tool'&&value.content.includes('RECOVERED_STORE')),JSON.stringify(requests.at(-1).messages));
 assert.equal((await worker.send('close')).ok,true);
});
