// Native OneBot uses Cindy's reviewed wording, scoped to its actual controls.
// Upstream files keep their source/license records in onebot-renderer.sync.json.
import fs from 'node:fs';
import path from 'node:path';
const keys=new Set();
// OneBot pages and Chat's teammate conversation share these words.
for(const dir of ['apps/desktop-mygo/pages/onebot','apps/desktop-mygo/pages/chat']){
 for(const name of fs.readdirSync(dir).filter(n=>n.endsWith('.go'))){
  const code=fs.readFileSync(path.join(dir,name),'utf8');
  for(const m of code.matchAll(/\b(?:bt|botT)\(\s*['"]([^'"]+)['"]/g))keys.add(m[1]);
 }
}
const sourceFiles=[['zh-CN','zh-CN'],['en','en-US']];
const full={};
for(const [source,target]of sourceFiles){
 const out={};
 function flatten(value,prefix){for(const[key,item]of Object.entries(value)){const name=prefix?prefix+'.'+key:key;if(typeof item==='string')out[name]=item;else if(item&&typeof item==='object')flatten(item,name);}}
 flatten(JSON.parse(fs.readFileSync('apps/desktop/frontend/src/features/cindy/locales/onebot.'+source+'.json','utf8')),'');full[target]=out;
}
const wanted=name=>[...keys].some(key=>name===key||name===key+'_one'||name===key+'_other'||key.endsWith('.')&&name.startsWith(key));
for(const[source,target]of sourceFiles){
 const table={};for(const[key,value]of Object.entries(full[target]))if(wanted(key))table['onebotCindy.'+key]=value;
 // Chinese has one count form; native i18n can use its canonical base for the
 // English plural pair without inventing a new translation or dropping one.
 for(const key of Object.keys(full['en-US'])){
  if(!wanted(key)||!/_one$|_other$/.test(key))continue;
  const base=key.replace(/_(one|other)$/,'');
  if(target==='zh-CN'&&full[target][base]===undefined&&full[target][base+'_other']!==undefined)table['onebotCindy.'+base]=full[target][base+'_other'];
 }
 fs.writeFileSync('apps/desktop-mygo/i18n/onebot-cindy.'+target+'.json',JSON.stringify(table,null,2)+'\n');
 console.log(target,Object.keys(table).length);
}
