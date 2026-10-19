import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';

const hash=bytes=>createHash('sha256').update(bytes).digest('hex');

// Use the locked production tree, including its peer/type dependencies. This
// is deliberately conservative; the list is not a claim that every file ships.
export function verifyFrontendNotices(root,catalog){
 const frontend=path.join(root,'apps/desktop/frontend');
 const lock=JSON.parse(fs.readFileSync(path.join(frontend,'package-lock.json'),'utf8'));
 if(lock.lockfileVersion!==3||catalog.schemaVersion!==1)throw Error('Unsupported notice or lockfile version');
 const records=new Map();
 for(const record of catalog.components.filter(item=>item.kind==='npm')){
  const key=record.name+'|'+record.version;
  if(records.has(key))throw Error('Duplicate npm notice: '+key);
  records.set(key,record);
 }
 const actual=new Set();
 for(const [location,locked] of Object.entries(lock.packages)){
  if(!location||locked.dev)continue;
  // npm installs only the matching optional platform package on this machine.
  // Other locked platforms still belong to the same monorepo notice record.
  if(locked.optional&&!fs.existsSync(path.join(frontend,location,'package.json')))continue;
  if(!location.startsWith('node_modules/')||/[\\:]/.test(location)||path.posix.normalize(location)!==location)throw Error('Invalid package location');
  const directory=path.join(frontend,location);
  const installed=JSON.parse(fs.readFileSync(path.join(directory,'package.json'),'utf8'));
  if(installed.version!==locked.version)throw Error('Installed version mismatch: '+location);
  const key=installed.name+'|'+installed.version,record=records.get(key);actual.add(key);
  if(!record||!Array.isArray(record.notices)||!record.notices.length)throw Error('Missing npm notice: '+key);
  for(const notice of record.notices){
   if(!/^(LICENSE|LICENCE|COPYING|COPYRIGHT|NOTICE)([._-][A-Za-z0-9._-]+)?$/i.test(notice.sourceFile)||!/^licenses\/[A-Za-z0-9][A-Za-z0-9._-]*\.txt$/.test(notice.file)||!/^[a-f0-9]{64}$/.test(notice.sha256))throw Error('Invalid notice record: '+key);
   const copied=fs.readFileSync(path.join(root,'third_party',notice.file));
   if(notice.sourceURL){
    // Some monorepos omit the root license from their npm tarballs. Preserve
    // the audited upstream text with a commit-pinned URL tied to package.json;
    // do not fabricate a node_modules file that npm ci would later discard.
    const source=/^https:\/\/raw\.githubusercontent\.com\/([^/]+\/[^/]+)\/([a-f0-9]{40})\/([^/]+)$/.exec(notice.sourceURL);
    if(!source||source[3]!==notice.sourceFile)throw Error('Upstream notice needs a pinned source: '+key);
    // npm metadata points at the same repository through several forms: a git
    // prefix, a commit fragment, or a monorepo subdirectory path. Normalize
    // them all before comparing against the pinned raw URL.
    const repo=(typeof installed.repository==='string'?installed.repository:installed.repository?.url??'').replace(/^git\+/,'').replace(/#.*$/,'').replace(/\.git$/,'').replace(/\/tree\/[^/]+\/.*$/,'');
    if(repo!=='https://github.com/'+source[1])throw Error('Upstream notice repository mismatch: '+key);
   }else{
    const original=fs.readFileSync(path.join(directory,notice.sourceFile));
    if(hash(original)!==notice.sha256)throw Error('Notice content mismatch: '+key);
   }
   if(hash(copied)!==notice.sha256)throw Error('Notice content mismatch: '+key);
  }
 }
 for(const key of records.keys())if(!actual.has(key))throw Error('Stale npm notice: '+key);
 return actual.size;
}

// Refresh at dependency introduction, not at packaging. Resolve the entire
// locked production tree before touching the catalog; a missing license aborts
// the update instead of producing a partially complete success report.
export function updateFrontendNotices(root,catalog){
 const frontend=path.join(root,'apps/desktop/frontend');
 const lock=JSON.parse(fs.readFileSync(path.join(frontend,'package-lock.json'),'utf8'));
 if(lock.lockfileVersion!==3||catalog.schemaVersion!==1)throw Error('Unsupported notice or lockfile version');
 const previous=new Map(catalog.components.filter(item=>item.kind==='npm').map(item=>[item.name+'|'+item.version,item]));
 const records=new Map(),copies=[];
 for(const [location,locked] of Object.entries(lock.packages)){
  if(!location||locked.dev)continue;
  if(locked.optional&&!fs.existsSync(path.join(frontend,location,'package.json')))continue;
  if(!location.startsWith('node_modules/')||/[\\:]/.test(location)||path.posix.normalize(location)!==location)throw Error('Invalid package location');
  const directory=path.join(frontend,location),installed=JSON.parse(fs.readFileSync(path.join(directory,'package.json'),'utf8'));
  if(installed.version!==locked.version)throw Error('Installed version mismatch: '+location);
  const key=installed.name+'|'+installed.version;if(records.has(key))continue;
  // License-update scripts are code, not license texts. Never sweep arbitrary
  // files merely because their name begins with "license".
  const names=fs.readdirSync(directory,{withFileTypes:true}).filter(entry=>entry.isFile()&&/^(?:LICENSE|LICENCE|COPYING|COPYRIGHT|NOTICE)(?:[._-](?:MIT|APACHE(?:-2\.0)?|BSD|MPL|OFL|ISC))?(?:\.(?:txt|md|mkd))?$/i.test(entry.name)).map(entry=>entry.name).sort();
  let notices=names.map(sourceFile=>{
   const bytes=fs.readFileSync(path.join(directory,sourceFile));
   const prior=previous.get(key)?.notices.find(notice=>notice.sourceFile===sourceFile&&!notice.sourceURL);
   const file=prior?.file||'licenses/'+('npm-'+installed.name+'-'+installed.version+'-'+sourceFile+'.txt').replace(/[^A-Za-z0-9._-]/g,'-');
   if(!/^licenses\/[A-Za-z0-9][A-Za-z0-9._-]*\.txt$/.test(file))throw Error('Invalid notice destination: '+file);
   copies.push({file,bytes});return {sourceFile,file,sha256:hash(bytes)};
  });
  if(!notices.length){
   notices=previous.get(key)?.notices.filter(notice=>notice.sourceURL)||[];
   if(!notices.length)throw Error('Missing npm license source: '+key+'; inspect the exact upstream revision before updating');
  }
  records.set(key,{...previous.get(key),kind:'npm',name:installed.name,version:installed.version,license:installed.license||previous.get(key)?.license,notices});
 }
 const next={...catalog,components:[...catalog.components.filter(item=>item.kind!=='npm'),...[...records.values()].sort((a,b)=>(a.name+'|'+a.version).localeCompare(b.name+'|'+b.version))]};
 // All source discovery succeeded. Preserve unrelated notices and old files;
 // only copy the explicitly resolved license texts for this dependency tree.
 for(const {file,bytes} of copies){const target=path.join(root,'third_party',file);fs.mkdirSync(path.dirname(target),{recursive:true});if(!fs.existsSync(target)||hash(fs.readFileSync(target))!==hash(bytes))fs.writeFileSync(target,bytes);}
 verifyFrontendNotices(root,next);
 return next;
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
 try{
  const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
  const args=process.argv.slice(2),update=args.includes('--update');
  const filename=args.find(arg=>!arg.startsWith('--'))||path.join(root,'third_party/runtime-notices.json');
  let catalog=JSON.parse(fs.readFileSync(filename,'utf8'));
  if(update){catalog=updateFrontendNotices(root,catalog);fs.writeFileSync(filename,JSON.stringify(catalog,null,2)+'\n');}
  console.log('Frontend dependency notices verified: '+verifyFrontendNotices(root,catalog));
 }catch(error){console.error(error.message);process.exitCode=1;}
}
