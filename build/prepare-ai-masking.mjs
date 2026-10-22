// Reuse the complete upstream masking summary/records widgets in Privacy,
// with only their dependencies. Never import the Settings/application shell.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
const root=resolve(import.meta.dirname,'..'),source=resolve(process.argv[2]),assets=join(source,'internal/gui/assets');
const require=createRequire(join(root,'apps/desktop/frontend/package.json')),{parse}=require('@babel/parser'),traverse=require('@babel/traverse').default;
const text=await readFile(join(assets,'app.js'),'utf8');
let program;traverse(parse(text,{sourceType:'script'}),{Program(p){program=p;}});
const selected=new Set(),seen=new Set(),queue=['renderMaskObs','maskOpenRecords','renderMaskTry'];
const overrides=new Set(['state','view','mode','params','setShown','openMaskRecord','fit']);
function refs(p){p.traverse({Identifier(id){if(!id.isReferencedIdentifier()&&!id.isBindingIdentifier())return;const b=id.scope.getBinding(id.node.name);if(!b||b.scope===program.scope)queue.push(id.node.name);}});}
const from=text.indexOf('// ---------- what masking swapped'),to=text.indexOf('function renderOTel(',from);
if(from<0||to<from)throw Error('Magpie masking widget boundary changed');
for(const p of program.get('body'))if(p.node.start>=from&&p.node.start<to&&!p.isFunctionDeclaration()&&!p.isVariableDeclaration()){selected.add(p);refs(p);}
while(queue.length){const name=queue.shift();if(seen.has(name)||overrides.has(name))continue;seen.add(name);const b=program.scope.getBinding(name);if(!b)continue;let top=b.path;while(!top.parentPath.isProgram())top=top.parentPath;if(selected.has(top))continue;selected.add(top);refs(top);}
for(const name of ['renderSettings','loadSettings','renderProviders','renderAgents','show','openSettings'])if(seen.has(name))throw Error('Privacy records depend on application shell: '+name);
let shared=[...selected].sort((a,b)=>a.node.start-b.node.start).map(p=>text.slice(p.node.start,p.node.end)).join('\n\n');
// An unavailable summary is not a perpetual skeleton or an empty success.
const swallowed='}).catch(() => {}).finally(() => { if (maskBusy === p) maskBusy = null; });';
if(!shared.includes(swallowed))throw Error('Review masking summary error handling');
shared=shared.replace(swallowed,'}).catch(error => { window.winOneMaskingError(error); }).finally(() => { if (maskBusy === p) maskBusy = null; });');
await writeFile(join(assets,'winone-masking-shared.js'),shared+'\n');
const html=`<!doctype html><html class="winone-ai-page winone-masking" lang="en" data-settings-ui><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WinOne · Privacy</title><script src="winone-plugin-bridge.js"></script><script src="boot.js"></script><script src="theme.js"></script><script src="compat.js"></script><link rel="stylesheet" href="app.css"><link rel="stylesheet" href="winone-ai-settings.css"><link rel="stylesheet" href="winone-masking.css"></head><body class="window win"><main id="setPage-privacy"><section><h2 data-t>Try it</h2><div id="maskTryList"></div></section><div id="maskObs" class="mask-obs"></div><div id="maskRecs" class="mask-recs" hidden></div></main><div id="status" role="status" class="status"></div><script src="i18n.js"></script><script src="winone-masking-shared.js"></script><script src="winone-masking-host.js"></script></body></html>`;
await writeFile(join(assets,'masking.html'),html);
const revision=execFileSync('git',['-C',source,'rev-parse','HEAD'],{encoding:'utf8'}).trim();
await writeFile(join(assets,'winone-masking-source.json'),JSON.stringify({revision,upstreamSHA256:createHash('sha256').update(text).digest('hex'),sharedSHA256:createHash('sha256').update(shared).digest('hex'),declarations:selected.size,adaptations:['WinOne Privacy placement and shared grayscale controls','visible summary failure/retry','no application shell']},null,2)+'\n');
console.log('Prepared complete masking records with '+selected.size+' shared declarations');
