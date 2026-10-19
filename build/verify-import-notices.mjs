// Source-level gate for newly imported code, design tokens and local assets.
// --update refreshes only adaptation digests after review; license hashes are
// immutable here. Run at introduction/upgrade, before any application build.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
function local(root,name){
 if(typeof name!=='string'||!name||/[\\:]/.test(name)||name.startsWith('/')||path.posix.normalize(name)!==name||name.split('/').includes('..'))throw Error('Invalid imported source path: '+name);
 return path.join(root,name);
}
export function verifyImports(root,catalog,{update=false}={}){
 if(catalog.schemaVersion!==1||!Array.isArray(catalog.imports))throw Error('Invalid import catalog');
 const names=new Set();let count=0;
 const imports=catalog.imports.map(entry=>{
  if(!entry.name||names.has(entry.name)||!/^https:\/\/github\.com\/[^/]+\/[^/]+$/.test(entry.repository)||!/^[a-f0-9]{40}$/.test(entry.commit)||!entry.license||!Array.isArray(entry.notices)||!entry.notices.length||!Array.isArray(entry.files)||!entry.files.length)throw Error('Incomplete import attribution: '+entry.name);
  names.add(entry.name);
  for(const notice of entry.notices){
   if(!notice.file.startsWith('licenses/')||!/^[a-f0-9]{64}$/.test(notice.sha256)||sha(fs.readFileSync(local(path.join(root,'third_party'),notice.file)))!==notice.sha256)throw Error('Imported license mismatch: '+entry.name+'/'+notice.file);
  }
  const paths=new Set();
  const files=entry.files.map(file=>{
   if(paths.has(file.path))throw Error('Duplicate imported adaptation: '+file.path);paths.add(file.path);
   const actual=sha(fs.readFileSync(local(root,file.path)));
   if(!update&&actual!==file.sha256)throw Error('Imported adaptation changed; review and update attribution: '+file.path);
   count++;return {...file,sha256:actual};
  });
  return {...entry,files};
 });
 return {catalog:{...catalog,imports},count};
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
 try{
  const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),filename=path.join(root,'third_party/import-notices.json');
  const update=process.argv.includes('--update'),result=verifyImports(root,JSON.parse(fs.readFileSync(filename,'utf8')),{update});
  if(update)fs.writeFileSync(filename,JSON.stringify(result.catalog,null,2)+'\n');
  console.log(`Imported sources and license notices verified: ${result.catalog.imports.length} imports, ${result.count} adaptations`);
 }catch(error){console.error(error.message);process.exitCode=1;}
}
