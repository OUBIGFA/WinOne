import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {createHash} from 'node:crypto';
import JSZip from 'jszip';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {exportConversationArchive} from './archive.mjs';
import {openPayload} from './cindy/xdtshareCrypto.ts';
import {validateManifest} from './cindy/xdtshareFormat.pure.ts';

async function fixture(){
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'cshare-'));
 const cwd=path.join(root,'workspace'),directory=path.join(root,'sessions');await fs.mkdir(cwd);await fs.mkdir(directory);
 const files=path.join(directory,'attachments','original');await fs.mkdir(files,{recursive:true});
 const attachment=path.join(files,'proof.txt'),data=Buffer.from('COMPLETE_ATTACHMENT\n'.repeat(5000));await fs.writeFile(attachment,data);
 const manager=SessionManager.create(cwd,directory);
 manager.appendCustomEntry('winone.input',{id:'input-one',text:'Read the complete file',attachments:[{path:attachment,name:'proof.txt',size:data.length,mimeType:'text/plain'}]});
 const usage={input:1,output:1,cacheRead:0,cacheWrite:0,totalTokens:2,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}};
 for(let index=0;index<65;index++){
  manager.appendMessage({role:'user',content:index?'USER_'+index:'Read the complete file',timestamp:1000+index*2});
  manager.appendMessage({role:'assistant',content:[{type:'text',text:'ANSWER_'+index}],api:'openai-completions',provider:'winone',model:'fixture',usage,stopReason:'stop',timestamp:1001+index*2});
 }
 const conversation={id:'fixture-conversation',title:'Complete Pi task',projectId:'project',directory:cwd,sessionDirectory:directory,sessionFile:manager.getSessionFile(),hasMessages:true,permission:'ask',model:{provider:'fixture',model:'model'},createdAt:'2026-10-11T00:00:00Z',updatedAt:'2026-10-11T00:01:00Z'};
 return {root,manager,conversation,attachment,data,draft:{text:'UNSENT_DRAFT',attachments:[{path:attachment,name:'proof.txt',size:data.length,mimeType:'text/plain'}]},attachmentRoots:[files]};
}

async function unpack(target,password){
 const file=await fs.readFile(target),payload=openPayload(file,password),zip=await JSZip.loadAsync(payload.zipBytes);
 const manifest=validateManifest(JSON.parse(await zip.file('manifest.json').async('string')));
 for(const entry of manifest.entries){const bytes=await zip.file(entry.path).async('nodebuffer');assert.equal(bytes.length,entry.bytes);assert.equal(createHash('sha256').update(bytes).digest('hex'),entry.sha256);}
 return {file,zip,manifest};
}

test('Cindy archive contains all Pi messages, byte-identical native history, draft and full attachment',async()=>{
 const f=await fixture(),targetPath=path.join(f.root,'complete.cshare'),original=await fs.readFile(f.conversation.sessionFile);
 const outcome=await exportConversationArchive({...f,targetPath});assert.equal(outcome.status,'ok');assert.equal(outcome.fidelity,'full');assert.equal(outcome.messageCount,130);
 const {file,zip,manifest}=await unpack(targetPath);assert.equal(file.subarray(0,8).toString(),'XDTSHARE');assert.equal(manifest.agentKind,'pi');assert.equal(manifest.counts.messages,130);
 assert.equal(JSON.parse(await zip.file('session.json').async('string')).totalTokenUsage,130);
 const messages=(await zip.file('messages.jsonl').async('string')).split('\n').map(line=>JSON.parse(line));assert.match(messages[0].content,/Read the complete file/);assert.match(messages.at(-1).content,/ANSWER_64/);
 assert.equal(manifest.transcripts.length,1);assert.ok(manifest.activeSdkSessionId.startsWith('pi-'));assert.ok(!manifest.activeSdkSessionId.includes(f.root));
 assert.deepEqual(await zip.file(manifest.transcripts[0].path).async('nodebuffer'),original);
 const media=JSON.parse(await zip.file('media-map.json').async('string')).entries;assert.equal(media.length,1);assert.deepEqual(await zip.file(media[0].zipPath).async('nodebuffer'),f.data);
 assert.match(await zip.file('winone/draft.json').async('string'),/UNSENT_DRAFT/);
 assert.deepEqual(await fs.readFile(f.conversation.sessionFile),original);
});

test('original Cindy encryption opens with NFC-equivalent passwords and rejects wrong passwords or tampering',async()=>{
 const f=await fixture(),targetPath=path.join(f.root,'encrypted.cshare');
 await exportConversationArchive({...f,targetPath,password:'Cafe\u0301'});
 const {file,manifest}=await unpack(targetPath,'Café');assert.equal(manifest.counts.messages,130);assert.equal(file[9],1);
 assert.throws(()=>openPayload(file,'wrong'),error=>error.code==='SHARE_PASSWORD_WRONG');
 const tampered=Buffer.from(file);tampered[tampered.length-1]^=1;assert.throws(()=>openPayload(tampered,'Café'),error=>error.code==='SHARE_PASSWORD_WRONG');
});

test('the real 256 MiB preflight leaves the target untouched and excludes media only on explicit retry',async()=>{
 const f=await fixture(),targetPath=path.join(f.root,'oversize.cshare');
 const handle=await fs.open(f.attachment,'r+');try{await handle.truncate(257*1024*1024);}finally{await handle.close();}
 await fs.writeFile(targetPath,'OLD_EXPORT');
 const oversized=await exportConversationArchive({...f,targetPath});assert.equal(oversized.status,'oversize');assert.equal(oversized.limitBytes,256*1024*1024);assert.ok(oversized.mediaBytes>oversized.limitBytes);assert.equal(await fs.readFile(targetPath,'utf8'),'OLD_EXPORT');
 const retried=await exportConversationArchive({...f,targetPath,excludeMedia:true});assert.equal(retried.status,'ok');assert.equal(retried.fidelity,'partial');assert.equal(retried.mediaExcluded,1);
 const {manifest,zip}=await unpack(targetPath);assert.equal(manifest.counts.messages,130);assert.equal(manifest.counts.media,0);assert.equal(JSON.parse(await zip.file('media-map.json').async('string')).entries[0].zipPath,null);
});

test('missing or out-of-scope attachments are reported without exporting unrelated files',async()=>{
 const f=await fixture(),outside=path.join(f.root,'not-an-attachment.txt');await fs.writeFile(outside,'DO_NOT_EXPORT');
 f.draft.attachments.push({path:outside,name:'not-an-attachment.txt'},{path:path.join(path.dirname(f.attachment),'missing.txt'),name:'missing.txt'});
 const targetPath=path.join(f.root,'partial.cshare'),outcome=await exportConversationArchive({...f,targetPath});assert.equal(outcome.fidelity,'partial');assert.equal(outcome.mediaMissing,2);assert.equal(outcome.mediaDropped,1);
 const {zip}=await unpack(targetPath);assert.ok(!Object.keys(zip.files).some(name=>name.includes('not-an-attachment')));
});

test('inline images are portable media while a missing transcript is explicitly db-only',async()=>{
 const f=await fixture(),png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l5sAAAAASUVORK5CYII=','base64');
 f.manager.appendMessage({role:'user',content:[{type:'image',mimeType:'image/png',data:png.toString('base64')}],timestamp:9000});
 const targetPath=path.join(f.root,'images.cshare');await exportConversationArchive({...f,targetPath});
 const {zip,manifest}=await unpack(targetPath),media=JSON.parse(await zip.file('media-map.json').async('string')).entries;
 assert.equal(manifest.counts.media,2);const image=media.find(entry=>entry.kind==='image');assert.deepEqual(await zip.file(image.zipPath).async('nodebuffer'),png);
 const missing=await exportConversationArchive({...f,conversation:{...f.conversation,sessionFile:path.join(f.conversation.sessionDirectory,'missing.jsonl')},targetPath:path.join(f.root,'missing.cshare')});
 assert.equal(missing.fidelity,'db-only');assert.equal(missing.missingTranscripts.length,1);
});

test('cancelled collection and malformed native history cannot replace an existing archive',async()=>{
 const f=await fixture(),targetPath=path.join(f.root,'kept.cshare');await fs.writeFile(targetPath,'KEEP_ORIGINAL','utf8');
 const controller=new AbortController();controller.abort();await assert.rejects(exportConversationArchive({...f,targetPath,signal:controller.signal}),error=>error.name==='AbortError');
 await fs.appendFile(f.conversation.sessionFile,'\n{"unfinished":','utf8');await assert.rejects(exportConversationArchive({...f,targetPath}),/incomplete or damaged/);
 assert.equal(await fs.readFile(targetPath,'utf8'),'KEEP_ORIGINAL');
});
