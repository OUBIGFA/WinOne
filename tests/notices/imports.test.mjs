import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {verifyImports} from '../../build/verify-import-notices.mjs';
const sha=text=>createHash('sha256').update(text).digest('hex');
function fixture(t){
 const base=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../_temp/notice-tests');fs.mkdirSync(base,{recursive:true});
 const root=fs.mkdtempSync(path.join(base,'imports-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 fs.mkdirSync(path.join(root,'third_party/licenses'),{recursive:true});fs.mkdirSync(path.join(root,'src'));
 fs.writeFileSync(path.join(root,'third_party/licenses/example.txt'),'original license');fs.writeFileSync(path.join(root,'src/example.css'),'example style');
 return {root,catalog:{schemaVersion:1,imports:[{name:'example',repository:'https://github.com/example/project',commit:'a'.repeat(40),license:'MIT',notices:[{file:'licenses/example.txt',sha256:sha('original license')}],files:[{path:'src/example.css',sha256:sha('example style')}]}]}};
}
test('checks copied implementation and original attribution before building',t=>{const f=fixture(t);assert.equal(verifyImports(f.root,f.catalog).count,1);fs.writeFileSync(path.join(f.root,'src/example.css'),'reviewed change');assert.throws(()=>verifyImports(f.root,f.catalog),/adaptation changed/);const result=verifyImports(f.root,f.catalog,{update:true});assert.equal(verifyImports(f.root,result.catalog).count,1);});
test('updating source digests never blesses a changed license',t=>{const f=fixture(t);fs.writeFileSync(path.join(f.root,'third_party/licenses/example.txt'),'changed');assert.throws(()=>verifyImports(f.root,f.catalog,{update:true}),/license mismatch/);});
test('rejects unpinned sources, missing licenses and path escapes',t=>{const f=fixture(t),item=f.catalog.imports[0];item.commit='main';assert.throws(()=>verifyImports(f.root,f.catalog),/attribution/);item.commit='a'.repeat(40);item.files[0].path='../outside';assert.throws(()=>verifyImports(f.root,f.catalog),/path/);item.notices=[];assert.throws(()=>verifyImports(f.root,f.catalog),/attribution/);});
