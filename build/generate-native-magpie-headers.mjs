// Extract the original preset values as data; never execute upstream page code.
import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import ts from '../apps/desktop/frontend/node_modules/typescript/lib/typescript.js';
const root=path.resolve(import.meta.dirname,'..');
const source=process.argv[2];
if(!source)throw Error('Pass the pinned Magpie source directory');
const revision=JSON.parse(fs.readFileSync(path.join(root,'third_party/sources.json'),'utf8')).magpie.commit;
const input='internal/gui/assets/headerpresets.js';
const bytes=execFileSync('git',['-C',source,'show',revision+':'+input]);
const ast=ts.createSourceFile(input,bytes.toString('utf8'),ts.ScriptTarget.Latest,true,ts.ScriptKind.JS);
function literal(node){
 if(ts.isStringLiteral(node))return node.text;
 if(ts.isArrayLiteralExpression(node))return node.elements.map(literal);
 if(ts.isObjectLiteralExpression(node))return Object.fromEntries(node.properties.map(p=>{
  if(!ts.isPropertyAssignment(p))throw Error('Review non-literal header preset property');
  return [ts.isIdentifier(p.name)||ts.isStringLiteral(p.name)?p.name.text:p.name.getText(ast),literal(p.initializer)];
 }));
 throw Error('Review non-literal header preset');
}
let presets;
function visit(node){if(ts.isVariableDeclaration(node)&&node.name.getText(ast)==='HEADER_PRESETS')presets=literal(node.initializer);ts.forEachChild(node,visit);}
visit(ast);
if(!Array.isArray(presets)||presets.length===0)throw Error('Upstream header presets were not found');
const hash=b=>createHash('sha256').update(b).digest('hex');
const target='apps/desktop-mygo/pages/aiservices/header_presets.json';
const data=JSON.stringify(presets,null,2)+'\n';
const manifest={repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice:'licenses/upstream-magpie-'+revision+'-0-LICENSE.txt',inputs:{[input]:hash(bytes)},files:{[target]:hash(data)},adaptation:'Original preset names and header bytes; original case-insensitive fill-only merge, rendered with existing native controls.'};
for(const [name,text] of [[target,data],['third_party/magpie-native-headers.json',JSON.stringify(manifest,null,2)+'\n']]){
 const destination=path.join(root,name);
 if(process.argv.includes('--check')){if(fs.readFileSync(destination,'utf8').replaceAll('\r\n','\n')!==text)throw Error('Native header presets differ: '+name);}
 else fs.writeFileSync(destination,text,'utf8');
}
console.log('Original Magpie header presets verified:',presets.length);
