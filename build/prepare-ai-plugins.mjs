// Package exactly the reviewed registry. Updates first regenerate the lock
// through update-ai-plugins.mjs; mismatched sources never produce a bundle
// whose runtime rejects the very files the build just copied.
import fs from 'node:fs/promises';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {isDeepStrictEqual} from 'node:util';
import {createHash} from 'node:crypto';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const args=process.argv.slice(2);
if(args.length!==3)throw Error('Usage: prepare-ai-plugins.mjs <community-source> <bun.exe> <AI-output>');
const [source,bun,output]=args.map(value=>path.resolve(value));
execFileSync('git',['-C',root,'check-ignore','--quiet','--no-index',path.join(output,'plugin-output.probe')]);
const lock=JSON.parse(await fs.readFile(path.join(root,'third_party/ai-plugins.json'),'utf8'));
async function regularFile(filename){
 const stat=await fs.lstat(filename);
 if(!stat.isFile()||stat.isSymbolicLink())throw Error('Expected a regular resource file: '+filename);
 return await fs.readFile(filename);
}
function descendant(parent,relative){
 const full=path.resolve(parent,relative);
 if(!full.startsWith(parent+path.sep)||path.isAbsolute(relative)||relative.split(/[\\/]/).includes('..'))throw Error('Invalid plugin resource path');
 return full;
}
// Validate the complete input before replacing any output resources.
const resources=[];
for(const plugin of lock.plugins){
 const folder=plugin.directory.replace(/^packages\//,'');
 if(!/^[a-z0-9-]+$/.test(folder))throw Error('Invalid plugin folder');
 for(const file of plugin.files){
  const bytes=await regularFile(descendant(source,file.path));
  if(bytes.length!==file.size||createHash('sha256').update(bytes).digest('hex')!==file.sha256)throw Error('Plugin source differs from its reviewed lock: '+file.path);
  resources.push({relative:'plugins/'+folder+'/'+file.path.slice(plugin.directory.length+1),bytes});
 }
}
const bunBytes=await regularFile(bun);
if(createHash('sha256').update(bunBytes).digest('hex')!==lock.bun.executableSHA256)throw Error('Bun differs from its reviewed lock');
resources.push({relative:'runtime/bun.exe',bytes:bunBytes});
resources.push({relative:'plugins/LICENSE.txt',bytes:await regularFile(path.join(source,'LICENSE'))});
const inventory=(await fs.readFile(path.join(root,'internal/distribution/ai_plugin_runtime_paths.txt'),'utf8')).trim().split(/\s+/).sort();
const expectedPaths=[...resources.map(resource=>'shared/ai/'+resource.relative),'shared/ai/plugins/source-lock.json'].sort();
if(!isDeepStrictEqual(inventory,expectedPaths))throw Error('Distribution plugin inventory differs from the reviewed resources');
for(const resource of resources){
 const destination=descendant(output,resource.relative);
 await fs.mkdir(path.dirname(destination),{recursive:true});
 await fs.writeFile(destination,resource.bytes);
}
await fs.writeFile(path.join(output,'plugins/source-lock.json'),JSON.stringify(lock,null,2)+'\n');
console.log(`Prepared ${lock.plugins.length} account plugins and shared Bun`);
