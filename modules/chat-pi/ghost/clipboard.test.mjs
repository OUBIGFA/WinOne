// The original LibrarySlot's byte/limit rules through WinOne's real clipboard
// adapter. Native OS writes are independently checked by the UI; this fixture
// controls only that final acknowledgement, never forges an earlier success.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createGhostLibrary} from './platform-library.mjs';
import sharp from 'sharp';

const repository=fileURLToPath(new URL('../../../',import.meta.url));
async function fixture(t,host){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(repository,'_temp/plugin-task-close/clipboard-tests');
 await fs.mkdir(base,{recursive:true});const root=await fs.mkdtemp(path.join(base,'library-'));
 const ghost={enabled:true,approval:{state:'approved',revision:'clipboard-fixture'},manifest:{id:'clipboard-fixture',name:'Clipboard fixture',library:true}};
 const library=createGhostLibrary({root,find:()=>ghost,current:()=>true,sendToGhost:()=>true,showSave:async()=>null,host,log:{warn(){}},mediaStore:{},stopNode:async()=>{}});
 t.after(()=>library.close());return {root,library};
}

test('Library clipboard PNG keeps large bytes off the bounded wire and waits for the actual native acknowledgement',async t=>{
 let resolve,requested,settled=false;
 const observed=new Promise(done=>{requested=done;});
 const copied=new Promise(done=>{resolve=done;});
 const f=await fixture(t,async(method,input)=>{
  assert.equal(method,'clipboard-png');assert.equal(input.data,undefined);
  assert.equal(path.dirname(input.path),path.join(f.root,'clipboard-staging'));
  assert.ok(Buffer.byteLength(JSON.stringify(input))<1024,'only private metadata belongs in the native protocol');
  requested(input.path);return copied;
 });
 // Use the shipped image implementation to produce real PNG chunks/CRCs.
 // Level zero keeps a plain bitmap above 1 MB without malformed filler bytes.
 const bytes=await sharp({create:{width:1024,height:512,channels:4,background:{r:17,g:17,b:17,alpha:1}}}).png({compressionLevel:0}).toBuffer();
 assert.ok(bytes.length>1024*1024);
 const work=f.library.request('clipboard-fixture',{op:'clipboardWrite',encoding:'base64',content:bytes.toString('base64')}).then(result=>{settled=true;return result;});
 const file=await Promise.race([observed,work.then(result=>{throw Error('Clipboard callback was not reached: '+JSON.stringify(result));})]);
 assert.deepEqual(await fs.readFile(file),bytes);
 assert.equal(settled,false,'emitting a path must not confirm clipboard success');
 resolve({copied:true});const result=await work;
 assert.equal(result.ok,true,JSON.stringify(result));
 await assert.rejects(fs.stat(file),{code:'ENOENT'});
});

test('Library clipboard rejects a failed native receipt and retires only its own temporary PNG',async t=>{
 let staged;
 const f=await fixture(t,async(method,input)=>{assert.equal(method,'clipboard-png');staged=input.path;return {copied:false,message:'owned clipboard fixture is locked'};});
 const keep=path.join(f.root,'kept.txt');await fs.writeFile(keep,'not a clipboard temporary file');
 const bytes=await sharp({create:{width:1,height:1,channels:4,background:{r:17,g:17,b:17,alpha:1}}}).png().toBuffer();
 const result=await f.library.request('clipboard-fixture',{op:'clipboardWrite',encoding:'base64',content:bytes.toString('base64')});
 assert.equal(result.ok,false,JSON.stringify(result));
 await assert.rejects(fs.stat(staged),{code:'ENOENT'});
 assert.equal(await fs.readFile(keep,'utf8'),'not a clipboard temporary file');
});
