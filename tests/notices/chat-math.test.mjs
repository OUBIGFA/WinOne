import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {verifyMathNotices} from '../../build/verify-chat-math-notices.mjs';

const root=fileURLToPath(new URL('../../',import.meta.url));
const catalog=()=>JSON.parse(fs.readFileSync(path.join(root,'third_party/chat-math-notices.json'),'utf8'));

test('locked MathJax dependencies retain original font evidence and source',()=>{
 assert.deepEqual(verifyMathNotices(),{dependencies:3,fonts:105,files:12});
});
test('an empty license list is not accepted as verification',()=>{
 const value=catalog(); value.components[1].notices=[];
 assert.throws(()=>verifyMathNotices({catalog:value}),/lacks original license evidence/);
});
test('unresolved license review cannot be bypassed by supplying text',()=>{
 const value=catalog(); value.components[1].licenseReview.status='blocked';
 assert.throws(()=>verifyMathNotices({catalog:value}),/lacks original license evidence/);
});
test('font evidence and full source must match their reviewed hashes',()=>{
 for(const target of ['fontEvidence','resources']){
  const value=catalog();
  (target==='fontEvidence'?value.fontEvidence:value.resources[0]).sha256='0'.repeat(64);
  assert.throws(()=>verifyMathNotices({catalog:value}),/Math notice\/resource changed/);
 }
});
test('the published source archive remains bound to the npm lock',()=>{
 const value=catalog(); value.resources[0].integrity='sha512-not-the-source';
 assert.throws(()=>verifyMathNotices({catalog:value}),/npm lock integrity/);
});
test('dropping one font license also breaks the exact runtime inventory',()=>{
 const value=catalog(); value.components[1].notices.splice(2,1);
 assert.throws(()=>verifyMathNotices({catalog:value}),/runtime inventory differs/);
});
