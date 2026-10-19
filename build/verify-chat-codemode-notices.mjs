import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';

// Verify locked source notices immediately and, when provided, shipped copies.
const root=fileURLToPath(new URL('../',import.meta.url));
const moduleRoot=path.join(root,'modules/chat-pi');
const catalog=JSON.parse(await fs.readFile(path.join(root,'third_party/chat-codemode-notices.json'),'utf8'));
const lock=JSON.parse(await fs.readFile(path.join(moduleRoot,'package-lock.json'),'utf8'));
const bundle=process.argv[2];
const hash=async file=>createHash('sha256').update(await fs.readFile(file)).digest('hex');
for(const component of catalog.components){
 if(component.kind==='npm'){
  const installed=JSON.parse(await fs.readFile(path.join(moduleRoot,component.sourceDirectory,'package.json'),'utf8'));
  if(installed.name!==component.name||installed.version!==component.version||lock.packages[component.sourceDirectory]?.version!==component.version)throw Error('Code Mode dependency version differs: '+component.name);
 }
 for(const notice of component.notices){
  if(await hash(path.join(root,'third_party',notice.file))!==notice.sha256)throw Error('Code Mode notice changed: '+notice.file);
  if(bundle&&await hash(path.join(bundle,notice.file))!==notice.sha256)throw Error('Shipped Code Mode notice differs: '+notice.file);
 }
}
for(const resource of catalog.resources){
 if(await hash(path.join(moduleRoot,resource.source))!==resource.sha256)throw Error('Code Mode resource changed: '+resource.source);
 if(bundle&&await hash(path.join(bundle,resource.file))!==resource.sha256)throw Error('Shipped Code Mode resource differs: '+resource.file);
}
console.log('Code Mode dependency notices and VM resource verified');
