// Flatten the pinned Cindy permission descriptions for the native plugin
// confirmation. Generated files are not hand-edited; --check is read-only.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
for(const [source,locale] of [['zh-CN','zh-CN'],['en','en-US']]){
 const tree=JSON.parse(fs.readFileSync(path.join(root,'modules/chat-pi/cindy/ghost/locales',source+'.json'),'utf8'));
 const values={};
 function visit(value,key){if(typeof value==='string')values['ghostPermission.'+key]=value;else if(value&&typeof value==='object')for(const [name,child] of Object.entries(value))visit(child,key?key+'.'+name:name);}
 visit(tree.settings?.ghosts?.perm,'');
 if(!Object.keys(values).length)throw Error('Pinned Cindy permission translations are missing');
 const file=path.join(root,'apps/desktop-mygo/i18n','ghost-permissions.'+locale+'.json');
 const text=JSON.stringify(values,null,2)+'\n';
 if(process.argv.includes('--check')){if(fs.readFileSync(file,'utf8')!==text)throw Error('Native permission translations are stale: '+file);}else fs.writeFileSync(file,text);
 console.log('Native Cindy permission translations: '+locale+' '+Object.keys(values).length);
}
