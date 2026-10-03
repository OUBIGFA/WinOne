import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {verifyFrontendNotices} from '../../build/verify-frontend-notices.mjs';

function fixture(t){
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'winone-notices-'));
 t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const frontend=path.join(root,'apps/desktop/frontend'),packageDir=path.join(frontend,'node_modules/example');
 fs.mkdirSync(packageDir,{recursive:true});fs.mkdirSync(path.join(root,'third_party/licenses'),{recursive:true});
 fs.writeFileSync(path.join(frontend,'package-lock.json'),JSON.stringify({lockfileVersion:3,packages:{'':{},'node_modules/example':{version:'1.0.0'},'node_modules/dev-only':{version:'2.0.0',dev:true}}}));
 fs.writeFileSync(path.join(packageDir,'package.json'),JSON.stringify({name:'example',version:'1.0.0'}));
 fs.writeFileSync(path.join(packageDir,'LICENSE'),'fixture license');fs.writeFileSync(path.join(root,'third_party/licenses/example.txt'),'fixture license');
 const sha256=createHash('sha256').update('fixture license').digest('hex');
 return {root,packageDir,catalog:{schemaVersion:1,components:[{kind:'npm',name:'example',version:'1.0.0',notices:[{sourceFile:'LICENSE',file:'licenses/example.txt',sha256}]}]}};
}
test('covers production entries and ignores dev-only packages',t=>{const f=fixture(t);assert.equal(verifyFrontendNotices(f.root,f.catalog),1);});
test('rejects missing notices and wrong installed versions',t=>{
 const f=fixture(t);assert.throws(()=>verifyFrontendNotices(f.root,{schemaVersion:1,components:[]}),/Missing/);
 fs.writeFileSync(path.join(f.packageDir,'package.json'),JSON.stringify({name:'example',version:'2.0.0'}));
 assert.throws(()=>verifyFrontendNotices(f.root,f.catalog),/version/);
});
test('rejects changed license content',t=>{const f=fixture(t);fs.writeFileSync(path.join(f.packageDir,'LICENSE'),'changed');assert.throws(()=>verifyFrontendNotices(f.root,f.catalog),/content/);});
