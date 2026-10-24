import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
const root=new URL('../../',import.meta.url);
const read=path=>readFile(new URL(path,root));
const digest=bytes=>createHash('sha256').update(bytes).digest('hex');

for(const filename of ['chat-interaction-source.json','chat-runtime-source.json','chat-resilience-source.json','chat-links-source.json','chat-queue-source.json'])test(`Cindy chat imports retain the pinned source, license and notice (${filename})`,async()=>{
 const source=JSON.parse(await read('third_party/cindy/'+filename));
 assert.equal(source.repository,'https://github.com/makecindy/cindy');
 assert.match(source.revision,/^[a-f0-9]{40}$/);
 assert.equal(digest(await read(source.licenseFile)),source.licenseSHA256);
 assert.equal(digest(await read(source.noticeFile)),source.noticeSHA256);
 for(const file of source.files){
  assert.match(file.sourceSHA256,/^[a-f0-9]{64}$/);
  assert.ok(file.adaptation);
  const bytes=await read(file.target);assert.ok(bytes.length>0,'empty import: '+file.target);
  if(file.unchanged)assert.equal(digest(bytes),file.sourceSHA256,'changed upstream source: '+file.target);
 }
});
