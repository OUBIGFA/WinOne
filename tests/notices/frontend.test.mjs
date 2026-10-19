import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {verifyFrontendNotices,updateFrontendNotices} from '../../build/verify-frontend-notices.mjs';

function fixture(t){
 const base=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../_temp/notice-tests');fs.mkdirSync(base,{recursive:true});
 const root=fs.mkdtempSync(path.join(base,'frontend-'));
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
test('accepts an upstream license omitted by npm only from its repository and a pinned commit',t=>{
 const f=fixture(t);fs.renameSync(path.join(f.packageDir,'LICENSE'),path.join(f.packageDir,'not-published-license'));
 fs.writeFileSync(path.join(f.packageDir,'package.json'),JSON.stringify({name:'example',version:'1.0.0',repository:{url:'https://github.com/owner/example.git'}}));
 const notice=f.catalog.components[0].notices[0];notice.sourceURL='https://raw.githubusercontent.com/owner/example/'+('a'.repeat(40))+'/LICENSE';
 assert.equal(verifyFrontendNotices(f.root,f.catalog),1);
 notice.sourceURL=notice.sourceURL.replace('owner/example','stranger/example');assert.throws(()=>verifyFrontendNotices(f.root,f.catalog),/repository/);
 notice.sourceURL='https://raw.githubusercontent.com/owner/example/main/LICENSE';assert.throws(()=>verifyFrontendNotices(f.root,f.catalog),/pinned/);
});
test('updates new dependencies at intake and keeps unrelated notice records',t=>{
 const f=fixture(t),other={kind:'go',name:'unrelated',version:'1',notices:[]};
 fs.writeFileSync(path.join(f.packageDir,'license-update.mjs'),'not a license');
 const next=updateFrontendNotices(f.root,{schemaVersion:1,components:[other]});
 assert.deepEqual(next.components[0],other);assert.equal(verifyFrontendNotices(f.root,next),1);
 assert.equal(next.components[1].notices.length,1);assert.equal(next.components[1].notices[0].sourceFile,'LICENSE');
 assert.equal(fs.readFileSync(path.join(f.root,'third_party',next.components[1].notices[0].file),'utf8'),'fixture license');
});
test('does not manufacture a license when the newly installed package omits one',t=>{
 const f=fixture(t);fs.renameSync(path.join(f.packageDir,'LICENSE'),path.join(f.packageDir,'license-update.mjs'));
 assert.throws(()=>updateFrontendNotices(f.root,{schemaVersion:1,components:[]}),/Missing npm license source/);
});

test('accepts a repository URL that points at a monorepo subdirectory',t=>{
 const f=fixture(t);fs.renameSync(path.join(f.packageDir,'LICENSE'),path.join(f.packageDir,'not-published-license'));
 fs.writeFileSync(path.join(f.packageDir,'package.json'),JSON.stringify({name:'example',version:'1.0.0',repository:'https://github.com/owner/example/tree/main/packages/example'}));
 f.catalog.components[0].notices[0].sourceURL='https://raw.githubusercontent.com/owner/example/'+('a'.repeat(40))+'/LICENSE';
 assert.equal(verifyFrontendNotices(f.root,f.catalog),1);
});
