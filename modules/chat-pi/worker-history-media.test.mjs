import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';
import {SessionManager} from '@earendil-works/pi-coding-agent';

// Read a real saved Pi conversation. Older media must remain lazy until its
// history page is requested; opening recent text must not decode every image.
test('opening saved history materializes only the requested page of media',{timeout:60000},async t=>{
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.resolve('_temp/chat-history-media');
 await fs.mkdir(base,{recursive:true});
 const root=await fs.mkdtemp(path.join(base,'media-'));
 const cwd=path.join(root,'workspace'),directory=path.join(root,'sessions');
 await fs.mkdir(cwd);await fs.mkdir(directory);
 const manager=SessionManager.create(cwd,directory);
 const usage={input:1,output:1,cacheRead:0,cacheWrite:0,totalTokens:2,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}};
 for(let n=0;n<120;n++){
  manager.appendMessage({role:'user',content:'Saved question '+n,timestamp:n*4+1});
  manager.appendMessage({role:'assistant',content:[{type:'toolCall',id:'image-'+n,name:'read',arguments:{path:'image.png'}}],timestamp:n*4+2,api:'openai-completions',provider:'fixture',model:'fixture',stopReason:'toolUse',usage});
  manager.appendMessage({role:'toolResult',toolCallId:'image-'+n,toolName:'read',content:[{type:'image',mimeType:'image/png',data:Buffer.alloc(100*1024,n).toString('base64')}],timestamp:n*4+3,isError:false});
  manager.appendMessage({role:'assistant',content:[{type:'text',text:'Saved answer '+n}],timestamp:n*4+4,api:'openai-completions',provider:'fixture',model:'fixture',stopReason:'stop',usage});
 }
 const started=performance.now();
 const child=spawn(process.env.WINONE_CHAT_BUN_EXE??process.execPath,[process.env.WINONE_CHAT_WORKER??fileURLToPath(new URL('./worker.mjs',import.meta.url))],{cwd,stdio:['pipe','pipe','pipe'],windowsHide:true});
 let buffer='',ready=false,stderr='',seq=0;const pending=new Map();
 child.stderr.on('data',chunk=>stderr+=chunk);
 child.stdout.on('data',chunk=>{buffer+=chunk;for(;;){const i=buffer.indexOf('\n');if(i<0)break;const value=JSON.parse(buffer.slice(0,i));buffer=buffer.slice(i+1);if(value.type==='ready')ready=true;else if(value.type==='response'){pending.get(value.id)?.(value);pending.delete(value.id);}}});
 t.after(async()=>{if(child.exitCode===null&&child.signalCode===null){const ended=once(child,'exit');child.kill();await ended;}});
 const deadline=Date.now()+20000;while(!ready){if(Date.now()>deadline)throw Error(stderr||'Worker startup timed out');await new Promise(resolve=>setTimeout(resolve,10));}
 const readyMs=performance.now()-started;
 const call=async(method,params)=>{const id=String(++seq);const result=await new Promise(resolve=>{pending.set(id,resolve);child.stdin.write(JSON.stringify({id,method,params})+'\n');});assert.equal(result.ok,true,result.error);return result.value;};
 const opened=performance.now();
 const page=await call('init',{cwd,sessionDirectory:directory,sessionFile:manager.getSessionFile(),agentDir:path.join(root,'agent'),baseUrl:'http://127.0.0.1:9/v1',key:'winone-fixture-history-token',model:{id:'fixture',name:'Fixture'},permission:'ask'});
 const openMs=performance.now()-opened;
 const media=await fs.readdir(path.join(directory,'attachments','tool-media'));
 console.log(JSON.stringify({sample:'120 turns, 120 tool images × 100 KiB',readyMs:Math.round(readyMs),openMs:Math.round(openMs),mediaFiles:media.length,messages:page.messages.length}));
 assert.equal(page.messages.filter(m=>m.role==='user').length,30);
 assert.equal(media.length,30,'unshown images were decoded while loading the latest page');
 const older=await call('state',{before:page.before,limit:30});
 assert.equal(older.messages[0].content,'Saved question 60');
 assert.equal((await fs.readdir(path.join(directory,'attachments','tool-media'))).length,60);
 assert.ok(page.messages.filter(m=>m.role==='toolResult').every(m=>m.content[0].path));
});
