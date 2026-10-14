import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';
import {SessionManager} from '@earendil-works/pi-coding-agent';

// Inspect the real worker's persisted projection: tool chatter must not spend
// the user's conversation-history budget or split ordinary turns across pages.
test('worker history pages by complete user turns while bounding its wire payload',{timeout:15000},async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'history-'));
 const cwd=path.join(root,'workspace'),directory=path.join(root,'sessions');await fs.mkdir(cwd);await fs.mkdir(directory);
 const manager=SessionManager.create(cwd,directory);
 const appendTurn=(name,count,text='tool output')=>{
  manager.appendMessage({role:'user',content:name,timestamp:Date.now()});
  for(let index=0;index<count;index++){
   manager.appendMessage({role:'assistant',content:[{type:'toolCall',id:`${name}-${index}`,name:'read',arguments:{path:'fixture.txt'}}],timestamp:Date.now(),api:'openai-completions',provider:'fixture',model:'fixture',stopReason:'toolUse',usage:{input:1,output:1,cacheRead:0,cacheWrite:0,totalTokens:2,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}}});
   manager.appendMessage({role:'toolResult',toolCallId:`${name}-${index}`,toolName:'read',content:[{type:'text',text}],isError:false,timestamp:Date.now()});
  }
  manager.appendMessage({role:'assistant',content:[{type:'text',text:'Done'}],timestamp:Date.now(),api:'openai-completions',provider:'fixture',model:'fixture',stopReason:'stop',usage:{input:1,output:1,cacheRead:0,cacheWrite:0,totalTokens:2,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}}});
 };
 for(const name of ['one','two','three','four'])appendTurn(name,20);
 const child=spawn(process.execPath,[fileURLToPath(new URL('./worker.mjs',import.meta.url))],{stdio:['pipe','pipe','pipe'],windowsHide:true});
 let buffer='',ready=false,stderr='';const pending=new Map();let sequence=0;
 child.stderr.on('data',chunk=>stderr+=chunk);child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const end=buffer.indexOf('\n');if(end<0)break;const value=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}}});
 const inspect=async(params={})=>{const id=`inspect-${sequence++}`;const response=await new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method:'inspect',params:{file:manager.getSessionFile(),cwd,directory,...params}})+'\n');});assert.equal(response.ok,true,response.error);return response.value;};
 try{
  const deadline=Date.now()+10000;while(!ready){if(Date.now()>deadline)throw Error(stderr||'Worker did not start');await new Promise(resolve=>setTimeout(resolve,10));}
  const page=await inspect();assert.equal(page.hasOlder,false);assert.equal(page.messages.length,168);assert.equal(page.messages[0].content,'one');
  const recent=await inspect({limit:2});assert.deepEqual(recent.messages.filter(message=>message.role==='user').map(message=>message.content),['three','four']);assert.equal(recent.messages.length,84);assert.equal(recent.hasOlder,true);
  const older=await inspect({before:recent.before,limit:2});assert.deepEqual(older.messages.filter(message=>message.role==='user').map(message=>message.content),['one','two']);assert.equal(older.hasOlder,false);
  appendTurn('large',5,'中'.repeat(15000));
  const bounded=await inspect();assert.ok(Buffer.byteLength(JSON.stringify(bounded))<800*1024);assert.equal(bounded.messages[0].role,'user');assert.equal(bounded.messages.at(-1).role,'assistant');
 }finally{if(child.exitCode===null&&child.signalCode===null){const exited=once(child,'exit');child.kill();await exited;}}
});
