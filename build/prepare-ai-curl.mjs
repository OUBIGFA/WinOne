// Reuse the selected Magpie parser, including its Windows shell quoting rules.
// Only the two pure declarations are copied; no application UI is imported.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
const root=resolve(import.meta.dirname,'..'),source=resolve(process.argv[2]||'');
if(!process.argv[2])throw Error('Usage: prepare-ai-curl.mjs <Magpie-source>');
const require=createRequire(join(root,'apps/desktop/frontend/package.json'));
const {parse}=require('@babel/parser');
const code=await readFile(join(source,'internal/gui/assets/app.js'),'utf8');
const names=['respellURL','curlOf'],functions=names.map(name=>{
 const matches=parse(code,{sourceType:'script'}).program.body.filter(node=>node.type==='FunctionDeclaration'&&node.id.name===name);
 if(matches.length!==1)throw Error('Upstream curl parser changed: '+name);
 const node=matches[0];return code.slice(node.start,node.end);
});
const revision=execFileSync('git',['-C',source,'rev-parse','HEAD'],{encoding:'utf8',windowsHide:true}).trim();
await writeFile(join(root,'apps/desktop/frontend/src/features/magpie-curl.js'),['// Magpie '+revision+' / MIT. Extracted by build/prepare-ai-curl.mjs.',...functions,'export {curlOf};',''].join('\n'));
console.log('Prepared the upstream curl import parser at '+revision);
