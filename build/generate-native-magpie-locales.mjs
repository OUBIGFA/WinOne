// Keep the native port's labels identical to the reviewed upstream page.
// Read literal dictionaries; never execute upstream UI code while generating.
import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import ts from '../apps/desktop/frontend/node_modules/typescript/lib/typescript.js';
const root=path.resolve(import.meta.dirname,'..'),source=path.resolve(process.argv[2]||'');
if(!process.argv[2])throw Error('Pass the selected Magpie source');
const revision=JSON.parse(fs.readFileSync(path.join(root,'third_party/sources.json'))).magpie.commit;
const inputs={},zh={};
const emit=(name,value)=>{const target=path.join(root,name),text=JSON.stringify(value,null,2)+'\n';if(process.argv.includes('--check')){if(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n')!==text)throw Error('Native Magpie translation differs: '+name);}else fs.writeFileSync(target,text);};
for(const filename of ['internal/gui/assets/i18n.js',...['routing','redact','usage','sessions','library','otel'].map(p=>'internal/gui/assets/plugins/'+p+'/i18n.js')]){
 const bytes=execFileSync('git',['-C',source,'show',revision+':'+filename],{maxBuffer:16<<20});inputs[filename]=createHash('sha256').update(bytes).digest('hex');
 const ast=ts.createSourceFile(filename,bytes.toString('utf8'),ts.ScriptTarget.Latest,true,ts.ScriptKind.JS);
 const visit=node=>{if(ts.isPropertyAssignment(node)&&node.name.getText(ast)==='zh'&&ts.isObjectLiteralExpression(node.initializer))for(const p of node.initializer.properties)if(ts.isPropertyAssignment(p)&&ts.isStringLiteral(p.name)&&ts.isStringLiteral(p.initializer))zh[p.name.text]=p.initializer.text;ts.forEachChild(node,visit);};visit(ast);
}
const adapt=s=>s.replaceAll(/\bmagpie\b/gi,'WinOne').replaceAll(/(?<!\{)\{([a-zA-Z0-9_]+)\}(?!\})/g,'{{$1}}');
for(const [locale,translate] of [['en-US',k=>k],['zh-CN',k=>zh[k]]]){
 const table=Object.fromEntries(Object.keys(zh).sort().map(k=>['aiUpstream.'+k,adapt(translate(k))]));
 emit('apps/desktop-mygo/i18n/ai-upstream.'+locale+'.json',table);
}
emit('third_party/magpie-native-locales.json',{repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice:'licenses/upstream-magpie-'+revision+'-0-LICENSE.txt',inputs,adaptations:['Use WinOne product name and native named interpolation; original upstream labels and Chinese translations']});
console.log('Native upstream labels generated:',Object.keys(zh).length);
