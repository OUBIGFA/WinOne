// Source-level licensing gate for the exact OneBot dependency closure. It does
// not refresh hashes: license/version changes require an explicit review.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';

const root=fileURLToPath(new URL('../',import.meta.url));
const moduleRoot=path.join(root,'modules/chat-pi');
const catalog=JSON.parse(fs.readFileSync(path.join(root,'third_party/onebot-notices.json'),'utf8'));
const lock=JSON.parse(fs.readFileSync(path.join(moduleRoot,'package-lock.json'),'utf8'));
const manifest=JSON.parse(fs.readFileSync(path.join(moduleRoot,'package.json'),'utf8'));
const spec=JSON.parse(fs.readFileSync(path.join(root,catalog.sourceSpec),'utf8'));
const taskSpec=JSON.parse(fs.readFileSync(path.join(root,'third_party/cindy/onebot-tasks.sync.json'),'utf8'));
const worktreeSpec=JSON.parse(fs.readFileSync(path.join(root,'third_party/cindy/onebot-worktree.sync.json'),'utf8'));
const voiceSpec=JSON.parse(fs.readFileSync(path.join(root,'third_party/cindy/onebot-voice.sync.json'),'utf8'));
const importSpec=JSON.parse(fs.readFileSync(path.join(root,'third_party/cindy/onebot-import.sync.json'),'utf8'));
const wechatSpec=JSON.parse(fs.readFileSync(path.join(root,'third_party/cindy/onebot-wechat.sync.json'),'utf8'));
const assets=JSON.parse(fs.readFileSync(path.join(root,'third_party/onebot-assets.json'),'utf8')).files;
if(voiceSpec.revision!==spec.revision||voiceSpec.license!==spec.license||voiceSpec.repository!==spec.repository)throw Error('OneBot voice source attribution differs from the reviewed source');
if(worktreeSpec.revision!==spec.revision||worktreeSpec.license!==spec.license||worktreeSpec.repository!==spec.repository)throw Error('OneBot worktree source attribution differs from the reviewed source');
if(taskSpec.revision!==spec.revision||taskSpec.license!==spec.license||taskSpec.repository!==spec.repository)throw Error('OneBot task source attribution differs from the reviewed source');
const hash=file=>createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const bundle=process.argv[2];
const declared=new Map(catalog.components.filter(item=>item.kind==='npm').map(item=>[item.sourceDirectory,item]));
const visited=new Set();
function dependency(name,from=''){
 let base=from;
 for(;;){
  const key=(base?base+'/':'')+'node_modules/'+name;
  if(lock.packages[key])return key;
  if(!base)throw Error('Unresolved OneBot dependency: '+name);
  const index=base.lastIndexOf('/node_modules/');
  base=index<0?'':base.slice(0,index);
 }
}
function visit(key){
 if(visited.has(key))return;
 const locked=lock.packages[key];
 if(locked.os&&!locked.os.includes(process.platform)||locked.cpu&&!locked.cpu.includes(process.arch))return;
 visited.add(key);
 const entry=lock.packages[key];
 if(!declared.has(key))throw Error('OneBot dependency notice missing: '+key);
 for(const name of Object.keys({...entry.dependencies,...entry.optionalDependencies}))visit(dependency(name,key));
}
for(const name of catalog.rootDependencies){
 const key=dependency(name);
 if(manifest.dependencies[name]!==lock.packages[key].version)throw Error('OneBot direct dependency is not pinned: '+name);
 visit(key);
}
for(const component of catalog.components){
 if(component.kind==='npm'){
  if(!visited.has(component.sourceDirectory))throw Error('Stale OneBot dependency notice: '+component.name);
  const pkg=JSON.parse(fs.readFileSync(path.join(moduleRoot,component.sourceDirectory,'package.json'),'utf8'));
  if(pkg.name!==component.name||pkg.version!==component.version||pkg.license!==component.license||lock.packages[component.sourceDirectory]?.version!==(component.registryVersion??component.version))throw Error('OneBot dependency metadata differs: '+component.name);
 }else if(component.revision!==spec.revision||component.license!==spec.license||component.repository!==spec.repository){
  throw Error('OneBot source attribution differs from the sync spec');
 }
 const policy=component.noticePolicy;
 if(policy){
  const declaration=`- npm:${component.name}@${component.registryVersion??component.version} (${component.license})`;
  if(policy.kind!=='cindy-package-metadata'||policy.licenseTextMissing!==true||policy.repository!==spec.repository||policy.revision!==spec.revision||policy.path!=='docs/legal/notices/desktop-win.txt'||policy.declaration!==declaration||!component.notices.some(notice=>notice.sourceFile==='package.json'))throw Error('Invalid Cindy metadata-only disclosure: '+component.name);
  const cindy=path.join(root,'dist/resources/sources/cindy');
  if(fs.existsSync(path.join(cindy,'.git'))){
   const original=execFileSync('git',['-C',cindy,'show',spec.revision+':'+policy.path],{maxBuffer:16*1024*1024});
   if(createHash('sha256').update(original).digest('hex')!==policy.sourceSHA256||!original.toString('utf8').split('Packages without a standalone license file (license per package.json):')[1]?.split(/\r?\n/).includes(declaration))throw Error('Cindy disclosure evidence changed: '+component.name);
  }
 }else if(component.kind==='npm'&&component.notices.some(notice=>notice.sourceFile==='package.json'))throw Error('Package metadata is not a license text: '+component.name);
 for(const notice of component.notices){
  if(hash(path.join(root,'third_party',notice.file))!==notice.sha256)throw Error('OneBot license changed: '+notice.file);
  if(component.kind==='npm'&&hash(path.join(moduleRoot,component.sourceDirectory,notice.sourceFile))!==notice.sha256)throw Error('Installed OneBot license differs: '+component.name);
  if(bundle&&hash(path.join(bundle,notice.file))!==notice.sha256)throw Error('Shipped OneBot license differs: '+notice.file);
 }
}
const inventory=fs.readFileSync(path.join(root,'internal/distribution/chat_onebot_runtime_paths.txt'),'utf8').trim().split(/\r?\n/).sort();
const expected=['shared/chat/onebot.js','shared/chat/wechat-silk-worker.js','shared/chat/licenses/onebot-notices.json',...assets.map(entry=>'shared/chat/'+entry.target),...catalog.components.flatMap(component=>component.notices.map(notice=>'shared/chat/'+notice.file))].sort();
if(JSON.stringify(inventory)!==JSON.stringify(expected))throw Error('OneBot runtime inventory differs from the reviewed notices');
if(!spec.files.length)throw Error('OneBot source sync is empty');
for(const file of [...spec.files,...taskSpec.files,...worktreeSpec.files,...voiceSpec.files,...importSpec.files,...wechatSpec.files]){
 if(hash(path.join(root,file.target))!==file.sha256)throw Error('OneBot source differs from pinned sync: '+file.target);
}
for(const entry of assets){if(hash(path.join(root,entry.source))!==entry.sha256||bundle&&hash(path.join(bundle,entry.target))!==entry.sha256)throw Error('OneBot asset changed: '+entry.target);}
console.log(`OneBot notices verified: ${spec.files.length} source files, ${visited.size} npm dependencies`);
