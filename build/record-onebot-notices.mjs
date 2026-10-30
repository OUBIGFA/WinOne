// Explicit source-notice registration for reviewed npm additions to OneBot.
// No generated licenses: copy exact original package files, fail on unknown
// licenses or missing notices, then run the independent read-only verifier.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const moduleRoot=path.join(root,'modules/chat-pi');
const catalogFile=path.join(root,'third_party/onebot-notices.json');
const catalog=JSON.parse(fs.readFileSync(catalogFile,'utf8'));
const lock=JSON.parse(fs.readFileSync(path.join(moduleRoot,'package-lock.json'),'utf8'));
const roots=[...new Set([...catalog.rootDependencies,...process.argv.slice(2)])];
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
const visited=new Set(), additions=[];
function resolve(name,base=''){
 for(;;){const key=(base?base+'/':'')+'node_modules/'+name;if(lock.packages[key])return key;if(!base)throw Error('Unresolved: '+name);const at=base.lastIndexOf('/node_modules/');base=at<0?'':base.slice(0,at);}
}
function visit(key){
 if(visited.has(key))return;visited.add(key);
 const locked=lock.packages[key];
 if(locked.os&&!locked.os.includes(process.platform)||locked.cpu&&!locked.cpu.includes(process.arch)){visited.delete(key);return;}
 const home=path.join(moduleRoot,key),pkg=JSON.parse(fs.readFileSync(path.join(home,'package.json'),'utf8'));
 if(!catalog.components.some(item=>item.sourceDirectory===key)){
  if(!['MIT','ISC','Apache-2.0','BSD-2-Clause','BSD-3-Clause','Python-2.0','0BSD','Apache-2.0 AND LGPL-3.0-or-later'].includes(pkg.license))throw Error('License needs review: '+pkg.name+' '+pkg.license);
  const names=fs.readdirSync(home).filter(name=>/^(licen[cs]e|notice|copying)(?:[.-].*)?$/i.test(name)&&fs.statSync(path.join(home,name)).isFile());
  // Some locked packages (e.g. https-proxy-agent 5.0.1) publish the complete
  // MIT license inside README instead of a separate LICENSE file. Preserve the
  // original whole document, only when the grant and disclaimer are both there.
  if(!names.some(name=>/^licen[cs]e|^copying/i.test(name))&&pkg.license==='MIT'&&fs.existsSync(path.join(home,'README.md'))){
   const readme=fs.readFileSync(path.join(home,'README.md'),'utf8');
   if(/Permission is hereby granted, free of charge/i.test(readme)&&/IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE/i.test(readme)&&/Copyright/i.test(readme))names.push('README.md');
  }
  const hasLicense=names.some(name=>/^licen[cs]e|^copying/i.test(name))||names.includes('README.md');
  let noticePolicy;
  if(!hasLicense){
   // User explicitly chose Cindy's actual distribution policy for IM dependencies
   // (2026-10-26). Only an EXACT name/version/license in the pinned metadata-only
   // appendix qualifies; preserve originals and NEVER fabricate LICENSE.
   const spec=JSON.parse(fs.readFileSync(path.join(root,'third_party/cindy/onebot.sync.json'),'utf8'));
   const evidence=execFileSync('git',['-C',path.join(root,'dist/resources/sources/cindy'),'show',spec.revision+':docs/legal/notices/desktop-win.txt'],{maxBuffer:16*1024*1024});
   const appendix=evidence.toString('utf8').split('Packages without a standalone license file (license per package.json):')[1];
   const declaration=`- npm:${pkg.name}@${pkg.version} (${pkg.license})`;
   if(!appendix?.split(/\r?\n/).includes(declaration))throw Error('No matching pinned Cindy disclosure: '+declaration);
   names.push('package.json');
   if(fs.existsSync(path.join(home,'README.md')))names.push('README.md');
   noticePolicy={kind:'cindy-package-metadata',licenseTextMissing:true,repository:spec.repository,revision:spec.revision,path:'docs/legal/notices/desktop-win.txt',sourceSHA256:hash(evidence),declaration};
  }
  if(!hasLicense&&!noticePolicy)throw Error('Original license missing: '+key);
  const notices=names.map(sourceFile=>{
   const bytes=fs.readFileSync(path.join(home,sourceFile));
   const file=`licenses/npm-onebot-${pkg.name.replaceAll('/','-').replace(/^@/,'')}-${pkg.version}-${sourceFile}.txt`;
   const destination=path.join(root,'third_party',file);
   if(fs.existsSync(destination)&&hash(fs.readFileSync(destination))!==hash(bytes))throw Error('Existing original differs: '+file);
   return {sourceFile,file,sha256:hash(bytes),bytes};
  });
  additions.push({kind:'npm',name:pkg.name,version:pkg.version,license:pkg.license,sourceDirectory:key,...(noticePolicy?{noticePolicy}:{}),notices});
 }
 for(const name of Object.keys({...lock.packages[key].dependencies,...lock.packages[key].optionalDependencies}))visit(resolve(name,key));
}
for(const name of roots)visit(resolve(name));
// Collect and validate the entire closure before changing the catalog.
for(const component of additions){
 for(const notice of component.notices){fs.writeFileSync(path.join(root,'third_party',notice.file),notice.bytes);delete notice.bytes;}
 catalog.components.push(component);
}
for(const component of catalog.components.filter(item=>item.kind==='npm')){
 const registryVersion=lock.packages[component.sourceDirectory].version;
 if(registryVersion!==component.version){
  if(component.version!=='v'+registryVersion)throw Error('Unexpected package/registry version mismatch: '+component.name);
  component.registryVersion=registryVersion;
 }
}
catalog.rootDependencies=roots;
fs.writeFileSync(catalogFile,JSON.stringify(catalog,null,2)+'\n');
const assets=fs.existsSync(path.join(root,'third_party/onebot-assets.json'))?JSON.parse(fs.readFileSync(path.join(root,'third_party/onebot-assets.json'))).files:[];
const inventory=['shared/chat/onebot.js','shared/chat/wechat-silk-worker.js','shared/chat/licenses/onebot-notices.json',...assets.map(entry=>'shared/chat/'+entry.target),...catalog.components.flatMap(component=>component.notices.map(notice=>'shared/chat/'+notice.file))].sort();
fs.writeFileSync(path.join(root,'internal/distribution/chat_onebot_runtime_paths.txt'),inventory.join('\n')+'\n');
console.log(`Recorded ${additions.length} new dependency notices; ${visited.size} in the OneBot closure`);
