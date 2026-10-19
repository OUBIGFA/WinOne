// Extract complete upstream routing behavior and adapt its presentation to
// WinOne. The application shell and other plugin pages are never included.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
const root=resolve(import.meta.dirname,'..');
const require=createRequire(join(root,'apps/desktop/frontend/package.json'));
const {parse}=require('@babel/parser');
const traverse=require('@babel/traverse').default;
const html=require('parse5');
const sourceRoot=resolve(process.argv[2]||'');
const assets=join(sourceRoot,'internal/gui/assets');
const read=name=>readFile(join(assets,name),'utf8');
const original=await read('app.js');
let source=original;
const edits=[],changed=new Set();
const ast=parse(original,{sourceType:'script'});
// The group's picker supplies onPick; agent configuration and navigation
// belong to WinOne's other AI-service pages. Keep the actual picker widgets.
traverse(ast,{FunctionDeclaration(p){
 const name=p.node.id?.name;
 if(name==='openPicker'){
  for(const s of p.get('body.body'))if(s.isIfStatement()&&/FOLLOWS_MODEL|agent\.wired/.test(original.slice(s.node.test.start,s.node.test.end)))edits.push([s.node.start,s.node.end,'']);
  edits.push([p.node.body.start+1,p.node.body.start+1,'\n  if (typeof field.onPick !== "function") throw new Error("Routing picker requires its own selection callback");']);
  changed.add(name);
 }
 if(name==='commit'){
  const parts=p.get('body.body'),at=parts.findIndex(s=>s.isIfStatement()&&original.slice(s.node.test.start,s.node.test.end)==='field.onPick');
  if(at<0||!parts[at+1])throw Error('Magpie picker callback contract changed');
  edits.push([parts[at+1].node.start,p.node.body.end-1,'']);changed.add(name);
 }
 if(name==='input')p.traverse({IfStatement(s){if(original.slice(s.node.test.start,s.node.test.end).includes('e.key === "Escape"')){edits.push([s.node.start,s.node.end,'if (e.key === "Escape") closePicker();']);changed.add(name);}}});
 if(name==='suffixSegs')p.traverse({CallExpression(s){if(s.node.callee.name==='renderSettings'){edits.push([s.node.start,s.node.end,'redrawNoNames()']);changed.add(name);}}});
 if(name==='loadQuotas')p.traverse({CallExpression(s){if(['renderQuotas','loadUpstream'].includes(s.node.callee.name)){edits.push([s.node.start,s.node.end,'window.dispatchEvent(new Event("winone-routing-quotas"))']);changed.add(name);}}});
 if(name==='closeModal')p.traverse({IfStatement(s){if(original.slice(s.node.test.start,s.node.test.end).startsWith('adding &&')){edits.push([s.node.start,s.node.end,'']);changed.add(name);}}});
 // Routing can now enable its groups itself. Keep the upstream feature
 // configuration and visibility updates; WinOne owns navigation and tabs.
 if(name==='applyFeatures'){
  const parts=p.get('body.body'),at=parts.findIndex(s=>s.isExpressionStatement()&&s.node.expression.callee?.name==='featureStyle');
  if(at<0||!parts[at+1])throw Error('Magpie feature visibility contract changed');
  edits.push([parts[at+1].node.start,p.node.body.end-1,'']);changed.add(name);
 }
}});
if(changed.size!==7)throw Error('Magpie shared routing UI changed; review the seven host boundaries before building');
for(const [start,end,replacement]of edits.sort((a,b)=>b[0]-a[0]))source=source.slice(0,start)+replacement+source.slice(end);
const context=await read('context.js'),contextEnd=context.indexOf('window.ctxFmt = fmtK;');
if(contextEnd<0)throw Error('Magpie context-card export changed');
const routingContext=context.slice(0,contextEnd+'window.ctxFmt = fmtK;'.length)+'\n})();\n';
const originalRouting=await read('plugins/routing/client.js');
let routing=originalRouting;
// These are presentation boundaries, not a second routing implementation.
// Keep every editor, template, setting, trace and permission callback intact.
const {createElement}=require('react'),{renderToStaticMarkup}=require('react-dom/server'),{Waypoints}=require('lucide-react');
const routingIcon=renderToStaticMarkup(createElement(Waypoints,{size:26,'aria-hidden':true})).replace(/^<svg[^>]*>/,'').replace(/<\/svg>$/,'');
for(const [from,to] of [
 ['more.append(gsec);','$("#view-routing").insertBefore(gsec, $("#view-routing").firstElementChild);'],
 ['data-t>Routing</span>','data-t>Request activity</span>'],
 ['Routing groups are off in Plugins › Built-in: the groups magpie finds, one model several providers serve, still route, but your own can\'t be made or changed here.','Turn on routing groups to create or edit custom groups. Automatic groups remain available.'],
 ["logo.innerHTML = '<use href=\"#bird\"/>';",'logo.setAttribute("viewBox", "0 0 24 24"); logo.setAttribute("fill", "none"); logo.setAttribute("stroke", "currentColor"); logo.setAttribute("stroke-width", "1.75"); logo.setAttribute("stroke-linecap", "round"); logo.setAttribute("stroke-linejoin", "round"); logo.innerHTML = '+JSON.stringify(routingIcon)+';'],
 ['hub.append(logo, el("b", "", "magpie"), hubSub, chip);','hub.append(logo, el("b", "", "WinOne"), hubSub, chip);'],
 ]){if(routing.split(from).length!==2)throw Error('Routing presentation boundary changed: '+from);routing=routing.replace(from,to);}
const roots=[routing,await read('kernel.js'),routingContext];
let program;
traverse(parse(source,{sourceType:'script'}),{Program(p){program=p;}});
function referenced(p){
 const result=new Set();
 p.traverse({Identifier(id){
  if(!id.isReferencedIdentifier()&&!id.isBindingIdentifier())return;
  const binding=id.scope.getBinding(id.node.name);
  if(!binding||binding.scope===program.scope)result.add(id.node.name);
 }});
 return result;
}
const seed=new Set(['scrollOnPurpose','redrawNoNames','closePicker','filter','move','commit','toggleFold','prefsSettled']);
for(const text of roots)traverse(parse(text,{sourceType:'script'}),{Identifier(p){if((p.isReferencedIdentifier()||p.isBindingIdentifier())&&!p.scope.getBinding(p.node.name))seed.add(p.node.name);}});
const overrides=new Set(['load','loadProviders','show','mode','state','view','applyPrefs','fit']);
const selected=new Set(),names=new Set(),queue=[...seed];
const reasons=new Map([...seed].map(name=>[name,'routing plugin']));
const enqueue=(refs,reason)=>{for(const name of refs){if(!reasons.has(name))reasons.set(name,reason);queue.push(name);}};
const topLevel=program.get('body');
const from=source.indexOf('let purposeUntil ='),to=source.indexOf('async function show(');
if(from<0||to<from)throw Error('Magpie scroll lifecycle boundary changed');
const pickerFrom=source.indexOf('$("#q").addEventListener("input", filter)'),pickerTo=source.indexOf('// ---------- profiles ----------',pickerFrom);
if(pickerFrom<0||pickerTo<pickerFrom)throw Error('Magpie picker event boundary changed');
for(const p of topLevel){
 const text=source.slice(p.node.start,p.node.end);
 const effect=!p.isFunctionDeclaration()&&!p.isVariableDeclaration();
 if((p.node.start>=pickerFrom&&p.node.start<pickerTo&&!text.includes('profileEscape()'))
   ||(effect&&p.node.start>=from&&p.node.start<to)
   ||(effect&&/segsPress|window\.hideAccounts =|pressed = \{/.test(text))
   ||(effect&&text.startsWith('document.addEventListener("mouseover"'))){selected.add(p);enqueue(referenced(p),'UI event at '+p.node.loc.start.line);}
}
while(queue.length){
 const name=queue.shift();if(names.has(name)||overrides.has(name))continue;names.add(name);
 const binding=program.scope.getBinding(name);if(!binding)continue;
 let top=binding.path;while(!top.parentPath.isProgram())top=top.parentPath;
 if(selected.has(top))continue;selected.add(top);enqueue(referenced(top),name);
 // Restore the selected widget's persisted state, without loading unrelated
 // app preferences, clients, update checks or provider discovery.
 for(const effect of topLevel.filter(p=>p.isTryStatement())){
  if(selected.has(effect))continue;
  let writes=false;effect.traverse({AssignmentExpression(p){if(p.get('left').isIdentifier({name}))writes=true;}});
  if(writes){selected.add(effect);enqueue(referenced(effect),name);}
 }
}
const order=[...selected].sort((a,b)=>a.node.start-b.node.start);
const forbidden=['renderAgents','renderProviders','renderSettings','loadSettings','loadInstalls','layoutNav','navHides','setPick'];
for(const name of forbidden)if(names.has(name)){
 const chain=[name];while(reasons.has(chain.at(-1))&&!chain.includes(reasons.get(chain.at(-1))))chain.push(reasons.get(chain.at(-1)));
 throw Error(`Routing now depends on application page ${chain.reverse().join(' → ')}; adapt its host boundary first`);
}
const shared='// Generated from Magpie app.js: only routing UI dependencies. See winone-routing-source.json.\n'+order.map(p=>source.slice(p.node.start,p.node.end)).join('\n\n')+'\n';
const document=html.parse(await read('index.html'));
const nodes=[];function walk(n){nodes.push(n);for(const child of n.childNodes||[])walk(child);}walk(document);
const id=node=>node.attrs?.find(a=>a.name==='id')?.value;
const only=key=>{const node=nodes.find(n=>id(n)===key);if(!node)throw Error(`Magpie routing UI element ${key} missing`);return html.serializeOuter(node);};
const bird=nodes.find(n=>n.tagName==='defs'&&n.childNodes?.some(c=>id(c)==='bird'));
if(!bird)throw Error('Magpie routing animation definitions missing');
const page=`<!doctype html>
<html lang="en" class="winone-routing winone-ai-page" data-settings-ui><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WinOne · Routing</title>
<script src="winone-routing-bridge.js"></script><script src="boot.js"></script><script src="theme.js"></script><script src="compat.js"></script>
<link rel="stylesheet" href="app.css"><link rel="stylesheet" href="plugins/routing/routing.css"><link rel="stylesheet" href="plugins/library/library.css"><link rel="stylesheet" href="context.css"><link rel="stylesheet" href="winone-ai-settings.css"><link rel="stylesheet" href="winone-routing.css"></head>
<body class="window win"><svg width="0" height="0" aria-hidden="true" class="winone-routing-defs">${html.serializeOuter(bird)}</svg>
<template data-slot="page"></template><span id="status" class="status" role="status" aria-live="polite" aria-atomic="true"></span>${only('modal')}${only('pop')}
<script src="i18n.js"></script><script src="kernel.js"></script><script src="plugins/routing/i18n.js"></script><script src="plugins/usage/i18n.js"></script><script src="purposes.js"></script>
<script src="winone-routing-client.js"></script><script src="winone-routing-shared.js"></script><script src="winone-routing-context.js"></script><script src="winone-routing.js"></script></body></html>\n`;
const revision=execFileSync('git',['-C',sourceRoot,'rev-parse','HEAD'],{encoding:'utf8',windowsHide:true}).trim();
const hash=value=>createHash('sha256').update(value).digest('hex');
await writeFile(join(assets,'winone-routing-client.js'),routing,'utf8');
await writeFile(join(assets,'winone-routing.html'),page,'utf8');
await writeFile(join(assets,'winone-routing-shared.js'),shared,'utf8');
await writeFile(join(assets,'winone-routing-context.js'),routingContext,'utf8');
await writeFile(join(assets,'winone-routing-source.json'),JSON.stringify({revision,upstreamAppSHA256:hash(original),routingSHA256:hash(routing),upstreamRoutingSHA256:hash(originalRouting),presentation:'WinOne settings; complete routing behavior retained',sharedSHA256:hash(shared),adaptedHelpers:[...changed],helpers:order.map(p=>p.node.id?.name||p.node.declarations?.map(d=>source.slice(d.id.start,d.id.end))||p.node.type),pages:['routing']},null,2)+'\n','utf8');
console.log(`Prepared Magpie ${revision.slice(0,8)} routing plugin with ${order.length} shared UI declarations; no application shell`);
