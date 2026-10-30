// Pinned package bytes and the native image decoder must travel with onebot.js.
// Recording is explicit; normal builds only verify and copy the reviewed list.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
const root=fileURLToPath(new URL('../',import.meta.url)),module=path.join(root,'modules/chat-pi');
const manifest=path.join(root,'third_party/onebot-assets.json');
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
if(process.argv.includes('--record')){
 const files=[];
 const walk=(source,target)=>{for(const entry of fs.readdirSync(path.join(root,source),{withFileTypes:true})){const from=source+'/'+entry.name,to=target+'/'+entry.name;if(entry.isSymbolicLink())throw Error('Linked OneBot asset');if(entry.isDirectory())walk(from,to);else files.push({source:from,target:to,sha256:hash(fs.readFileSync(path.join(root,from)))});}};
 walk('modules/chat-pi/cindy/bot-packages','bot-packages');
 for(const name of ['sharp','@img/colour','@img/sharp-win32-x64','detect-libc','semver','silk-wasm'])walk('modules/chat-pi/node_modules/'+name,'node_modules/'+name);
 fs.writeFileSync(manifest,JSON.stringify({schemaVersion:1,files:files.sort((a,b)=>a.target.localeCompare(b.target))},null,2)+'\n');
}
const catalog=JSON.parse(fs.readFileSync(manifest));
const index=process.argv.indexOf('--copy'),destination=index<0?undefined:path.resolve(process.argv[index+1]);
if(destination){const relative=path.relative(path.join(root,'_temp'),destination);if(!relative||relative.startsWith('..')||path.isAbsolute(relative))throw Error('OneBot assets must build inside _temp/<task>');}
for(const entry of catalog.files){
 const bytes=fs.readFileSync(path.join(root,entry.source));if(hash(bytes)!==entry.sha256)throw Error('OneBot asset differs from its source: '+entry.source);
 if(destination){const target=path.join(destination,entry.target);fs.mkdirSync(path.dirname(target),{recursive:true});fs.writeFileSync(target,bytes);}
}
console.log('OneBot runtime assets verified: '+catalog.files.length);
