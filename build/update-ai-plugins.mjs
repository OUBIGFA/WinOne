// Refresh the complete bundled Magpie registry from an explicitly selected
// checkout. This only reads source; it never installs or executes a plugin.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';

const root=path.resolve(import.meta.dirname,'..');
const source=path.resolve(process.argv[2]??'');
if(!process.argv[2])throw Error('Usage: node build/update-ai-plugins.mjs <community-source>');
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
const revision=execFileSync('git',['-C',source,'rev-parse','HEAD'],{encoding:'utf8',windowsHide:true}).trim();
const registryBytes=await fs.readFile(path.join(source,'registry.json'));
const registry=JSON.parse(registryBytes);
const old=JSON.parse(await fs.readFile(path.join(root,'third_party/ai-plugins.json'),'utf8'));
const packages=new Map();
for(const folder of await fs.readdir(path.join(source,'packages'))){
 const filename=path.join(source,'packages',folder,'package.json');
 try{const info=JSON.parse(await fs.readFile(filename,'utf8'));packages.set(info.name,{info,folder});}
 catch(error){if(error.code!=='ENOENT')throw error;}
}
async function filesIn(directory,relative){
 if(path.isAbsolute(relative)||relative.split(/[\\/]/).includes('..')||/[\x00*?\[\]]/.test(relative))throw Error('Review the package file pattern: '+relative);
 const name=path.join(directory,relative),stat=await fs.lstat(name);
 if(stat.isSymbolicLink())throw Error('A bundled plugin must not contain symbolic links: '+name);
 if(stat.isDirectory()){const entries=await fs.readdir(name);return (await Promise.all(entries.sort().map(entry=>filesIn(directory,path.posix.join(relative,entry))))).flat();}
 if(!stat.isFile())throw Error('Not a regular plugin resource: '+name);
 return [relative.replaceAll('\\','/')];
}
const plugins=[];
for(const listed of registry.plugins){
 const found=packages.get(listed.package);if(!found)throw Error('Listed plugin has no source: '+listed.package);
 const {info,folder}=found,directory='packages/'+folder;
 if(Object.keys(info.dependencies??{}).length)throw Error('Review new runtime dependencies before bundling '+info.name);
 const names=new Set(['package.json',...(await Promise.all((info.files??[]).map(name=>filesIn(path.join(source,directory),name)))).flat()]);
 const files=[];
 for(const name of [...names].sort()){
  const bytes=await fs.readFile(path.join(source,directory,name));
  files.push({path:directory+'/'+name,size:bytes.length,sha256:hash(bytes)});
 }
 plugins.push({...listed,providers:listed.providers??[],version:info.version,directory,entry:(info.main??info.magpie?.middleware??info.magpie?.agent??info.magpie?.page?.file??'').replace(/^\.\//,''),files});
}
const licenseBytes=await fs.readFile(path.join(source,'LICENSE'));
const previousLicense=await fs.readFile(path.join(root,'third_party',old.license.file));
if(licenseBytes.toString('utf8').replaceAll('\r\n','\n')!==previousLicense.toString('utf8').replaceAll('\r\n','\n'))throw Error('Community license changed; review it before updating the lock');
const lock={...old,license:{...old.license,sha256:hash(licenseBytes)},revision,registrySHA256:hash(registryBytes),plugins};
const catalog={bun:old.bun,plugins:plugins.map(p=>({...p,directory:p.directory.slice('packages/'.length),files:p.files.map(f=>({...f,path:f.path.slice(p.directory.length+1)}))}))};
const inventory=[...plugins.flatMap(p=>p.files.map(f=>'shared/ai/plugins/'+f.path.slice('packages/'.length))),'shared/ai/plugins/LICENSE.txt','shared/ai/plugins/source-lock.json','shared/ai/runtime/bun.exe'].sort();
// These three generated projections must change together: the build checks
// inventory, and the runtime checks the very same bytes before loading code.
for(const [name,value]of [['third_party/ai-plugins.json',lock],['modules/ai-magpie/plugin-catalog.json',catalog]])await fs.writeFile(path.join(root,name),JSON.stringify(value,null,2)+'\n');
await fs.writeFile(path.join(root,'third_party',old.license.file),licenseBytes);
await fs.writeFile(path.join(root,'internal/distribution/ai_plugin_runtime_paths.txt'),inventory.join('\n')+'\n');
console.log(`Locked ${plugins.length} bundled plugins at ${revision}; ${inventory.length} runtime files`);
