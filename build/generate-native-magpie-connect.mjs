// Extract the reviewed upstream's pure example builders. No upstream app,
// network, environment or credentials are made available to the evaluation.
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
const root=path.resolve(import.meta.dirname,'..'),source=path.resolve(process.argv[2]||'');
if(!process.argv[2])throw Error('Pass Magpie source');
const revision=JSON.parse(fs.readFileSync(path.join(root,'third_party/sources.json'))).magpie.commit;
const filename='internal/gui/assets/plugins/gateway/client.js';
const bytes=execFileSync('git',['-C',source,'show',revision+':'+filename],{maxBuffer:8<<20}),text=bytes.toString('utf8');
const a=text.indexOf('const FLAVORS ='),b=text.indexOf('// every exposed model',a);if(a<0||b<0)throw Error('Gateway example boundary changed');
const context=vm.createContext({navigator:{platform:'Win32'},snipOS:'windows'});
vm.runInContext(text.slice(a,b)+'\nthis.examples={FLAVORS,curlSnippet,envSnippet};',context,{timeout:1000});
const result=[];
const emit=(name,value)=>{const target=path.join(root,name),text=JSON.stringify(value,null,2)+'\n';if(process.argv.includes('--check')){if(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n')!==text)throw Error('Native connection examples differ: '+name);}else fs.writeFileSync(target,text);};
for(const [id,f]of Object.entries(context.examples.FLAVORS)){
 const base=f.base('WINONE_ROOT');const row={id,name:f.name,note:f.note,baseSuffix:base.slice('WINONE_ROOT'.length),decide:!!f.decide,examples:{}};
 for(const os of ['windows','unix']){
  context.snipOS=os;
  row.examples[os]={curl:context.examples.curlSnippet(f.curl(base,'WINONE_MODEL','WINONE_KEY')),python:f.python(base,'WINONE_MODEL','WINONE_KEY'),node:f.node(base,'WINONE_MODEL','WINONE_KEY')};
  if(f.baseEnv)row.examples[os].shell=context.examples.envSnippet([[f.baseEnv,base],[f.keyEnv,'WINONE_KEY']]);
 }
 result.push(row);
}
const out='apps/desktop-mygo/pages/aiservices/gateway_examples.json';emit(out,result);
emit('third_party/magpie-native-connect.json',{repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice:'licenses/upstream-magpie-'+revision+'-0-LICENSE.txt',source:filename,sourceSHA256:createHash('sha256').update(bytes).digest('hex'),output:out,adaptations:['Replace root/model/key placeholders at render time; WinOne requires its own gateway key']});
console.log('Generated upstream connection examples for',result.length,'APIs');
