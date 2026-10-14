import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {prepareAttachments} from './attachments.mjs';
import * as attachmentsModule from './attachments.mjs';

test('an omitted Go attachment slice is an empty attachment list',async()=>{
 assert.deepEqual(await prepareAttachments('C:\\fixture',null),{attachments:[],images:[],context:''});
});

test('imported text is sent as bounded attachment context, with the original name',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-attached-'));
 const file=path.join(root,'attachments','one','note.txt');await fs.mkdir(path.dirname(file),{recursive:true});await fs.writeFile(file,'Useful attached content');
 const result=await prepareAttachments(root,[file],false);
 assert.match(result.context,/Useful attached content/);assert.equal(result.attachments[0].name,'note.txt');assert.equal(result.images.length,0);
 await assert.rejects(prepareAttachments(root,[path.join(root,'outside.txt')],false),/imported/);
});

test('image attachments require an image-capable model and carry image data',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-image-'));
 const file=path.join(root,'attachments','one','pixel.png');await fs.mkdir(path.dirname(file),{recursive:true});await fs.writeFile(file,await fs.readFile(new URL('../../internal/branding/chat-welcome.png',import.meta.url)));
 await assert.rejects(prepareAttachments(root,[file],false),/images/);
 const result=await prepareAttachments(root,[file],true);assert.equal(result.images[0].mimeType,'image/png');assert.ok(result.images[0].data.length>20);
});

test('large original images use Pi resizing before acceptance and retain their original files',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-large-image-'));
 const file=path.join(root,'attachments','one','large.png');await fs.mkdir(path.dirname(file),{recursive:true});
 const pixel=await fs.readFile(new URL('../../internal/branding/chat-welcome.png',import.meta.url));
 const original=Buffer.concat([pixel,Buffer.alloc(13*1024*1024)]);await fs.writeFile(file,original);
 const result=await prepareAttachments(root,[file],true);assert.ok(result.images[0].data.length<4.5*1024*1024);assert.deepEqual(await fs.readFile(file),original);
});

test('the actual request payload has one aggregate image budget without deleting text or history',async()=>{
 const pixel=await fs.readFile(new URL('../../internal/branding/chat-welcome.png',import.meta.url));
 const url='data:image/png;base64,'+Buffer.concat([pixel,Buffer.alloc(1024*1024)]).toString('base64');
 const payload={model:'fixture',messages:[{role:'user',content:[{type:'text',text:'Historical instruction retained'},...Array.from({length:13},()=>({type:'image_url',image_url:{url}}))]}]};
 const result=await attachmentsModule.limitChatPayload(payload);
 assert.ok(Buffer.byteLength(JSON.stringify(result))<=16*1024*1024);assert.equal(result.messages[0].content.filter(part=>part.type==='image_url').length,13);assert.ok(JSON.stringify(result).includes('Historical instruction retained'));assert.equal(payload.messages[0].content[1].image_url.url,url);
});
