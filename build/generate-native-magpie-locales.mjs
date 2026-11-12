// Keep only labels used by the retained native gateway. Removed upstream
// pages must not keep their resources in the product's embedded dictionaries.
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
for(const filename of ['internal/gui/assets/i18n.js']){
 const bytes=execFileSync('git',['-C',source,'show',revision+':'+filename],{maxBuffer:16<<20});inputs[filename]=createHash('sha256').update(bytes).digest('hex');
 const ast=ts.createSourceFile(filename,bytes.toString('utf8'),ts.ScriptTarget.Latest,true,ts.ScriptKind.JS);
 const visit=node=>{if(ts.isPropertyAssignment(node)&&node.name.getText(ast)==='zh'&&ts.isObjectLiteralExpression(node.initializer))for(const p of node.initializer.properties)if(ts.isPropertyAssignment(p)&&ts.isStringLiteral(p.name)&&ts.isStringLiteral(p.initializer))zh[p.name.text]=p.initializer.text;ts.forEachChild(node,visit);};visit(ast);
}
const adapt=s=>s.replaceAll(/\bmagpie\b/gi,'WinOne').replaceAll(/(?<!\{)\{([a-zA-Z0-9_]+)\}(?!\})/g,'{{$1}}');
const used=new Set(),pages=path.join(root,'apps/desktop-mygo/pages/aiservices');
for(const filename of fs.readdirSync(pages).filter(name=>name.endsWith('.go')&&!name.endsWith('_test.go'))){
 const text=fs.readFileSync(path.join(pages,filename),'utf8');
 for(const match of text.matchAll(/upstreamT\(\s*("(?:\\.|[^"\\])*")/g))used.add(JSON.parse(match[1]));
}
for(const example of JSON.parse(fs.readFileSync(path.join(pages,'gateway_examples.json'),'utf8')))used.add(example.note);
// Native wording absent from upstream has one manually maintained owner.
// Keep generated entries disjoint because the runtime rejects duplicate keys.
const native={};
for(const locale of ['en-US','zh-CN'])native[locale]=JSON.parse(fs.readFileSync(path.join(root,'apps/desktop-mygo/i18n/ai-upstream-adapted.'+locale+'.json'),'utf8'));
for(const key of used){
 const id='aiUpstream.'+key;
 if(!!native['en-US'][id]!==!!native['zh-CN'][id])throw Error('Native gateway translation languages differ: '+key);
 if(!zh[key]&&!native['zh-CN'][id])throw Error('Missing retained gateway translation: '+key);
}
for(const [locale,translate] of [['en-US',k=>k],['zh-CN',k=>zh[k]]]){
 const table=Object.fromEntries([...used].filter(k=>!native[locale]['aiUpstream.'+k]).sort().map(k=>['aiUpstream.'+k,adapt(translate(k))]));
 emit('apps/desktop-mygo/i18n/ai-upstream.'+locale+'.json',table);
}
emit('third_party/magpie-native-locales.json',{repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice:'licenses/upstream-magpie-'+revision+'-0-LICENSE.txt',inputs,adaptations:['Retain only native gateway labels and connection notes; use WinOne product name and named interpolation; remove the unavailable Agents-page direction']});
console.log('Native upstream labels generated:',used.size);
