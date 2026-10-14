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
   if(!/^(LICENSE|LICENCE|COPYING|NOTICE)([._-][A-Za-z0-9._-]+)?$/i.test(notice.sourceFile)||!/^licenses\/[A-Za-z0-9][A-Za-z0-9._-]*\.txt$/.test(notice.file)||!/^[a-f0-9]{64}$/.test(notice.sha256))throw Error('Invalid notice record: '+key);
   const copied=fs.readFileSync(path.join(root,'third_party',notice.file));
   if(notice.sourceURL){
    // Some monorepos omit the root license from their npm tarballs. Preserve
    // the audited upstream text with a commit-pinned URL tied to package.json;
    // do not fabricate a node_modules file that npm ci would later discard.
    const source=/^https:\/\/raw\.githubusercontent\.com\/([^/]+\/[^/]+)\/([a-f0-9]{40})\/([^/]+)$/.exec(notice.sourceURL);
    if(!source||source[3]!==notice.sourceFile)throw Error('Upstream notice needs a pinned source: '+key);
    const repo=(typeof installed.repository==='string'?installed.repository:installed.repository?.url??'').replace(/^git\+/,'').replace(/\.git$/,'');
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

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
 try{
  const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
  const catalog=JSON.parse(fs.readFileSync(process.argv[2]||path.join(root,'third_party/runtime-notices.json'),'utf8'));
  console.log('Frontend dependency notices verified: '+verifyFrontendNotices(root,catalog));
 }catch(error){console.error(error.message);process.exitCode=1;}
}
